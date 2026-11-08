package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"netmerge/dm"
)

// The optional download manager (package dm) is a separate module. This file is the only place where the product
// talks to it; the manager is off until the person turns it on in Settings.

func setupDownloads() {
	dm.Setup(dm.Host{
		Root:       rootDir,
		Version:    appVersion,
		Lines:      dmLines,
		LineRate:   dmLineRate,
		CallActive: callNow,
		PresetDownloads: func() (string, string) {
			s := getSettings()
			if p, ok := findPreset(s, s.Preset); ok && s.Preset != "" {
				return p.Name, p.Downloads
			}
			return "", ""
		},
		Guard:            func(h http.HandlerFunc) http.HandlerFunc { return guard(h) },
		Log:              logf,
		Note:             func(kind, format string, a ...any) { note(kind, format, a...) },
		Notify:           trayBalloon,
		Power:            dlPower,
		PowerCancel:      dlPowerCancel,
		OpenPath:         openPath,
		FreeSpace:        freeSpace,
		MarkFromInternet: markFromInternet,
		Run: func(args []string) error {
			if len(args) == 0 {
				return errors.New("no command")
			}
			return hiddenCmd(args[0], args[1:]...).Run()
		},
	})
}

var (
	dmCapMu   sync.Mutex
	dmCapAt   time.Time
	dmCapData map[string][2]float64
)

// dmLineRate: the speed the dispatcher has measured for a line (bytes/s), read from its capacity file.
func dmLineRate(name string) float64 {
	dmCapMu.Lock()
	defer dmCapMu.Unlock()
	if time.Since(dmCapAt) > 30*time.Second {
		dmCapAt = time.Now()
		dmCapData = nil
		if b, err := os.ReadFile(capFile()); err == nil {
			_ = json.Unmarshal(b, &dmCapData)
		}
	}
	return dmCapData[capKey(name)][0]
}

// dmLines describes the connections for the download manager. The address and index of an adapter are looked up
// when a connection is opened, not once: Wi-Fi reconnects, DHCP renewals and replugged cables change them in the
// middle of a download.
func dmLines() dm.Lines {
	var res dm.Lines
	s := getSettings()
	ads := map[string]Adapter{}
	for _, a := range getAdapters(false) {
		ads[a.Name] = a
	}
	for _, l := range activeLinks(s) {
		if _, ok := ads[l.Name]; !ok {
			continue
		}
		name := l.Name
		line := dm.Line{Name: name, Kind: l.Kind, Rate: dmLineRate(name),
			Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
				a, ok := adapterByName(name)
				if !ok {
					return nil, errors.New("this connection is not available any more")
				}
				return dialOn(ctx, a, network, addr)
			},
			Alive: func() bool {
				for _, al := range activeLinks(getSettings()) {
					if al.Name == name {
						return !al.Dead
					}
				}
				return false
			}}
		switch {
		case isStandby(l.Link): // a "busy only" or "backup" line stays out of downloads while its role keeps it out
			res.Held = append(res.Held, line)
		case l.Kind == "mobile":
			res.Mobile = append(res.Mobile, line)
		default:
			res.Use = append(res.Use, line)
		}
	}
	return res
}
