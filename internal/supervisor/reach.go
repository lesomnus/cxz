package supervisor

import (
	"errors"
	"io/fs"
	"syscall"
)

// Unreachable reports an error from Call that means nothing is listening on the
// session's socket. A supervisor that cannot be reached is not a supervisor
// that is busy: there is no agent running, whatever the last recorded state
// says, and treating the two alike let one dead session block every update of
// an installation -- and made `session stop` impossible, because stopping also
// goes through this socket. See cxz#223.
func Unreachable(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, fs.ErrNotExist)
}
