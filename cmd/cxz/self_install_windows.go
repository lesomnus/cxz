package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"github.com/lesomnus/cxz/internal/selfupdate"
	"github.com/lesomnus/xli"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func selfInstallCommand() *xli.Command {
	return &xli.Command{Name: "self-install", Aliases: []string{"windows-install"}, Brief: "Install this frontend for the current user and register its directory in PATH", Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		programs, err := windows.KnownFolderPath(windows.FOLDERID_UserProgramFiles, windows.KF_FLAG_DONT_VERIFY)
		if err != nil {
			return err
		}
		source, err := os.Executable()
		if err != nil {
			return err
		}
		target := filepath.Join(programs, "cxz", "cxz.exe")
		if err := validPathDirectory(filepath.Dir(target)); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		changed, err := selfupdate.Install(source, target)
		if err != nil {
			return err
		}
		label := "Already installed"
		if changed {
			label = "Installed"
		}
		fmt.Fprintf(c.Writer, "%s: %s\n", label, target)
		environment, _, err := registry.CreateKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("executable installed, but user PATH could not be opened; retry self-install: %w", err)
		}
		defer environment.Close()
		pathChanged, err := addUserPath(environment, filepath.Dir(target))
		if err != nil {
			return fmt.Errorf("executable installed, but user PATH was not updated; retry self-install: %w", err)
		}
		if pathChanged {
			broadcastEnvironmentChange()
			fmt.Fprintln(c.Writer, "Added installation directory to user PATH.")
		} else {
			fmt.Fprintln(c.Writer, "Installation directory is already in user PATH.")
		}
		fmt.Fprintln(c.Writer, "Restart Windows Terminal / your shell to pick up PATH. Existing shells keep their old environment.")
		fmt.Fprintln(c.Writer, "Then: cxz integration add windows-terminal")
		return nil
	})}
}

func validPathDirectory(dir string) error {
	if !filepath.IsAbs(dir) || strings.ContainsAny(dir, ";\x00\r\n\"") {
		return fmt.Errorf("installation directory cannot be represented in PATH: %q", dir)
	}
	return nil
}

// Read the raw user value, not the process's merged system/user PATH. Preserve
// its type, environment references, spelling and ordering when appending.
func addUserPath(environment registry.Key, dir string) (bool, error) {
	if err := validPathDirectory(dir); err != nil {
		return false, err
	}
	path, kind, err := environment.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		path, kind, err = "", registry.EXPAND_SZ, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range strings.Split(path, ";") {
		entry = strings.Trim(strings.TrimSpace(entry), `"`)
		if kind == registry.EXPAND_SZ {
			entry, err = registry.ExpandString(entry)
			if err != nil {
				return false, err
			}
		}
		if strings.EqualFold(filepath.Clean(entry), filepath.Clean(dir)) {
			return false, nil
		}
	}
	updated := path
	if updated != "" && !strings.HasSuffix(updated, ";") {
		updated += ";"
	}
	updated += dir
	if kind == registry.EXPAND_SZ {
		err = environment.SetExpandStringValue("Path", updated)
	} else {
		err = environment.SetStringValue("Path", updated)
	}
	return err == nil, err
}

func broadcastEnvironmentChange() {
	// Notify Explorer and other environment-aware apps. A hung recipient must
	// not block installation, and a child cannot change its parent shell's PATH.
	message, _ := windows.UTF16PtrFromString("Environment")
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	proc.Call(0xffff, 0x001a, 0, uintptr(unsafe.Pointer(message)), 0x0002, 1000, 0)
	runtime.KeepAlive(message)
}
