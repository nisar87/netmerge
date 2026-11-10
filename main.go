package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"time"
)

const uiPort = 9196

func main() {
	background := flag.Bool("background", false, "start without opening the window")
	autotest := flag.Bool("autotest", false, "run the speed test automatically after start")
	dev := flag.String("root", "", "root folder (development)")
	virt := flag.Bool("allow-virtual", false, "treat virtual adapters (Hyper-V, VMware) as real connections; for testing in a virtual machine")
	flag.Parse()
	if *virt {
		allowVirtual = true
	}

	exe, _ := os.Executable()
	rootDir = filepath.Dir(exe)
	if *dev != "" {
		rootDir = *dev
	}
	// a hard crash of the Go runtime writes its report here (the window has no console to show it)
	os.MkdirAll(filepath.Join(rootDir, "data"), 0o755)
	crashLog := filepath.Join(rootDir, "data", "go-crash.log")
	if st, err := os.Stat(crashLog); err == nil && st.Size() > 1<<20 { // one line per start adds up over the years
		_ = os.Rename(crashLog, crashLog+".old")
	}
	if cf, err := os.OpenFile(crashLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(cf, "--- start %s ---\n", time.Now().Format("2006-01-02 15:04:05"))
		debug.SetCrashOutput(cf, debug.CrashOptions{})
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/", uiPort)

	// Second launch: just show the window of the running instance.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", uiPort))
	if err != nil {
		// a short timeout: a program that holds the port but never answers must not leave this launch hanging
		if c, err := (&http.Client{Timeout: 3 * time.Second}).Get(url + "api/ping"); err == nil {
			c.Body.Close()
			if !*background {
				openWindow(url)
			}
			return
		}
		messageBox("NetMerge", fmt.Sprintf("Port %d is used by another program.", uiPort))
		return
	}

	if !isAdmin() {
		ln.Close()
		if relaunchAsAdmin() {
			return
		}
		messageBox("NetMerge", "NetMerge needs administrator rights to merge connections.\nPlease click Yes on the prompt.")
		return
	}

	initLog()
	logf("NetMerge starting (folder %s)", rootDir)
	loadSettings()
	syncLinks(getAdapters(true))
	if _, err := os.Stat(filepath.Join(rootDir, "data", ".shortcut")); err != nil {
		createShortcut()
		_ = os.WriteFile(filepath.Join(rootDir, "data", ".shortcut"), []byte("1"), 0o644)
	}

	engine.Adopt()
	if running, _, _, _ := engine.Status(); !running && getSettings().MergeOnLaunch {
		// at Windows start-up the adapters may not be ready yet, so keep trying for a couple of minutes
		go safely("auto start", func() {
			for i := 0; i < 30 && !manualPower.Load(); i++ {
				if r, _, _, _ := engine.Status(); r {
					return
				}
				err := engine.Start()
				if err == nil {
					return
				}
				logf("start on launch failed (try %d): %v", i+1, err)
				time.Sleep(5 * time.Second)
				getAdapters(true)
			}
		})
	} else if running {
		go engine.Apply()
	}
	go safely("stats", statsLoop)
	go safely("filters", blockUpdater)
	go safely("health", healthLoop)
	go safely("dispatch", dispatchLoop)
	go safely("usage", usageSaver)
	if *autotest {
		go func() {
			for i := 0; i < 40; i++ {
				if r, _, _, _ := engine.Status(); r {
					break
				}
				time.Sleep(time.Second)
			}
			time.Sleep(4 * time.Second)
			logf("auto speed test started")
			runSpeedTest()
		}()
	}

	go safely("tray", func() { runTray(url) })
	if v := os.Getenv("NETMERGE_UPDATE_API"); v != "" {
		updateAPI = v
	}
	go safely("updates", updateLoop)
	go safely("providers", providerLoop)
	go safely("history", historyLoop)
	go safely("events", eventsLoop)
	go dnsGuardLoop()
	go presetScheduleLoop()
	if engineMissing() {
		go safely("engine download", downloadEngine)
	}

	srv := &http.Server{Handler: routes(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	if !*background {
		openWindow(url)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	select {
	case <-quitCh:
	case <-sig:
	}
	logf("NetMerge quitting")
	trayRemove()
	usageSave()
	eventsSave()
	historySave()
	engine.Stop()
	_ = srv.Close()
}
