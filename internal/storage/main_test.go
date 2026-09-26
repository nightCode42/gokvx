package storage_test

import (
	"fmt"
	"os"
	"testing"

	"go.uber.org/goleak"
)

// TestMain verifies that no test leaks a goroutine, such as the log's
// background syncer. When started as the child of the crash test, it runs the
// crash workload instead of the tests.
func TestMain(m *testing.M) {
	if dir := os.Getenv(crashChildEnv); dir != "" {
		fmt.Fprintln(os.Stderr, runCrashChild(dir))
		os.Exit(2)
	}
	goleak.VerifyTestMain(m)
}
