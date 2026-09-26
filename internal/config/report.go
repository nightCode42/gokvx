package config

import (
	"net/url"
	"reflect"
	"slices"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/v2"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// Warning describes one setting that weakens safety, and what it costs.
type Warning struct {
	// Setting is the key and its value, for example "storage.fsync=os".
	Setting string
	// Consequence says what an operator gives up with this setting.
	Consequence string
}

// String returns the warning as one line: the setting, then its consequence.
func (w Warning) String() string {
	return w.Setting + ": " + w.Consequence
}

// Warnings returns one distinct warning for each setting that weakens safety,
// so that every such setting is announced at startup (KV-CFG-021). A safe
// configuration has none.
func (c *Config) Warnings() []Warning {
	var out []Warning
	if c.Auth.Mode == AuthModeDisabled {
		out = append(out, Warning{
			Setting:     "auth.mode=disabled",
			Consequence: "mutual TLS and token verification are off; any caller that can connect may read and write every key",
		})
	}
	if c.Auth.InsecureAllowRemote {
		out = append(out, Warning{
			Setting:     "auth.insecure_allow_remote=true",
			Consequence: "a node with authentication disabled may accept connections from the network",
		})
	}
	if !c.TLS.Enabled {
		out = append(out, Warning{
			Setting:     "tls.enabled=false",
			Consequence: "traffic is unencrypted and callers are not identified by certificate",
		})
	}
	switch c.Storage.Fsync {
	case FsyncInterval:
		out = append(out, Warning{
			Setting:     "storage.fsync=interval",
			Consequence: "a crash can lose writes acknowledged within the last storage.fsync_interval",
		})
	case FsyncOS:
		out = append(out, Warning{
			Setting:     "storage.fsync=os",
			Consequence: "a crash can lose any acknowledged write the operating system has not flushed",
		})
	case FsyncAlways:
	}
	return out
}

// Redacted returns a copy of the configuration that is safe to log
// (KV-CFG-006). Credentials embedded in URLs, as in https://user:pass@host,
// are replaced; the rest is unchanged. The copy shares no lists with the
// original.
//
// The configuration holds no other secrets: private keys and credentials are
// files, and only their paths appear here (KV-CFG-005).
func (c *Config) Redacted() Config {
	r := c.clone()
	r.Auth.JWT.IssuerURL = redactURL(r.Auth.JWT.IssuerURL)
	r.Auth.JWT.JWKSURL = redactURL(r.Auth.JWT.JWKSURL)
	r.Observability.Tracing.OTLPEndpoint = redactURL(r.Observability.Tracing.OTLPEndpoint)
	return r
}

// clone returns a copy of the configuration whose lists are copies too.
func (c *Config) clone() Config {
	out := *c
	for _, f := range fields {
		if f.isList() {
			list := f.valueOf(&out)
			list.Set(reflect.ValueOf(slices.Clone(list.Interface().([]string))))
		}
	}
	return out
}

// redactURL replaces the user information in a URL with REDACTED. Values
// that are not URLs with user information are returned unchanged.
func redactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.User == nil {
		return s
	}
	u.User = url.User("REDACTED")
	return u.String()
}

// YAML renders the configuration as YAML with durations written as in
// configuration files. It does not redact; call [Config.Redacted] first when
// the output is logged or shown.
func (c *Config) YAML() ([]byte, error) {
	k := koanf.New(".")
	for _, f := range fields {
		value := f.valueOf(c).Interface()
		if f.typ == durationType {
			value = formatDuration(value.(time.Duration))
		}
		if err := k.Set(f.key, value); err != nil {
			return nil, kverr.Wrap(kverr.ReasonInternal, err, "configuration could not be rendered")
		}
	}
	out, err := k.Marshal(yaml.Parser())
	if err != nil {
		return nil, kverr.Wrap(kverr.ReasonInternal, err, "configuration could not be rendered")
	}
	return out, nil
}
