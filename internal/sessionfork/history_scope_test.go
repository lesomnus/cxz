package sessionfork

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A session profile accumulates every thread it has ever held, so a fork that
// copied the folder whole handed its ancestors' threads to the next generation
// and the pile only grew. These are the shapes seen on the installation: claude
// keeps a transcript with a directory of the same name beside it, codex keeps
// one rollout whose name ends with the thread id.
func TestForkCopiesOnlyTheForkedConversation(t *testing.T) {
	for _, tc := range []struct {
		name, vendor string
		layout       []string
		want         []string
	}{
		{
			name:   "claude",
			vendor: "35478acd-f054-4307-adb1-b404726ab075",
			layout: []string{
				"-workspace/35478acd-f054-4307-adb1-b404726ab075.jsonl",
				"-workspace/35478acd-f054-4307-adb1-b404726ab075/subagents/agent-a1.jsonl",
				"-workspace/35478acd-f054-4307-adb1-b404726ab075/tool-results/r1.txt",
				"-workspace/memory/notes.md",
				"-workspace/6f1d58bd-3d25-4715-b7e8-417147fa5a31.jsonl",
				"-workspace/6f1d58bd-3d25-4715-b7e8-417147fa5a31/subagents/agent-b1.jsonl",
			},
			want: []string{
				"-workspace/35478acd-f054-4307-adb1-b404726ab075.jsonl",
				"-workspace/35478acd-f054-4307-adb1-b404726ab075/subagents/agent-a1.jsonl",
				"-workspace/35478acd-f054-4307-adb1-b404726ab075/tool-results/r1.txt",
				"-workspace/memory/notes.md",
			},
		},
		{
			name:   "codex",
			vendor: "01a1748e-1d6c-71e2-b4bc-d839165a27ff",
			layout: []string{
				"2026/10/25/rollout-2026-10-25T19-33-53-01a1748e-1d6c-71e2-b4bc-d839165a27ff.jsonl",
				"2026/10/25/rollout-2026-10-25T19-35-18-01a1748f-6b3d-7873-b979-e54099efbfaa.jsonl",
				"2026/10/26/rollout-2026-10-26T06-58-20-01a17700-bfae-7e40-9410-c9112f81d6bb.jsonl",
			},
			want: []string{
				"2026/10/25/rollout-2026-10-25T19-33-53-01a1748e-1d6c-71e2-b4bc-d839165a27ff.jsonl",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			source, target := filepath.Join(base, "source"), filepath.Join(base, "target")
			for _, rel := range tc.layout {
				path := filepath.Join(source, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(rel), 0600); err != nil {
					t.Fatal(err)
				}
			}
			native, err := copyHistory(context.Background(), source, target, tc.vendor)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			if err := filepath.WalkDir(target, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				rel, _ := filepath.Rel(target, path)
				got = append(got, filepath.ToSlash(rel))
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			sort.Strings(got)
			sort.Strings(tc.want)
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("copied the wrong set\n got: %v\nwant: %v", got, tc.want)
			}
			// The provider is handed this path to fork from, so it has to be the
			// transcript of the conversation being forked.
			if filepath.Base(native) != filepath.Base(tc.want[0]) && !strings.HasSuffix(native, tc.vendor+".jsonl") {
				t.Fatal("native transcript is not the forked conversation's:", native)
			}
			// Every copied byte survives; the copy is no longer flushed per file.
			for _, rel := range tc.want {
				b, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(rel)))
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != rel {
					t.Fatalf("%s: content %q", rel, b)
				}
			}
		})
	}
}

func TestForkRefusesWhenTheForkedTranscriptIsAbsent(t *testing.T) {
	base := t.TempDir()
	source, target := filepath.Join(base, "source"), filepath.Join(base, "target")
	path := filepath.Join(source, "-workspace", "someone-else.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := copyHistory(context.Background(), source, target, "35478acd"); err == nil {
		t.Fatal("a profile without the forked transcript was accepted")
	}
}
