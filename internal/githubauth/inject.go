package githubauth

import (
	"bytes"
	"context"
	"fmt"

	"github.com/lesomnus/cxz/internal/dockerx"
)

const ConfigDir = "/cxz/state/data/gh"

// Only call after validating ownership of the destination project container.
func Inject(ctx context.Context, container, user string, data []byte) error {
	script := `set -eu
umask 077
test ! -L /cxz/state/data
mkdir -p /cxz/state/data/gh
test ! -L /cxz/state/data/gh
chmod 700 /cxz/state/data/gh
tmp=$(mktemp /cxz/state/data/gh/.hosts.XXXXXX)
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
mv -f "$tmp" /cxz/state/data/gh/hosts.yml`
	if err := dockerx.Input(ctx, bytes.NewReader(data), "exec", "-i", "--user", user, container, "sh", "-c", script); err != nil {
		return fmt.Errorf("could not inject project gh credentials")
	}
	return nil
}
