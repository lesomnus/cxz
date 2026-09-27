package main

import (
	"context"
	"fmt"
)

func serverUpdateCommand(ctx context.Context, root, action string) ([]byte, error) {
	return nil, fmt.Errorf("run --server on the Linux installation host")
}
