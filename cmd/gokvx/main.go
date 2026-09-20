// Command gokvx runs a gokvx key-value store node.
package main

import "fmt"

// Build metadata, injected at link time by the Makefile via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	fmt.Printf("gokvx %s (commit %s, built %s): not implemented yet\n", Version, Commit, BuildDate)
}
