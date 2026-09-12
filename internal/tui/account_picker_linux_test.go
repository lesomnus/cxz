//go:build linux

package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

func TestAccountPickerTerminalRestoration(t *testing.T) {
	for _, input := range []string{"main\r", "\x03"} {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
			if err != nil {
				t.Skipf("PTY unavailable: %v", err)
			}
			defer master.Close()
			if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
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
			if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 16, Col: 80}); err != nil {
				t.Fatal(err)
			}
			before, err := term.GetState(slave.Fd())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			type result struct {
				alias string
				err   error
			}
			done := make(chan result, 1)
			go func() { alias, err := SelectAccount(ctx, pickerAccounts(), slave, slave); done <- result{alias, err} }()
			ready := make(chan error, 1)
			go func() {
				var output strings.Builder
				buf := make([]byte, 4096)
				for {
					n, err := master.Read(buf)
					if err != nil {
						ready <- err
						return
					}
					output.Write(buf[:n])
					if strings.Contains(output.String(), "Search:") {
						ready <- nil
						return
					}
				}
			}()
			select {
			case err := <-ready:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("picker did not render")
			}
			if _, err := master.Write([]byte(input)); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-done:
				if input == "main\r" && (got.err != nil || got.alias != "main") {
					t.Fatalf("%+v", got)
				}
				if input == "\x03" && !errors.Is(got.err, ErrAccountSelectionCanceled) {
					t.Fatalf("%+v", got)
				}
			case <-ctx.Done():
				t.Fatal("picker did not exit")
			}
			after, err := term.GetState(slave.Fd())
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("terminal state not restored: %v", err)
			}
			pending, err := unix.IoctlGetInt(int(slave.Fd()), unix.TIOCINQ)
			if err != nil || pending != 0 {
				t.Fatalf("input left for shell: %d, %v", pending, err)
			}
		})
	}
}
