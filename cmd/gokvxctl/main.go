// Command gokvxctl is the command-line client and cluster dashboard for gokvx.
package main

import "fmt"

// Build metadata, injected at link time by the Makefile via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	fmt.Printf("gokvxctl %s (commit %s, built %s): not implemented yet\n", Version, Commit, BuildDate)
}
