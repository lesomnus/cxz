//go:build linux

package accounts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

func TestClaudeLoginPrivatePTY(t *testing.T) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	defer master.Close()
	if err = unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	if err = unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 100}); err != nil {
		t.Fatal(err)
	}
	before, err := term.GetState(slave.Fd())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runClaudeLogin(ctx, exec.CommandContext(ctx, "sh", "-c", `printf 'Visit https://example.invalid/\n'; IFS= read -r code; test "$code" = 'replacement#state'`), slave, slave)
	}()
	observed := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		var pending, all string
		stages := []struct{ want, send string }{{"[     ]", "private-code#state"}, {"inserted; ctrl+x to clear.", "\x18"}, {"[     ]", "replacement#state\r"}}
		for _, stage := range stages {
			for !strings.Contains(pending, stage.want) {
				n, err := master.Read(buf)
				if err != nil {
					observed <- err
					return
				}
				pending += string(buf[:n])
				all += string(buf[:n])
			}
			pending = ""
			if _, err := master.Write([]byte(stage.send)); err != nil {
				observed <- err
				return
			}
		}
		if strings.Contains(all, "private-code#state") || strings.Contains(all, "replacement#state") {
			observed <- fmt.Errorf("code leaked to terminal")
			return
		}
		observed <- nil
	}()
	select {
	case err := <-observed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("indicator not rendered")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("login did not finish")
	}
	after, err := term.GetState(slave.Fd())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("terminal not restored", err)
	}
}
