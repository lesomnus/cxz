package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

type displayedError struct{ error }

func (e *displayedError) Unwrap() error { return e.error }

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
	cxzupdate.Revision = buildRevision
	if handled, err := updateInternal(os.Args[1:]); handled {
		return err
	}
	if len(os.Args) == 2 && os.Args[1] == "_build-info" {
		return json.NewEncoder(os.Stdout).Encode(cxzupdate.Current())
	}
	state, err := defaultState()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err = newRoot(state).Run(ctx, os.Args[1:])
	var restart *cxzupdate.Restart
	if errors.As(err, &restart) {
		cancel()
		return restartFrontend(restart)
	}
	return err
}
