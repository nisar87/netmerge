package engine

import (
	"time"
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Metadata describes one connection while the engine decides where it goes.
type Metadata struct {
	Network     string // "tcp" | "udp"
	Type        string // how it came in: "HTTP", "HTTPS", "Socks5", "Shadowsocks", "Tun"
	SrcIP       netip.Addr
	SrcPort     uint16
	DstIP       netip.Addr
	DstPort     uint16
	Host        string // website name, when known (asked for by name, found by sniffing or from the built-in DNS)
	SniffHost   string // name found by sniffing when the connection was made to an address
	Process     string // program name on this PC, e.g. chrome.exe
	ProcessPath string
	processDone bool
	born        time.Time // when the engine first saw the connection (for NM_TRACE timings)
}

// RemoteAddress is "host:port" as written in the log: the website's name when it is known (asked for, or found
// in the connection), else the address. NetMerge's "Blocked today" list shows what is written here.
func (m *Metadata) RemoteAddress() string {
	h := m.Host
	if h == "" {
		h = m.SniffHost
	}
	if h == "" && m.DstIP.IsValid() {
		h = m.DstIP.Unmap().String()
	}
	if strings.Contains(h, ":") {
		return "[" + h + "]:" + strconv.Itoa(int(m.DstPort))
	}
	return h + ":" + strconv.Itoa(int(m.DstPort))
}

// ruleHost is the name rules match against: the requested name, else a sniffed one.
func (m *Metadata) ruleHost() string {
	if m.Host != "" {
		return m.Host
	}
	return m.SniffHost
}

// ruleEnv gives rules what they may need on demand (the program, an address for a name).
type ruleEnv struct {
	process func(m *Metadata)
	resolve func(host string) (netip.Addr, bool)
}

type condition interface {
	match(m *Metadata, env *ruleEnv) bool
	name() string    // as mihomo prints it, e.g. DomainSuffix
	payload() string // e.g. example.com
}

// Rule is one line of the rule list: a condition and where matching connections go.
type Rule struct {
	cond   condition
	Target string
}

func (r *Rule) Name() string    { return r.cond.name() }
func (r *Rule) Payload() string { return r.cond.payload() }

// Describe is how the log names the rule: "DomainSuffix(example.com)", "Match", "AND(...)".
func (r *Rule) Describe() string {
	if lc, ok := r.cond.(*logicCond); ok {
		return lc.name() + lc.payload()
	}
	if r.cond.payload() == "" {
		return r.cond.name()
	}
	return r.cond.name() + "(" + r.cond.payload() + ")"
}

// ---- conditions ----

type matchAll struct{}

func (matchAll) match(*Metadata, *ruleEnv) bool { return true }
func (matchAll) name() string                   { return "Match" }
func (matchAll) payload() string                { return "" }

type domainCond struct{ d string }

func (c domainCond) match(m *Metadata, _ *ruleEnv) bool { return strings.EqualFold(m.ruleHost(), c.d) }
func (c domainCond) name() string                       { return "Domain" }
func (c domainCond) payload() string                    { return c.d }

type suffixCond struct{ d string }

func (c suffixCond) match(m *Metadata, _ *ruleEnv) bool {
	h := strings.ToLower(m.ruleHost())
	return h == c.d || strings.HasSuffix(h, "."+c.d)
}
func (c suffixCond) name() string    { return "DomainSuffix" }
func (c suffixCond) payload() string { return c.d }

type keywordCond struct{ k string }

func (c keywordCond) match(m *Metadata, _ *ruleEnv) bool {
	return strings.Contains(strings.ToLower(m.ruleHost()), c.k)
}
func (c keywordCond) name() string    { return "DomainKeyword" }
func (c keywordCond) payload() string { return c.k }

type cidrCond struct {
	p         netip.Prefix
	raw       string
	noResolve bool
	src       bool
}

func (c cidrCond) match(m *Metadata, env *ruleEnv) bool {
	if c.src {
		return m.SrcIP.IsValid() && c.p.Contains(m.SrcIP.Unmap())
	}
	ip := m.DstIP
	if !ip.IsValid() && !c.noResolve && m.Host != "" && env != nil && env.resolve != nil {
		if a, ok := env.resolve(m.Host); ok {
			ip = a
		}
	}
	return ip.IsValid() && c.p.Contains(ip.Unmap())
}
func (c cidrCond) name() string {
	if c.src {
		return "SrcIPCIDR"
	}
	return "IPCIDR"
}
func (c cidrCond) payload() string { return c.raw }

type portCond struct {
	ps  portSet
	raw string
}

func (c portCond) match(m *Metadata, _ *ruleEnv) bool { return c.ps.has(m.DstPort) }
func (c portCond) name() string                       { return "DstPort" }
func (c portCond) payload() string                    { return c.raw }

type networkCond struct{ n string }

func (c networkCond) match(m *Metadata, _ *ruleEnv) bool { return m.Network == c.n }
func (c networkCond) name() string                       { return "Network" }
func (c networkCond) payload() string                    { return c.n }

type processCond struct {
	re    *regexp.Regexp
	exact string
	raw   string
}

func (c processCond) match(m *Metadata, env *ruleEnv) bool {
	if !m.processDone && env != nil && env.process != nil {
		env.process(m)
	}
	if m.Process == "" {
		return false
	}
	if c.re != nil {
		return c.re.MatchString(m.Process)
	}
	return strings.EqualFold(m.Process, c.exact)
}
func (c processCond) name() string {
	if c.re != nil {
		return "ProcessNameRegex"
	}
	return "ProcessName"
}
func (c processCond) payload() string { return c.raw }

type ruleSetCond struct {
	set *DomainSet
	nm  string
}

func (c ruleSetCond) match(m *Metadata, _ *ruleEnv) bool {
	return c.set != nil && c.set.Has(m.ruleHost())
}
func (c ruleSetCond) name() string    { return "RuleSet" }
func (c ruleSetCond) payload() string { return c.nm }

// logicCond is AND / OR / NOT over other conditions.
type logicCond struct {
	op   string // AND | OR | NOT
	subs []condition
}

func (c *logicCond) match(m *Metadata, env *ruleEnv) bool {
	switch c.op {
	case "AND":
		for _, s := range c.subs {
			if !s.match(m, env) {
				return false
			}
		}
		return true
	case "OR":
		for _, s := range c.subs {
			if s.match(m, env) {
				return true
			}
		}
		return false
	default: // NOT
		return len(c.subs) == 1 && !c.subs[0].match(m, env)
	}
}
func (c *logicCond) name() string { return c.op }

// payload is written the way mihomo writes it, because NetMerge's block counter reads it from the log:
// AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow)))))
func (c *logicCond) payload() string { return "(" + c.inner() + ")" }

func (c *logicCond) inner() string {
	part := func(s condition) string {
		if lc, ok := s.(*logicCond); ok {
			if lc.op == "NOT" {
				return "(NOT,(!" + part0(lc.subs[0]) + "))"
			}
			return "(" + lc.op + "," + lc.inner() + ")"
		}
		return part0(s)
	}
	if c.op == "NOT" {
		return "(!" + part0(c.subs[0]) + ")"
	}
	var ps []string
	for _, s := range c.subs {
		ps = append(ps, part(s))
	}
	sep := " && "
	if c.op == "OR" {
		sep = " || "
	}
	return "(" + strings.Join(ps, sep) + ")"
}

func part0(s condition) string {
	if lc, ok := s.(*logicCond); ok {
		return "(" + lc.op + "," + lc.inner() + ")"
	}
	return "(" + s.name() + "," + s.payload() + ")"
}

// ---- parsing ----

// ParseRules turns NetMerge's rule lines into rules. Rule sets come from providers (already loaded).
func ParseRules(lines []string, sets map[string]*DomainSet) ([]*Rule, error) {
	var out []*Rule
	for _, ln := range lines {
		r, err := parseRule(ln, sets)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", ln, err)
		}
		out = append(out, r)
	}
	return out, nil
}

func parseRule(ln string, sets map[string]*DomainSet) (*Rule, error) {
	ln = strings.TrimSpace(ln)
	kind, rest, _ := strings.Cut(ln, ",")
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if kind == "MATCH" {
		return &Rule{cond: matchAll{}, Target: strings.TrimSpace(rest)}, nil
	}
	if kind == "AND" || kind == "OR" || kind == "NOT" {
		// AND,((A,x),(B,y)),TARGET
		end := matchingParen(rest, 0)
		if !strings.HasPrefix(rest, "(") || end < 0 {
			return nil, fmt.Errorf("unbalanced brackets")
		}
		c, err := parseLogic(kind, rest[1:end], sets)
		if err != nil {
			return nil, err
		}
		target := strings.TrimPrefix(rest[end+1:], ",")
		return &Rule{cond: c, Target: strings.TrimSpace(target)}, nil
	}
	parts := strings.Split(rest, ",")
	if len(parts) < 2 {
		return nil, fmt.Errorf("needs a value and a target")
	}
	// PROCESS-NAME-REGEX may contain commas in theory; the target is the last part unless that is no-resolve
	opts := map[string]bool{}
	for len(parts) > 2 && (parts[len(parts)-1] == "no-resolve" || parts[len(parts)-1] == "src") {
		opts[parts[len(parts)-1]] = true
		parts = parts[:len(parts)-1]
	}
	target := strings.TrimSpace(parts[len(parts)-1])
	payload := strings.Join(parts[:len(parts)-1], ",")
	c, err := parseCond(kind, payload, opts, sets)
	if err != nil {
		return nil, err
	}
	return &Rule{cond: c, Target: target}, nil
}

func parseCond(kind, payload string, opts map[string]bool, sets map[string]*DomainSet) (condition, error) {
	payload = strings.TrimSpace(payload)
	switch kind {
	case "DOMAIN":
		return domainCond{strings.ToLower(payload)}, nil
	case "DOMAIN-SUFFIX":
		return suffixCond{strings.ToLower(strings.TrimPrefix(payload, "."))}, nil
	case "DOMAIN-KEYWORD":
		return keywordCond{strings.ToLower(payload)}, nil
	case "IP-CIDR", "IP-CIDR6", "SRC-IP-CIDR":
		p, err := netip.ParsePrefix(payload)
		if err != nil {
			a, err2 := netip.ParseAddr(payload)
			if err2 != nil {
				return nil, err
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		return cidrCond{p: p.Masked(), raw: payload, noResolve: opts["no-resolve"], src: kind == "SRC-IP-CIDR"}, nil
	case "DST-PORT":
		ps := parsePorts(splitPorts(payload))
		if len(ps) == 0 {
			return nil, fmt.Errorf("no port")
		}
		return portCond{ps: ps, raw: payload}, nil
	case "NETWORK":
		return networkCond{strings.ToLower(payload)}, nil
	case "PROCESS-NAME":
		return processCond{exact: payload, raw: payload}, nil
	case "PROCESS-NAME-REGEX":
		re, err := regexp.Compile(payload)
		if err != nil {
			return nil, err
		}
		return processCond{re: re, raw: payload}, nil
	case "RULE-SET":
		s, ok := sets[payload]
		if !ok {
			return nil, fmt.Errorf("rule set %q is not defined", payload)
		}
		return ruleSetCond{set: s, nm: payload}, nil
	}
	return nil, fmt.Errorf("unknown rule type %s", kind)
}

func splitPorts(s string) []any {
	var out []any
	for _, p := range strings.Split(s, "/") {
		if n, err := strconv.Atoi(p); err == nil {
			out = append(out, float64(n))
		} else {
			out = append(out, p)
		}
	}
	return out
}

// parseLogic reads "(A,x),(B,y)" (the inside of the outer brackets).
func parseLogic(op, s string, sets map[string]*DomainSet) (condition, error) {
	lc := &logicCond{op: op}
	for i := 0; i < len(s); {
		if s[i] == ',' || s[i] == ' ' {
			i++
			continue
		}
		if s[i] != '(' {
			return nil, fmt.Errorf("expected '(' in %q", s)
		}
		end := matchingParen(s, i)
		if end < 0 {
			return nil, fmt.Errorf("unbalanced brackets in %q", s)
		}
		inner := s[i+1 : end]
		kind, rest, _ := strings.Cut(inner, ",")
		kind = strings.ToUpper(strings.TrimSpace(kind))
		var c condition
		var err error
		if kind == "AND" || kind == "OR" || kind == "NOT" {
			rest = strings.TrimSpace(rest)
			e2 := matchingParen(rest, 0)
			if !strings.HasPrefix(rest, "(") || e2 < 0 {
				return nil, fmt.Errorf("unbalanced brackets in %q", inner)
			}
			c, err = parseLogic(kind, rest[1:e2], sets)
		} else {
			opts := map[string]bool{}
			ps := strings.Split(rest, ",")
			for len(ps) > 1 && (ps[len(ps)-1] == "no-resolve" || ps[len(ps)-1] == "src") {
				opts[ps[len(ps)-1]] = true
				ps = ps[:len(ps)-1]
			}
			c, err = parseCond(kind, strings.Join(ps, ","), opts, sets)
		}
		if err != nil {
			return nil, err
		}
		lc.subs = append(lc.subs, c)
		i = end + 1
	}
	if op == "NOT" && len(lc.subs) != 1 {
		return nil, fmt.Errorf("NOT takes exactly one condition")
	}
	if len(lc.subs) == 0 {
		return nil, fmt.Errorf("%s without conditions", op)
	}
	return lc, nil
}

func matchingParen(s string, open int) int {
	if open >= len(s) || s[open] != '(' {
		return -1
	}
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// ---- domain sets (block and allow lists) ----

// DomainSet holds a domain list in mihomo's text form: "+.x" (x and below), ".x" (below x only), "*.x" (one level
// below x) and "x" (exactly x). Looking up a name costs one map lookup per label.
type DomainSet struct {
	exact, tree, sub, wild map[string]struct{}
	n                      int
}

func NewDomainSet() *DomainSet {
	return &DomainSet{exact: map[string]struct{}{}, tree: map[string]struct{}{}, sub: map[string]struct{}{}, wild: map[string]struct{}{}}
}

func (d *DomainSet) Add(line string) {
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" || line[0] == '#' {
		return
	}
	switch {
	case strings.HasPrefix(line, "+."):
		d.tree[line[2:]] = struct{}{}
	case strings.HasPrefix(line, "*."):
		d.wild[line[2:]] = struct{}{}
	case strings.HasPrefix(line, "."):
		d.sub[line[1:]] = struct{}{}
	default:
		d.exact[line] = struct{}{}
	}
	d.n++
}

func (d *DomainSet) Len() int { return d.n }

func (d *DomainSet) Has(host string) bool {
	if host == "" {
		return false
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if _, ok := d.exact[host]; ok {
		return true
	}
	s, first := host, true
	for {
		if _, ok := d.tree[s]; ok {
			return true
		}
		if !first {
			if _, ok := d.sub[s]; ok {
				return true
			}
		}
		i := strings.IndexByte(s, '.')
		if i < 0 {
			return false
		}
		if first {
			// "*.x" matches exactly one label in front of x
			if _, ok := d.wild[s[i+1:]]; ok {
				return true
			}
		}
		s, first = s[i+1:], false
	}
}

// LoadDomainSet reads a text domain list from a file.
func LoadDomainSet(path string) (*DomainSet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d := NewDomainSet()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		d.Add(sc.Text())
	}
	return d, sc.Err()
}
