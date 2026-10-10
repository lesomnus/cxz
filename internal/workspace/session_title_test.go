package workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/auxiliary"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSessionTitleIsDurableManualMetadataWithoutAI(t *testing.T) {
	root := t.TempDir()
	c, err := auxiliary.New(root, func(context.Context, auxiliary.Input) (auxiliary.Output, error) {
		t.Error("manual title update executed AI")
		return auxiliary.Output{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Root: root, aux: c}
	title, err := m.SetSessionTitle(t.Context(), "session", "  `My\nnew title`  ")
	if err != nil || title != "My new title" {
		t.Fatal(title, err)
	}
	c.Close()
	restarted, err := auxiliary.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	v, err := restarted.Title("session")
	if err != nil || v.Text != title || !v.Manual || v.Phase != "manual" || v.Status != "completed" || v.Job != "" {
		t.Fatal("manual title lost on restart", v, err)
	}
}

func TestSessionTitleRejectsInvalidInputBeforeCreatingState(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	for _, text := range []string{"", " \n\t`\"' ", strings.Repeat("a", 4097)} {
		if _, err := m.SetSessionTitle(t.Context(), "s", text); status.Code(err) != codes.InvalidArgument {
			t.Fatal("invalid title accepted", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := m.SetSessionTitle(ctx, "s", "title"); err != context.Canceled {
		t.Fatal("ignored cancellation", err)
	}
	if m.aux != nil {
		t.Fatal("invalid input created title state")
	}
}
