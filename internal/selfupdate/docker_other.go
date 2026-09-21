//go:build !linux

package selfupdate

import (
	"context"
	"io"
)

func PrepareWithDocker(_ context.Context, target, _ string, _ io.Writer) (*Replacement, error) {
	return Prepare(target)
}
