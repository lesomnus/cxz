package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		var shown *displayedError
		if !errors.As(err, &shown) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func defaultState() (string, error) {
	if state := os.Getenv("CXZ_STATE"); state != "" {
		return state, nil
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "cxz"), nil
}

func run() error {
	state, err := defaultState()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return newRoot(state).Run(ctx, os.Args[1:])
}
