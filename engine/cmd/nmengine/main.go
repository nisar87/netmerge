// nmengine runs NetMerge's engine on its own (for tests and development). NetMerge itself starts the same engine
// as "NetMerge.exe -engine -d <folder>".
package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"

	"netmerge/engine"
)

func main() {
	// NM_PPROF=127.0.0.1:6060 serves Go's profiler, for finding where time goes (the comparison uses it)
	if a := os.Getenv("NM_PPROF"); a != "" {
		go http.ListenAndServe(a, nil)
	}
	os.Exit(engine.Run(os.Args[1:], os.Stdout))
}
