package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
)

func TestPreparationProgress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var out bytes.Buffer
	ready := make(chan struct{})
	polls := 0
	err := prepareWithProgress(ctx, &out, func(context.Context) (*api.Project, error) {
		polls++
		if polls == 1 {
			return &api.Project{ProvisionStep: "devcontainer-up"}, nil
		}
		if polls == 2 {
			close(ready)
		}
		return &api.Project{ProvisionStep: "agent-tools"}, nil
	}, func() error {
		select {
		case <-ready:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"configuration checks passed", "image pull/build", "selected agent binary", "workspace ready"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal(out.String())
		}
	}
}
func TestPreparationFailure(t *testing.T) {
	var out bytes.Buffer
	want := errors.New("build failed")
	err := prepareWithProgress(context.Background(), &out, func(context.Context) (*api.Project, error) { t.Fatal("unexpected poll"); return nil, nil }, func() error { return want }, time.Hour)
	if !errors.Is(err, want) || !strings.Contains(out.String(), "preparation failed") || strings.Contains(out.String(), "workspace ready") {
		t.Fatal(err, out.String())
	}
}
