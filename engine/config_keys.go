package engine

import (
	"reflect"
	"strings"
)

// ConfigKeys lists every settings key the engine reads, dotted for nested sections ("tun.route-address").
// NetMerge's tests check that everything NetMerge writes is among them.
func ConfigKeys() []string {
	var out []string
	var walk func(prefix string, t reflect.Type)
	walk = func(prefix string, t reflect.Type) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			p := name
			if prefix != "" {
				p = prefix + "." + name
			}
			out = append(out, p)
			walk(p, f.Type)
		}
	}
	walk("", reflect.TypeOf(Config{}))
	return out
}
