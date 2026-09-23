package kverr_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// catalogPath is the public error catalog, relative to this package.
const catalogPath = "../../docs/errors.md"

// catalogRow is one reason as the public catalog documents it.
type catalogRow struct {
	reason    kverr.Reason
	code      string
	retryable bool
	meaning   string
}

// TestErrorCatalogMatchesRegistry_KV_API_092 checks that docs/errors.md lists
// exactly the registered reasons, in order, with the same status code,
// retryability, and description, so the published contract cannot drift from
// the code.
// Verifies: KV-API-091, KV-API-092.
func TestErrorCatalogMatchesRegistry_KV_API_092(t *testing.T) {
	t.Parallel()

	rows := readCatalog(t)

	documented := make([]kverr.Reason, 0, len(rows))
	for _, row := range rows {
		documented = append(documented, row.reason)
	}
	require.Equal(t, kverr.AllReasons(), documented, "catalog reasons differ from the registry")

	for _, row := range rows {
		t.Run(string(row.reason), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, kverr.KindOf(row.reason).String(), row.code, "status code")
			assert.Equal(t, kverr.Retryable(row.reason), row.retryable, "retryable")
			assert.Equal(t, kverr.Description(row.reason), row.meaning, "meaning")
		})
	}
}

// readCatalog parses the table between the catalog markers in docs/errors.md.
func readCatalog(t *testing.T) []catalogRow {
	t.Helper()

	content, err := os.ReadFile(catalogPath)
	require.NoError(t, err)

	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	_, rest, found := strings.Cut(text, "<!-- catalog:start -->")
	require.True(t, found, "catalog start marker missing")
	table, _, found := strings.Cut(rest, "<!-- catalog:end -->")
	require.True(t, found, "catalog end marker missing")

	var rows []catalogRow
	for _, line := range strings.Split(strings.TrimSpace(table), "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue // header and separator rows
		}
		rows = append(rows, parseCatalogRow(t, line))
	}
	require.NotEmpty(t, rows, "catalog has no rows")
	return rows
}

// parseCatalogRow parses one "| `REASON` | `CODE` | yes | Meaning. |" row.
func parseCatalogRow(t *testing.T, line string) catalogRow {
	t.Helper()

	cells := strings.Split(strings.Trim(line, "| "), "|")
	require.Len(t, cells, 4, "catalog row has the wrong number of cells: %q", line)
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}

	retryable := cells[2]
	require.Contains(t, []string{"yes", "no"}, retryable, "retryable must be yes or no: %q", line)

	return catalogRow{
		reason:    kverr.Reason(strings.Trim(cells[0], "`")),
		code:      strings.Trim(cells[1], "`"),
		retryable: retryable == "yes",
		meaning:   cells[3],
	}
}
