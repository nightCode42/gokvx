// Command gokvx runs a gokvx key-value store node.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// Build metadata, injected at link time by the Makefile via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Exit codes returned by the process.
const (
	// exitOK means the command succeeded.
	exitOK = 0
	// exitFailure means the command failed, including an invalid
	// configuration (KV-CFG-002).
	exitFailure = 1
)

// main runs the command line and exits with its result.
func main() {
	os.Exit(run(os.Args[1:], os.Environ(), os.Stdout, os.Stderr))
}

// run executes the command line described by args against the given
// environment and output streams, and returns the process exit code. It is
// separate from main so tests can drive the binary in-process.
func run(args, environ []string, stdout, stderr io.Writer) int {
	root := newRootCommand(environ)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if err := root.Execute(); err != nil {
		printError(stderr, err)
		return exitFailure
	}
	return exitOK
}

// printError reports a failed command. For a kverr error it prints the
// message and the underlying cause rather than the internal wrapping chain,
// then lists field violations, such as those of an invalid configuration, one
// per line. Write errors are ignored: there is nowhere left to report them.
func printError(w io.Writer, err error) {
	var kerr *kverr.Error
	if !errors.As(err, &kerr) {
		_, _ = fmt.Fprintf(w, "error: %v\n", err)
		return
	}

	line := "error: " + kerr.Message()
	if cause := kerr.Unwrap(); cause != nil {
		line += ": " + cause.Error()
	}
	_, _ = fmt.Fprintln(w, line)
	for _, v := range kerr.Violations() {
		_, _ = fmt.Fprintf(w, "  - %s: %s\n", v.Field, v.Description)
	}
}
