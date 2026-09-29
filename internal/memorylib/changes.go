package memorylib

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/lesomnus/cxz/internal/core"
)

const changeIndexFile = ".changes.json"
const maxChangeIndex = 128 * 1024 * 1024
const maxTombstones = 4096

// Change describes the latest observed state of a memory or document, never its body.
// Document is empty for memory metadata. Created and updated both mean upsert.
type Change struct {
	Sequence uint64 `json:"sequence"`
	Kind     string `json:"kind"`
	MemoryID string `json:"memory_id"`
	Document string `json:"document,omitempty"`
	Name     string `json:"name,omitempty"`
	Revision string `json:"revision"`
}

type changeIndex struct {
	Version  int               `json:"version"`
	Epoch    string            `json:"epoch"`
	Sequence uint64            `json:"sequence"`
	Floor    uint64            `json:"floor"`
	Items    map[string]Change `json:"items"`
}

type changeCursor struct {
	Version  int    `json:"v"`
	Project  string `json:"project"`
	Epoch    string `json:"epoch"`
	Sequence uint64 `json:"sequence"`
	Baseline bool   `json:"baseline,omitempty"`
	After    string `json:"after,omitempty"`
}

func encodeCursor(c changeCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(raw string) (changeCursor, error) {
	var c changeCursor
	if len(raw) > 2048 {
		return c, fmt.Errorf("invalid memory changes cursor")
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return c, fmt.Errorf("invalid memory changes cursor")
	}
	if err := json.Unmarshal(b, &c); err != nil || c.Version != 1 || c.Epoch == "" || c.Project == "" || (!c.Baseline && c.After != "") {
		return c, fmt.Errorf("invalid memory changes cursor")
	}
	return c, nil
}

// observeChanges runs under the project lock. Scanning content hashes also detects
// host edits with unchanged mtimes. Failed scans never publish a partial index.
func (s *Store) observeChanges(ctx context.Context, r *os.Root) (*changeIndex, error) {
	idx := &changeIndex{Version: 1, Epoch: core.ID(), Items: make(map[string]Change)}
	b, err := read(r, changeIndexFile, maxChangeIndex)
	fresh := errors.Is(err, os.ErrNotExist)
	if err != nil && !fresh {
		return nil, err
	}
	if !fresh {
		if err := json.Unmarshal(b, idx); err != nil {
			return nil, fmt.Errorf("read memory change index: %w", err)
		}
		if idx.Version != 1 || idx.Epoch == "" || idx.Items == nil || idx.Floor > idx.Sequence {
			return nil, fmt.Errorf("invalid memory change index")
		}
	}
	current := make(map[string]Change)
	es, err := entries(r, ".", MaxMemories+5)
	if err != nil {
		return nil, err
	}
	for _, ent := range es {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := ent.Name()
		if !ent.IsDir() || !validID.MatchString(id) {
			continue
		}
		m, err := load(r, id)
		if err != nil {
			return nil, err
		}
		manifest, err := read(r, id+"/manifest.json", 65536)
		if err != nil {
			return nil, err
		}
		current[id] = Change{MemoryID: id, Name: m.Name, Revision: revision(manifest)}
		ds, err := docs(r, id, false)
		if err != nil {
			return nil, err
		}
		for _, d := range ds {
			current[id+"/"+d.Name] = Change{MemoryID: id, Document: d.Name, Revision: d.Revision}
		}
	}
	keys := make([]string, 0, len(current)+len(idx.Items))
	for k := range current {
		keys = append(keys, k)
	}
	for k, old := range idx.Items {
		if _, ok := current[k]; !ok && old.Kind != "deleted" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	changed := fresh
	for _, k := range keys {
		old, existed := idx.Items[k]
		next, live := current[k]
		if live && existed && old.Kind != "deleted" && old.Revision == next.Revision {
			continue
		}
		switch {
		case !live:
			next = old
			next.Kind = "deleted"
		case !existed || old.Kind == "deleted":
			next.Kind = "created"
		default:
			next.Kind = "updated"
		}
		idx.Sequence++
		next.Sequence = idx.Sequence
		idx.Items[k] = next
		changed = true
	}
	idx.prune(maxTombstones)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if changed {
		b, err := json.Marshal(idx)
		if err != nil {
			return nil, err
		}
		if len(b) > maxChangeIndex {
			return nil, fmt.Errorf("memory change index exceeds size limit")
		}
		if err := write(r, changeIndexFile, b); err != nil {
			return nil, err
		}
	}
	return idx, nil
}

func (idx *changeIndex) prune(limit int) {
	var keys []string
	for k, v := range idx.Items {
		if v.Kind == "deleted" {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return idx.Items[keys[i]].Sequence < idx.Items[keys[j]].Sequence })
	for i := 0; i < len(keys)-limit; i++ {
		seq := idx.Items[keys[i]].Sequence
		if seq > idx.Floor {
			idx.Floor = seq
		}
		delete(idx.Items, keys[i])
	}
}

func (s *Store) changes(ctx context.Context, r *os.Root, q Request) (Reply, error) {
	out := Reply{Location: s.root}
	limit := q.Limit
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 500 {
		return out, fmt.Errorf("memory changes limit must be between 1 and 500")
	}
	c := changeCursor{Version: 1, Project: key(s.Session.ProjectID), Baseline: true}
	if q.Cursor != "" {
		var err error
		c, err = decodeCursor(q.Cursor)
		if err != nil {
			return out, err
		}
		if c.Project != key(s.Session.ProjectID) {
			return out, fmt.Errorf("memory changes cursor belongs to another project")
		}
	}
	idx, err := s.observeChanges(ctx, r)
	if err != nil {
		return out, err
	}
	if q.Cursor == "" {
		c.Epoch = idx.Epoch
		c.Sequence = idx.Sequence
	}
	if c.Epoch != idx.Epoch || c.Sequence < idx.Floor {
		out.ResetRequired = true
		out.Message = "Memory changes cursor expired; discard cached metadata and call memory_changes without a cursor."
		return out, nil
	}
	if c.Sequence > idx.Sequence {
		return out, fmt.Errorf("memory changes cursor is ahead of the index")
	}
	out.Baseline = c.Baseline
	var keys []string
	for k, v := range idx.Items {
		if c.Baseline {
			if k > c.After && v.Kind != "deleted" && v.Sequence <= c.Sequence {
				keys = append(keys, k)
			}
		} else if v.Sequence > c.Sequence {
			keys = append(keys, k)
		}
	}
	if c.Baseline {
		sort.Strings(keys)
	} else {
		sort.Slice(keys, func(i, j int) bool { return idx.Items[keys[i]].Sequence < idx.Items[keys[j]].Sequence })
	}
	more := len(keys) > limit
	if more {
		keys = keys[:limit]
	}
	for _, k := range keys {
		v := idx.Items[k]
		if c.Baseline {
			v.Kind = "created"
			c.After = k
		} else {
			c.Sequence = v.Sequence
		}
		out.Changes = append(out.Changes, v)
	}
	if c.Baseline && !more {
		// Changes observed during baseline pagination are drained after its watermark.
		c.Baseline = false
		c.After = ""
		out.HasMore = idx.Sequence > c.Sequence
	} else {
		out.HasMore = more
		if !c.Baseline && !more {
			c.Sequence = idx.Sequence
		}
	}
	out.Cursor = encodeCursor(c)
	return out, nil
}
