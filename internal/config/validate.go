package config

import (
	"cmp"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// Validate checks every rule the configuration must satisfy and reports all
// violations at once, each naming its key, so an operator can fix a file in
// one pass (KV-CFG-002). It returns nil for a valid configuration, and
// otherwise an INVALID_ARGUMENT error whose violations list every problem.
func (c *Config) Validate() error {
	var v violations
	c.validateNode(&v)
	c.validateListen(&v)
	c.validateAuth(&v)
	c.validateTLS(&v)
	c.validateLimits(&v)
	c.validateStorage(&v)
	c.validateCluster(&v)
	c.validateObservability(&v)
	return v.err("configuration is invalid")
}

// violations collects the problems found in a configuration.
type violations struct {
	// list holds one entry per problem, in the order found.
	list []kverr.FieldViolation
}

// add records that key violates a rule described by description.
func (v *violations) add(key, description string) {
	v.list = append(v.list, kverr.FieldViolation{Field: key, Description: description})
}

// addf is add with a formatted description.
func (v *violations) addf(key, format string, args ...any) {
	v.add(key, fmt.Sprintf(format, args...))
}

// err returns nil when nothing was recorded, and otherwise an
// INVALID_ARGUMENT error carrying every violation sorted by key.
func (v *violations) err(message string) error {
	if len(v.list) == 0 {
		return nil
	}
	sorted := slices.Clone(v.list)
	slices.SortStableFunc(sorted, func(a, b kverr.FieldViolation) int {
		return strings.Compare(a.Field, b.Field)
	})
	return kverr.New(kverr.ReasonInvalidArgument, message).WithViolations(sorted...)
}

// requireValue records a violation when s is empty.
func requireValue(v *violations, key, s string) {
	if s == "" {
		v.add(key, "is required")
	}
}

// requirePositive records a violation unless x is greater than zero.
func requirePositive[T cmp.Ordered](v *violations, key string, x T) {
	var zero T
	if x <= zero {
		v.add(key, "must be greater than zero")
	}
}

// requireNonNegative records a violation when x is below zero.
func requireNonNegative[T cmp.Ordered](v *violations, key string, x T) {
	var zero T
	if x < zero {
		v.add(key, "must not be negative")
	}
}

// requireOneOf records a violation unless value is one of allowed.
func requireOneOf[T ~string](v *violations, key string, value T, allowed ...T) {
	if slices.Contains(allowed, value) {
		return
	}
	names := make([]string, len(allowed))
	for i, a := range allowed {
		names[i] = string(a)
	}
	v.addf(key, "must be one of %s (got %q)", strings.Join(names, ", "), value)
}

// requireHostPort records a violation unless addr is a host:port address
// with a numeric port. The host may be empty, meaning every interface.
func requireHostPort(v *violations, key, addr string) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		v.add(key, "must be an address of the form host:port")
		return
	}
	if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
		v.add(key, "must have a port between 1 and 65535")
	}
}

// requireHTTPSURL records a violation unless s is an absolute HTTPS URL. The
// value itself is not echoed, because a URL can carry credentials.
func requireHTTPSURL(v *violations, key, s string) {
	if s == "" {
		v.add(key, "is required")
		return
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		v.add(key, "must be an absolute https:// URL")
	}
}

// isLoopback reports whether addr binds only to the loopback interface. An
// empty host binds to every interface and is therefore not loopback.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
