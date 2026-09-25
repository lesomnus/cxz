package notification

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func TestPlaybackAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name, platform, ssh string
		missing, fail       bool
		want                bool
		calls               []string
	}{
		{name: "mac", platform: "darwin", want: true, calls: []string{"afplay"}},
		{name: "linux", platform: "linux", want: true, calls: []string{"paplay"}},
		{name: "windows", platform: "windows", want: true, calls: []string{"powershell.exe"}},
		{name: "linux playback failure", platform: "linux", fail: true, calls: []string{"paplay", "aplay"}},
		{name: "missing tools", platform: "linux", missing: true},
		{name: "unsupported", platform: "other"},
		{name: "ssh connection", platform: "linux", ssh: "SSH_CONNECTION"},
		{name: "ssh client", platform: "darwin", ssh: "SSH_CLIENT"},
		{name: "ssh tty", platform: "windows", ssh: "SSH_TTY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			var filename string
			got := play(context.Background(), Complete, tc.platform, func(key string) string {
				if key == tc.ssh {
					return "remote"
				}
				return ""
			}, func(name string) (string, error) {
				if tc.ssh != "" {
					t.Fatal("SSH attempted audio lookup")
				}
				if tc.missing {
					return "", exec.ErrNotFound
				}
				return name, nil
			}, func(ctx context.Context, name string, args ...string) error {
				calls = append(calls, name)
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("unbounded playback")
				}
				if tc.platform != "windows" {
					filename = args[0]
					data, err := os.ReadFile(filename)
					if err != nil || !bytes.Equal(data, complete) {
						t.Fatal("player did not receive embedded WAV", err)
					}
				}
				if tc.fail {
					return errors.New("no audio device")
				}
				return nil
			})
			if got != tc.want || !reflect.DeepEqual(calls, tc.calls) {
				t.Fatalf("got %v, %v", got, calls)
			}
			if filename != "" {
				if _, err := os.Stat(filename); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("temporary WAV retained", err)
				}
			}
		})
	}
}

func TestLinuxTriesAvailableBackend(t *testing.T) {
	for _, missing := range []bool{true, false} {
		var calls []string
		ok := play(context.Background(), Attention, "linux", func(string) string { return "" }, func(name string) (string, error) {
			if name == "paplay" && missing {
				return "", exec.ErrNotFound
			}
			return name, nil
		}, func(_ context.Context, name string, args ...string) error {
			calls = append(calls, name)
			if name == "paplay" {
				return errors.New("server unavailable")
			}
			data, err := os.ReadFile(args[0])
			if err != nil || !bytes.Equal(data, attention) {
				t.Fatal("wrong attention sound", err)
			}
			return nil
		})
		if !ok || calls[len(calls)-1] != "aplay" {
			t.Fatal("ALSA fallback missing", calls)
		}
	}
}

func TestPlaybackTimeoutAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	calls := 0
	playAudio := func() bool {
		return play(ctx, Complete, "linux", func(string) string { return "" }, func(s string) (string, error) { return s, nil }, func(ctx context.Context, _ string, _ ...string) error {
			calls++
			<-ctx.Done()
			return ctx.Err()
		})
	}
	if playAudio() || calls != 1 {
		t.Fatal("timed-out playback succeeded or started another tool")
	}
	if playAudio() || calls != 1 {
		t.Fatal("cancelled playback started")
	}
}
