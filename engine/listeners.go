package engine

import (
	"fmt"
	"io"
	"net"
	"strconv"
)

// applyListeners opens the API and sharing ports the settings ask for. A listener whose address and settings did
// not change keeps running (phones stay connected through a reload).
func (e *Engine) applyListeners(c *Config) error {
	if err := e.startAPI(c.ExternalController, c.Secret); err != nil {
		return fmt.Errorf("control API on %s: %w", c.ExternalController, err)
	}
	want := map[string]func() (io.Closer, error){}
	if c.MixedPort > 0 {
		bind := "127.0.0.1"
		if c.AllowLAN {
			bind = c.BindAddress
			if bind == "" || bind == "*" {
				bind = ""
			}
		}
		addr := net.JoinHostPort(bind, strconv.Itoa(c.MixedPort))
		want["mixed|"+addr] = func() (io.Closer, error) { return e.startMixed(addr) }
	}
	for _, lc := range c.Listeners {
		lc := lc
		switch lc.Type {
		case "shadowsocks":
			key := fmt.Sprintf("ss|%s|%d|%s|%s|%v", lc.Listen, lc.Port, lc.Cipher, lc.Password, lc.UDP)
			want[key] = func() (io.Closer, error) { return e.startSS(lc) }
		default:
			e.log.Warnf("listener %s: type %q is not supported", lc.Name, lc.Type)
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, cl := range e.inbounds {
		if _, keep := want[k]; !keep {
			_ = cl.Close()
			delete(e.inbounds, k)
		}
	}
	for k, open := range want {
		if _, ok := e.inbounds[k]; ok {
			continue
		}
		cl, err := open()
		if err != nil {
			return fmt.Errorf("sharing port (%s): %w", k, err)
		}
		e.inbounds[k] = cl
	}
	return nil
}
