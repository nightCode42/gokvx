package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests are white-box: they compare the key catalog derived from the
// Config type with the two documents that describe it, so neither can drift.

const (
	// specPath is the specification, whose Appendix D states the defaults.
	specPath = "../../docs/requirements.md"
	// referencePath is the configuration reference (KV-CFG-004).
	referencePath = "../../docs/configuration.md"
)

// appendixExamples are the keys whose Appendix D values are marked as
// deployment-specific examples rather than defaults.
var appendixExamples = map[string]bool{
	"advertise.client":        true,
	"advertise.peer":          true,
	"auth.jwt.issuer_url":     true,
	"auth.jwt.jwks_url":       true,
	"tls.allowed_peer_sans":   true,
	"tls.allowed_client_sans": true,
	"cluster.initial_peers":   true,
}

// TestDefaultsMatchAppendixD_KV_CFG_020 checks that every key in spec
// Appendix D exists with the stated default, and that the keys marked as
// examples default to empty.
// Verifies: KV-CFG-020.
func TestDefaultsMatchAppendixD_KV_CFG_020(t *testing.T) {
	t.Parallel()

	appendix := loadAppendixD(t)

	codeKeys := make([]string, 0, len(fields))
	for _, f := range fields {
		codeKeys = append(codeKeys, f.key)
	}
	require.ElementsMatch(t, codeKeys, appendix.Keys(), "Appendix D and Config have different keys")

	var stated Config
	require.NoError(t, appendix.Unmarshal("", &stated))
	defaults := Defaults()

	for _, f := range fields {
		got := f.valueOf(&defaults)
		if appendixExamples[f.key] {
			assert.True(t, got.IsZero() || got.Len() == 0, "%s is an example and must default to empty", f.key)
			continue
		}
		assert.Equal(t, f.valueOf(&stated).Interface(), got.Interface(), "default of %s", f.key)
	}
}

// loadAppendixD extracts the YAML block of spec Appendix D and loads it.
func loadAppendixD(t *testing.T) *koanf.Koanf {
	t.Helper()

	spec, err := os.ReadFile(specPath)
	require.NoError(t, err)
	_, appendix, found := strings.Cut(normalize(string(spec)), "## Appendix D")
	require.True(t, found, "Appendix D not found")
	_, block, found := strings.Cut(appendix, "```yaml\n")
	require.True(t, found, "Appendix D has no YAML block")
	block, _, found = strings.Cut(block, "```")
	require.True(t, found, "Appendix D YAML block is not closed")

	k := koanf.New(".")
	require.NoError(t, k.Load(bytesProvider(block), yaml.Parser()))
	return k
}

// bytesProvider is a koanf provider serving YAML held in memory.
type bytesProvider []byte

// ReadBytes returns the YAML for a parser.
func (b bytesProvider) ReadBytes() ([]byte, error) { return b, nil }

// Read is unused: the provider is always paired with a parser.
func (bytesProvider) Read() (map[string]any, error) {
	return nil, errors.New("bytesProvider requires a parser")
}

// TestReferenceDocumentsEveryKey_KV_CFG_004 checks that docs/configuration.md
// lists every key, in declaration order, with its correct type and default.
// Verifies: KV-CFG-004.
func TestReferenceDocumentsEveryKey_KV_CFG_004(t *testing.T) {
	t.Parallel()

	rows := readReference(t)
	defaults := Defaults()

	require.Len(t, rows, len(fields), "the reference must have one row per key")
	for i, f := range fields {
		row := rows[i]
		require.Equal(t, f.key, row[0], "row %d", i+1)
		assert.Equal(t, docType(f), row[1], "type of %s", f.key)
		assert.Equal(t, docDefault(f, &defaults), row[2], "default of %s", f.key)
		assert.NotEmpty(t, row[3], "description of %s", f.key)
	}
}

// readReference returns the cells of each row between the reference markers:
// key, type, default, description, with code formatting removed.
func readReference(t *testing.T) [][4]string {
	t.Helper()

	content, err := os.ReadFile(referencePath)
	require.NoError(t, err)
	_, rest, found := strings.Cut(normalize(string(content)), "<!-- keys:start -->")
	require.True(t, found, "keys start marker missing")
	table, _, found := strings.Cut(rest, "<!-- keys:end -->")
	require.True(t, found, "keys end marker missing")

	var rows [][4]string
	for _, line := range strings.Split(table, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue // header, separator, and section rows
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		require.Len(t, cells, 4, "row %q", line)
		var row [4]string
		for i, c := range cells {
			row[i] = strings.Trim(strings.TrimSpace(c), "`")
		}
		rows = append(rows, row)
	}
	return rows
}

// docType returns the type name the reference uses for a key.
func docType(f field) string {
	switch {
	case f.typ == durationType:
		return "duration"
	case f.isList():
		return "list"
	}
	switch f.typ.Kind() {
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int64, reflect.Uint32:
		return "integer"
	case reflect.Float64:
		return "number"
	default:
		return "string"
	}
}

// docDefault renders a key's default the way the reference writes it: "—"
// for an empty value, lists comma-separated, durations as in configuration.
func docDefault(f field, cfg *Config) string {
	v := f.valueOf(cfg)
	switch {
	case f.typ == durationType:
		return formatDuration(v.Interface().(time.Duration))
	case f.isList():
		if v.Len() == 0 {
			return "—"
		}
		return strings.Join(v.Interface().([]string), ", ")
	case v.Kind() == reflect.String && v.String() == "":
		return "—"
	default:
		return fmtValue(v)
	}
}

// fmtValue renders a scalar value in its natural text form.
func fmtValue(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
	default:
		return fmt.Sprint(v.Interface())
	}
}

// normalize converts CRLF line endings, so the tests read files the same way
// on every platform.
func normalize(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// TestEnvironmentNamesAreUnambiguous checks that no two keys share an
// environment variable, and that no key's variable could be mistaken for one
// Kubernetes injects for a Service.
func TestEnvironmentNamesAreUnambiguous(t *testing.T) {
	t.Parallel()

	assert.Len(t, fieldsByEnv, len(fields), "two keys map to one environment variable")
	for _, f := range fields {
		assert.False(t, kubernetesServiceLink.MatchString(f.envName()), "%s looks like a Service link", f.envName())
	}
}

// TestNoKeyHoldsASecret_KV_CFG_005 checks that no key looks like it holds a
// secret value. Every key is also a flag, and secrets must never be accepted
// as flags; a secret belongs in a file whose path is configured instead
// (docs/configuration.md, Secrets). Adding such a key fails this test by
// design.
// Verifies: KV-CFG-005.
func TestNoKeyHoldsASecret_KV_CFG_005(t *testing.T) {
	t.Parallel()

	secretWords := []string{"password", "secret", "token", "credential", "private", "passphrase", "api_key"}

	for _, f := range fields {
		if strings.HasSuffix(f.key, "_file") {
			continue // a path to a secret, which is how secrets are supplied
		}
		for _, word := range secretWords {
			assert.NotContains(t, f.key, word, "%s looks like a secret value; configure a file path instead", f.key)
		}
	}
}

// TestRegisterFlagsDefinesEveryKey checks that each key has a flag of the same
// name, so every setting can be overridden on the command line.
func TestRegisterFlagsDefinesEveryKey(t *testing.T) {
	t.Parallel()

	fs := pflag.NewFlagSet("gokvx", pflag.ContinueOnError)
	require.NoError(t, RegisterFlags(fs))

	for _, f := range fields {
		assert.NotNil(t, fs.Lookup(f.key), "flag --%s", f.key)
	}
}

// TestFormatDuration checks the compact form used in files and the reference.
func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := map[time.Duration]string{
		0:                                     "0s",
		100 * time.Millisecond:                "100ms",
		20 * time.Second:                      "20s",
		time.Minute:                           "1m",
		90 * time.Second:                      "1m30s",
		15 * time.Minute:                      "15m",
		time.Hour:                             "1h",
		time.Hour + 30*time.Minute:            "1h30m",
		time.Hour + time.Minute + time.Second: "1h1m1s",
	}

	for d, want := range tests {
		assert.Equal(t, want, formatDuration(d), "%v", d)
	}
}
