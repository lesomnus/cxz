package main

import (
	"context"
	"fmt"
)

func serverUpdateCommand(ctx context.Context, root, action string) ([]byte, error) {
	return nil, fmt.Errorf("run --server on the Linux installation host")
}

func serverRetryRuntime(context.Context, string, string) ([]byte, error) {
	return nil, fmt.Errorf("run runtime recovery retry on the Linux Manager host")
}
