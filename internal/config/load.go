package config

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// Options selects the sources [Load] reads on top of the built-in defaults.
// Each source is optional.
type Options struct {
	// File is the path of a YAML configuration file; empty means none.
	File string
	// Environ is the process environment as "NAME=value" pairs, usually
	// os.Environ(). Only GOKVX_* variables are read.
	Environ []string
	// Flags is a flag set on which [RegisterFlags] defined the configuration
	// flags. Only flags explicitly set on the command line are applied.
	Flags *pflag.FlagSet
}

// kubernetesServiceLink matches the variables Kubernetes injects for every
// Service in a namespace, such as GOKVX_CLIENT_SERVICE_HOST or
// GOKVX_CLIENT_PORT_2379_TCP. They share the GOKVX_ prefix with configuration
// variables but are not configuration, so they are ignored rather than
// rejected; otherwise every pod would fail to start next to its own Services.
var kubernetesServiceLink = regexp.MustCompile(
	`^[A-Z0-9_]+_(SERVICE_HOST|SERVICE_PORT(_[A-Z0-9_]+)?|PORT(_[0-9]+_(TCP|UDP|SCTP)(_(PROTO|PORT|ADDR))?)?)$`)

// Load assembles a configuration from the defaults, then the file, then the
// environment, then the flags, each overriding the ones before (KV-CFG-001).
//
// It fails with INVALID_ARGUMENT, listing every offending key, when a source
// contains a key or GOKVX_* variable that does not exist (KV-CFG-020), and
// when a value has the wrong type. Load does not check the rules between
// values; call [Config.Validate] for that.
func Load(opts Options) (Config, error) {
	k := koanf.New(".")
	if err := setDefaults(k); err != nil {
		return Config{}, err
	}

	var v violations
	if opts.File != "" {
		if err := loadFile(k, opts.File, &v); err != nil {
			return Config{}, err
		}
	}
	if err := loadEnv(k, opts.Environ, &v); err != nil {
		return Config{}, err
	}
	if opts.Flags != nil {
		if err := loadFlags(k, opts.Flags); err != nil {
			return Config{}, err
		}
	}
	if err := v.err("configuration contains unknown keys"); err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, kverr.Wrap(kverr.ReasonInvalidArgument, err, "configuration has a value of the wrong type")
	}
	return cfg, nil
}

// setDefaults loads every default into k, key by key.
func setDefaults(k *koanf.Koanf) error {
	defaults := Defaults()
	for _, f := range fields {
		if err := set(k, f.key, f.valueOf(&defaults).Interface()); err != nil {
			return err
		}
	}
	return nil
}

// loadFile applies the keys of a YAML file to k. Unknown keys, and values
// given where a section is expected, are recorded in v. An empty section,
// such as a bare "storage:" line, is ignored rather than erasing its defaults.
func loadFile(k *koanf.Koanf, path string, v *violations) error {
	src := koanf.New(".")
	if err := src.Load(file.Provider(path), yaml.Parser()); err != nil {
		return kverr.Wrap(kverr.ReasonInvalidArgument, err, "configuration file could not be read")
	}

	for _, key := range src.Keys() {
		value := src.Get(key)
		switch {
		case isKey(key):
			if err := set(k, key, value); err != nil {
				return err
			}
		case sections[key] && isEmptySection(value):
			// A section without keys changes nothing.
		case sections[key]:
			v.add(key, "is a section and takes nested keys, not a value")
		default:
			v.add(key, "is not a configuration key")
		}
	}
	return nil
}

// isKey reports whether key is a configuration key.
func isKey(key string) bool {
	_, ok := fieldsByKey[key]
	return ok
}

// isEmptySection reports whether a YAML value is an empty section: null or
// an empty mapping.
func isEmptySection(value any) bool {
	if value == nil {
		return true
	}
	m, ok := value.(map[string]any)
	return ok && len(m) == 0
}

// loadEnv applies GOKVX_* variables to k. Unknown GOKVX_* variables are
// recorded in v, except those Kubernetes injects for Services. A list is
// written as comma-separated values.
func loadEnv(k *koanf.Koanf, environ []string, v *violations) error {
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, envPrefix) {
			continue
		}
		f, ok := fieldsByEnv[name]
		if !ok {
			if !kubernetesServiceLink.MatchString(name) {
				v.add(name, "is not a configuration variable")
			}
			continue
		}
		if err := set(k, f.key, parseEnvValue(f, value)); err != nil {
			return err
		}
	}
	return nil
}

// parseEnvValue converts an environment variable's text to the form the
// decoder expects: a list for list keys, the text itself otherwise.
func parseEnvValue(f field, value string) any {
	if !f.isList() {
		return value
	}
	items := []string{}
	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// loadFlags applies the configuration flags that were explicitly set.
func loadFlags(k *koanf.Koanf, fs *pflag.FlagSet) error {
	var err error
	fs.Visit(func(fl *pflag.Flag) {
		f, ok := fieldsByKey[fl.Name]
		if !ok || err != nil {
			return // not a configuration flag, such as --config
		}
		var value any = fl.Value.String()
		if list, isList := fl.Value.(pflag.SliceValue); isList {
			value = list.GetSlice()
		}
		err = set(k, f.key, value)
	})
	return err
}

// set stores one key in k.
func set(k *koanf.Koanf, key string, value any) error {
	if err := k.Set(key, value); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "configuration key could not be set: "+key)
	}
	return nil
}

// RegisterFlags defines one flag per configuration key on fs, named after
// the key: --storage.fsync, --auth.jwt.audience, and so on. Each flag's
// default is the built-in default, and only flags set explicitly override
// the file and the environment.
func RegisterFlags(fs *pflag.FlagSet) error {
	defaults := Defaults()
	for _, f := range fields {
		usage := fmt.Sprintf("sets %s (environment: %s)", f.key, f.envName())
		if err := defineFlag(fs, f, f.valueOf(&defaults), usage); err != nil {
			return err
		}
	}
	return nil
}

// defineFlag defines the flag for one key, typed after the key.
func defineFlag(fs *pflag.FlagSet, f field, def reflect.Value, usage string) error {
	switch {
	case f.typ == durationType:
		fs.Duration(f.key, def.Interface().(time.Duration), usage)
		return nil
	case f.isList():
		fs.StringSlice(f.key, def.Interface().([]string), usage)
		return nil
	}

	switch f.typ.Kind() {
	case reflect.String:
		fs.String(f.key, def.String(), usage)
	case reflect.Bool:
		fs.Bool(f.key, def.Bool(), usage)
	case reflect.Int:
		fs.Int(f.key, def.Interface().(int), usage)
	case reflect.Int64:
		fs.Int64(f.key, def.Int(), usage)
	case reflect.Uint32:
		fs.Uint32(f.key, def.Interface().(uint32), usage)
	case reflect.Float64:
		fs.Float64(f.key, def.Float(), usage)
	default:
		return kverr.Newf(kverr.ReasonInternal, "configuration key %s has unsupported type %s", f.key, f.typ)
	}
	return nil
}
