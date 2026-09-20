// Command kvcheck runs the gokvx linearizability verification harness.
package main

import "fmt"

// Build metadata, injected at link time by the Makefile via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	fmt.Printf("kvcheck %s (commit %s, built %s): not implemented yet\n", Version, Commit, BuildDate)
}
