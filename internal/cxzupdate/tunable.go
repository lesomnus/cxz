package cxzupdate

import (
	"os"
	"time"
)

// Tunable reads a duration an installation under test may shorten. The update
// path waits in minutes by design -- a session has to be quiet, a refusal has
// to cool off -- and those waits are the whole cost of verifying it, so they
// are set in one place and can be set to seconds. An unparsable or non-positive
// value is ignored rather than argued with: a bad override must not make an
// installation stop updating. See cxz#224.
func Tunable(name string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(name); ok {
		if d, e := time.ParseDuration(v); e == nil && d > 0 {
			return d
		}
	}
	return def
}
