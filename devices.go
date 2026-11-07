package main

import (
	"context"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Finding out what a connected device is. We never see a phone's own name (it reaches us through a proxy), so
// we combine what the PC can learn: the hardware address (ARP table), the name the router knows for the IP
// (reverse DNS) and the maker of the network chip. A name you type always wins.

// DeviceView is what the Share page shows for one connected device.
type DeviceView struct {
	DeviceStat
	ID      string `json:"id"`      // key used to remember the name (hardware address, or the IP when unknown)
	MAC     string `json:"mac"`     // hardware address, if known
	Name    string `json:"name"`    // the name to show
	Source  string `json:"source"`  // "you" | "router" | "maker" | ""
	Host    string `json:"host"`    // name from the router
	Vendor  string `json:"vendor"`  // maker of the network chip
	Private bool   `json:"private"` // phone uses a random (private) hardware address
}

var ouiVendors = map[string]string{ // best-effort, only prefixes we are sure about
	"3c:07:54": "Apple", "f0:18:98": "Apple", "00:03:93": "Apple", "00:0a:95": "Apple",
	"00:12:47": "Samsung", "8c:77:12": "Samsung", "5c:0a:5b": "Samsung",
	"f8:a4:5f": "Xiaomi", "64:09:80": "Xiaomi", "28:6c:07": "Xiaomi",
	"00:e0:fc": "Huawei", "00:9a:cd": "Huawei",
	"3c:5a:b4": "Google", "f4:f5:d8": "Google",
	"50:c7:bf": "TP-Link", "14:cc:20": "TP-Link",
	"b8:27:eb": "Raspberry Pi", "dc:a6:32": "Raspberry Pi",
}

var macRe = regexp.MustCompile(`(?i)^\s*(\d+\.\d+\.\d+\.\d+)\s+([0-9a-f]{2}(?:-[0-9a-f]{2}){5})\s+\S+`)

type devInfo struct {
	mac, host string
	at        time.Time
}

var (
	devMu    sync.Mutex
	devCache = map[string]devInfo{}
	devBusy  = map[string]bool{}
)

// lookupDevice returns cached facts at once and refreshes them in the background (never blocks the UI).
func lookupDevice(ip string) devInfo {
	devMu.Lock()
	defer devMu.Unlock()
	d, ok := devCache[ip]
	if (!ok || time.Since(d.at) > 5*time.Minute || (d.mac == "" && time.Since(d.at) > 20*time.Second)) && !devBusy[ip] {
		devBusy[ip] = true
		go func() {
			n := devInfo{at: time.Now()}
			if out, err := hiddenCmd("arp", "-a").Output(); err == nil {
				for _, ln := range strings.Split(string(out), "\n") {
					if m := macRe.FindStringSubmatch(ln); m != nil && m[1] == ip {
						n.mac = strings.ToLower(strings.ReplaceAll(m[2], "-", ":"))
					}
				}
			}
			n.host = routerName(ip)
			logf("device %s: mac=%s name=%q", ip, n.mac, n.host)
			devMu.Lock()
			devCache[ip] = n
			delete(devBusy, ip)
			devMu.Unlock()
		}()
	}
	return d
}

func deviceViews(list []DeviceStat, names map[string]string) []DeviceView {
	out := make([]DeviceView, 0, len(list))
	for _, d := range list {
		info := lookupDevice(d.IP)
		v := DeviceView{DeviceStat: d, ID: d.IP, MAC: info.mac, Host: info.host}
		if info.mac != "" {
			v.ID = info.mac
			if b, err := hexByte(info.mac[:2]); err == nil && b&2 != 0 {
				v.Private = true // locally administered = random address
			} else {
				v.Vendor = ouiVendors[info.mac[:8]]
			}
		}
		switch {
		case names[v.ID] != "":
			v.Name, v.Source = names[v.ID], "you"
		case names[d.IP] != "":
			v.Name, v.Source = names[d.IP], "you"
		case v.Host != "":
			v.Name, v.Source = v.Host, "router"
		case v.Vendor != "":
			v.Name, v.Source = v.Vendor+" device", "maker"
		}
		out = append(out, v)
	}
	return out
}

func hexByte(s string) (int, error) {
	n := 0
	for _, c := range s {
		n *= 16
		switch {
		case c >= '0' && c <= '9':
			n += int(c - '0')
		case c >= 'a' && c <= 'f':
			n += int(c-'a') + 10
		default:
			return 0, net.InvalidAddrError("hex")
		}
	}
	return n, nil
}

// routerName asks the routers directly (not the PC's normal DNS, which goes through the merge engine) what
// name they know for this address; routers that hand out addresses keep the name the phone gave them.
func routerName(ip string) string {
	servers := map[string]bool{}
	for _, a := range getAdapters(false) {
		if a.Gateway != "" {
			servers[a.Gateway] = true
		}
		if p := net.ParseIP(a.IP).To4(); p != nil {
			servers[net.IPv4(p[0], p[1], p[2], 1).String()] = true
		}
	}
	if p := net.ParseIP(ip).To4(); p != nil {
		servers[net.IPv4(p[0], p[1], p[2], 1).String()] = true
	}
	for srv := range servers {
		srv := srv
		r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "udp", srv+":53")
		}}
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		names, err := r.LookupAddr(ctx, ip)
		cancel()
		if err == nil && len(names) > 0 {
			h := strings.TrimSuffix(names[0], ".")
			for _, suf := range []string{".lan", ".local", ".home", ".localdomain"} {
				h = strings.TrimSuffix(h, suf)
			}
			if h != "" {
				return h
			}
		}
	}
	return ""
}
