package shard_test

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/shard"
)

// update rewrites the golden file from the current slot function. Use it only
// with the maintainer's approval: a changed golden file means the slot
// function changed, which misroutes every stored key (internal/shard/AGENTS.md).
var update = flag.Bool("update", false, "rewrite testdata/slots.golden")

// goldenPath is the committed record of the slot function's output.
const goldenPath = "testdata/slots.golden"

// goldenKeys are the keys whose slots are locked. They cover ordinary keys,
// every brace-rule edge case, and binary keys.
var goldenKeys = []string{
	"",
	"a",
	"0",
	"/app/config",
	"/app/config/",
	"key",
	"Key",
	"KEY",
	"123456789",
	"user:1000",
	"users/1",
	"users/2",
	"{tenant-42}/users/1",
	"{tenant-42}/users/2",
	"tenant-42",
	"app/{tenant-42}/settings",
	"{}",
	"{}/x",
	"{}{a}",
	"{",
	"}",
	"{a",
	"a}",
	"}{",
	"a}b{c",
	"a}{b}",
	"{{a}}",
	"{a}{b}",
	"{a}",
	"{ }",
	"{x}",
	"{long-partition-key-with-many-characters}/suffix",
	"\x00",
	"\xff",
	"\x00\x01\x02\x03",
	"\xff\xfe\xfd\xfc",
	"\x00{\xff\x00}\x01",
	"\xc3\x28",
	"\xc3\x28{k}",
	"ключ",
	"{ключ}/значение",
	"键/值",
	"emoji/\U0001F511",
	strings.Repeat("k", 1024),
	"{" + strings.Repeat("p", 256) + "}/tail",
}

// TestSlotGolden_QA_006 checks the slot of every golden key against the
// committed file, so any change to the slot function fails the build.
// Verifies: QA-006, KV-DAT-021, KV-DAT-022.
func TestSlotGolden_QA_006(t *testing.T) {
	s, err := shard.NewSlotter(shard.DefaultSlotCount)
	require.NoError(t, err)

	if *update {
		writeGolden(t, s)
	}

	entries := readGolden(t)
	require.Len(t, entries, len(goldenKeys), "golden file and goldenKeys differ; review before running with -update")

	for i, e := range entries {
		require.Equal(t, goldenKeys[i], e.key, "golden file entry %d", i)
		assert.Equal(t, e.slot, s.Slot([]byte(e.key)), "slot of %q changed", e.key)
	}
}

// goldenEntry is one line of the golden file.
type goldenEntry struct {
	key  string
	slot uint32
}

// writeGolden rewrites the golden file with the current slot of every key.
func writeGolden(t *testing.T, s shard.Slotter) {
	t.Helper()

	var buf bytes.Buffer
	buf.WriteString("# Slot of each key with 16384 slots. Format: Go-quoted key, space, slot.\n")
	buf.WriteString("# Locked by TestSlotGolden_QA_006. Never regenerate without approval.\n")
	for _, key := range goldenKeys {
		fmt.Fprintf(&buf, "%s %d\n", strconv.Quote(key), s.Slot([]byte(key)))
	}
	require.NoError(t, os.WriteFile(goldenPath, buf.Bytes(), 0o600))
}

// readGolden parses the golden file, skipping comment lines.
func readGolden(t *testing.T) []goldenEntry {
	t.Helper()

	f, err := os.Open(goldenPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	var entries []goldenEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entries = append(entries, parseGoldenLine(t, line))
	}
	require.NoError(t, scanner.Err())
	return entries
}

// parseGoldenLine parses one `"key" slot` line.
func parseGoldenLine(t *testing.T, line string) goldenEntry {
	t.Helper()

	quoted, err := strconv.QuotedPrefix(line)
	require.NoError(t, err, "line %q", line)
	key, err := strconv.Unquote(quoted)
	require.NoError(t, err, "line %q", line)

	slot, err := strconv.ParseUint(strings.TrimSpace(line[len(quoted):]), 10, 32)
	require.NoError(t, err, "line %q", line)

	return goldenEntry{key: key, slot: uint32(slot)}
}
