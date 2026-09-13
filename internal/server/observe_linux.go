//go:build linux

package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Local journal writes wake inotify; no idle history scan or supervisor RPC.
// A cheap PID liveness check also catches SIGKILL with no final journal write.
func observeJournals(ctx context.Context, root string, changed func(string)) error {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), "cxz-journal-watch")
	defer f.Close()
	stop := context.AfterFunc(ctx, func() { f.Close() })
	defer stop()
	base := filepath.Join(root, "sessions")
	rootWD, err := unix.InotifyAddWatch(fd, base, unix.IN_CREATE|unix.IN_MOVED_TO)
	if err != nil {
		return err
	}
	watches := map[int]string{}
	pids := map[string]int{}
	add := func(id string) {
		path := filepath.Join(base, id)
		wd, err := unix.InotifyAddWatch(fd, path, unix.IN_MODIFY|unix.IN_CLOSE_WRITE|unix.IN_MOVED_TO|unix.IN_CREATE|unix.IN_DELETE)
		if err == nil {
			watches[wd] = id
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			add(e.Name())
		}
	}
	frames := make(chan []byte, 1)
	errors := make(chan error, 1)
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, err := f.Read(buf)
			if err != nil {
				errors <- err
				return
			}
			b := append([]byte(nil), buf[:n]...)
			select {
			case frames <- b:
			case <-ctx.Done():
				return
			}
		}
	}()
	dirty := map[string]bool{}
	for _, id := range watches {
		dirty[id] = true
	}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	liveness := time.NewTicker(2 * time.Second)
	defer liveness.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errors:
			if ctx.Err() != nil {
				return nil
			}
			return err
		case b := <-frames:
			for len(b) >= 16 {
				wd := int(int32(binary.NativeEndian.Uint32(b[:4])))
				mask := binary.NativeEndian.Uint32(b[4:8])
				n := int(binary.NativeEndian.Uint32(b[12:16]))
				if n > len(b)-16 {
					break
				}
				name := strings.TrimRight(string(b[16:16+n]), "\x00")
				b = b[16+n:]
				if mask&unix.IN_Q_OVERFLOW != 0 {
					for _, id := range watches {
						dirty[id] = true
					}
					continue
				}
				if wd == rootWD {
					if name != "" && mask&unix.IN_ISDIR != 0 {
						add(name)
						dirty[name] = true
					}
					continue
				}
				if id := watches[wd]; id != "" && (name == "events.jsonl" || name == "pid.json" || name == "session.json") {
					dirty[id] = true
				}
			}
		case <-tick.C:
			for id := range dirty {
				delete(dirty, id)
				changed(id)
				var p struct {
					Supervisor int `json:"supervisor"`
				}
				if b, err := os.ReadFile(filepath.Join(base, id, "pid.json")); err == nil && json.Unmarshal(b, &p) == nil && p.Supervisor > 0 {
					pids[id] = p.Supervisor
				}
			}
		case <-liveness.C:
			for id, pid := range pids {
				if unix.Kill(pid, 0) == unix.ESRCH {
					delete(pids, id)
					changed(id)
				}
			}
		}
	}
}
