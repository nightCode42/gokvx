package config

import (
	"reflect"
	"slices"
	"strings"
	"time"
)

// envPrefix starts the name of every configuration environment variable.
const envPrefix = "GOKVX_"

// field describes one configuration key: a leaf of the Config struct.
type field struct {
	// key is the dotted path, for example "storage.fsync".
	key string
	// index locates the field in Config for reflect.Value.FieldByIndex.
	index []int
	// typ is the field's Go type.
	typ reflect.Type
}

// durationType is the type of duration settings, written as "20s" or "15m".
var durationType = reflect.TypeFor[time.Duration]()

// fields lists every configuration key in declaration order. It is derived
// from the Config type once and never modified, so the keys used by the file,
// the environment, the flags, and the documentation cannot drift apart.
var fields = collectFields(reflect.TypeFor[Config](), "", nil)

// fieldsByKey indexes fields by key.
var fieldsByKey = indexFields(fields, func(f field) string { return f.key })

// fieldsByEnv indexes fields by environment variable name.
var fieldsByEnv = indexFields(fields, func(f field) string { return f.envName() })

// sections holds every key prefix that names a section, such as "auth.jwt".
var sections = collectSections(fields)

// collectFields walks a struct type and returns its leaves, using the koanf
// tags for key names.
func collectFields(t reflect.Type, prefix string, index []int) []field {
	var out []field
	for i := range t.NumField() {
		sf := t.Field(i)
		key := sf.Tag.Get("koanf")
		if prefix != "" {
			key = prefix + "." + key
		}
		idx := append(slices.Clone(index), i)
		if sf.Type.Kind() == reflect.Struct {
			out = append(out, collectFields(sf.Type, key, idx)...)
			continue
		}
		out = append(out, field{key: key, index: idx, typ: sf.Type})
	}
	return out
}

// indexFields builds a lookup table from fields.
func indexFields(fs []field, name func(field) string) map[string]field {
	m := make(map[string]field, len(fs))
	for _, f := range fs {
		m[name(f)] = f
	}
	return m
}

// collectSections returns every proper prefix of every key.
func collectSections(fs []field) map[string]bool {
	m := make(map[string]bool)
	for _, f := range fs {
		parts := strings.Split(f.key, ".")
		for i := 1; i < len(parts); i++ {
			m[strings.Join(parts[:i], ".")] = true
		}
	}
	return m
}

// envName returns the environment variable that sets the key, for example
// GOKVX_STORAGE_FSYNC for storage.fsync.
func (f field) envName() string {
	return envPrefix + strings.ToUpper(strings.ReplaceAll(f.key, ".", "_"))
}

// isList reports whether the key holds a list of strings.
func (f field) isList() bool {
	return f.typ.Kind() == reflect.Slice
}

// valueOf returns the key's value in cfg.
func (f field) valueOf(cfg *Config) reflect.Value {
	return reflect.ValueOf(cfg).Elem().FieldByIndex(f.index)
}

// formatDuration renders a duration the way it is written in configuration:
// "1m" and "1h" rather than time.Duration's "1m0s" and "1h0m0s".
func formatDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}
