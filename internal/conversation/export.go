package conversation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/lesomnus/cxz/internal/core"
)

var exportMu sync.Mutex

func (s *Store) export(ctx context.Context, q Query, r *Reply) error {
	if len(r.Bodies) == 0 {
		return nil
	}
	if !runtimeID.MatchString(s.Caller) {
		return fmt.Errorf("invalid caller identity")
	}
	var data bytes.Buffer
	for _, e := range r.Bodies {
		if err := json.NewEncoder(&data).Encode(e); err != nil {
			return err
		}
	}
	if data.Len() > MaxFileBytes {
		return fmt.Errorf("export exceeds 16 MiB")
	}
	exportMu.Lock()
	defer exportMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	callerDir := filepath.Join("sessions", s.Caller)
	path := filepath.Join(callerDir, "conversation-exports")
	for _, dir := range []string{"sessions", callerDir, path} {
		if err = root.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
			return err
		}
		st, e := root.Lstat(dir)
		if e != nil {
			return e
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("export directory must not be a symlink")
		}
	}
	dir, err := root.OpenRoot(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	directory, err := dir.Open(".")
	if err != nil {
		return err
	}
	entries, err := directory.ReadDir(-1)
	directory.Close()
	if err != nil {
		return err
	}
	type saved struct {
		name string
		size int64
		time int64
	}
	var old []saved
	var total int64
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") || !runtimeID.MatchString(strings.TrimSuffix(name, ".jsonl")) {
			continue
		}
		st, err := dir.Lstat(name)
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("invalid export file")
		}
		size := st.Size()
		if meta, e := dir.Lstat(strings.TrimSuffix(name, ".jsonl") + ".meta.json"); e == nil {
			size += meta.Size()
		}
		old = append(old, saved{name, size, st.ModTime().UnixNano()})
		total += size
	}
	sort.Slice(old, func(i, j int) bool {
		if old[i].time == old[j].time {
			return old[i].name < old[j].name
		}
		return old[i].time < old[j].time
	})
	name := core.ID()
	r.Path = filepath.Join(s.Root, path, name+".jsonl")
	r.MetadataPath = filepath.Join(s.Root, path, name+".meta.json")
	r.FileBytes = data.Len()
	metaReply := *r
	metaReply.Bodies = nil
	meta, err := json.MarshalIndent(struct {
		Reply Reply `json:"result"`
		Query Query `json:"query"`
	}{metaReply, q}, "", "  ")
	if err != nil {
		return err
	}
	needed := int64(data.Len() + len(meta))
	for len(old) > 0 && (len(old) >= 32 || total+needed > 64<<20) {
		victim := old[0]
		if err := dir.Remove(victim.name); err != nil {
			return err
		}
		if err := dir.Remove(strings.TrimSuffix(victim.name, ".jsonl") + ".meta.json"); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= victim.size
		old = old[1:]
	}
	write := func(name string, b []byte) error {
		f, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		if err == nil {
			err = f.Sync()
		}
		ce := f.Close()
		if err == nil {
			err = ce
		}
		if err != nil {
			dir.Remove(name)
		}
		return err
	}
	if err = write(name+".jsonl", data.Bytes()); err != nil {
		return err
	}
	if err = write(name+".meta.json", meta); err != nil {
		dir.Remove(name + ".jsonl")
		return err
	}
	r.Bodies = nil
	return nil
}
