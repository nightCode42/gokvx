// Command microservice1 is a reference REST service that consumes gokvx.
package main

import "fmt"

// Build metadata, injected at link time by the Makefile via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	fmt.Printf("microservice1 %s (commit %s, built %s): not implemented yet\n", Version, Commit, BuildDate)
}
