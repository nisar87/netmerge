// nmengine runs NetMerge's engine on its own (for tests and development). NetMerge itself starts the same engine
// as "NetMerge.exe -engine -d <folder>".
package main

import (
	"os"

	"netmerge/engine"
)

func main() { os.Exit(engine.Run(os.Args[1:], os.Stdout)) }
