// Package sessionfork snapshots a settled session into an independent profile.
// The caller holds either the live supervisor mutex or its offline lease.
package sessionfork

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
)

type Request struct {
	ID       string `json:"id"`
	ClientID string `json:"client_id"`
	RunID    string `json:"run_id"`
}

var runtimeID = regexp.MustCompile(`^[a-f0-9]{24}$`)

func Prepare(ctx context.Context, root string, source core.Session, snapshot core.Snapshot, events []core.Event, request Request) (result core.Session, err error) {
	if !runtimeID.MatchString(request.ID) || request.ClientID == "" || len(request.ClientID) > 1024 {
		return result, fmt.Errorf("invalid fork identity")
	}
	if request.RunID != snapshot.RunID {
		return result, fmt.Errorf("stale run_id")
	}
	switch snapshot.State {
	case "idle", "stopped", "interrupted", "failed":
	default:
		return result, fmt.Errorf("fork requires a settled conversation")
	}
	if len(snapshot.Pending) > 0 || snapshot.Queued != "" {
		return result, fmt.Errorf("resolve pending questions and queued input before forking")
	}
	if source.Kind != "codex" && source.Kind != "claude" {
		return result, fmt.Errorf("fork is unsupported for this agent")
	}
	if snapshot.VendorID == "" {
		for _, event := range events {
			if event.Kind == "input" {
				return result, fmt.Errorf("provider conversation identity is unavailable")
			}
		}
	}
	target := source
	target.ID, target.CreateID, target.CreatedAt, target.Supervisor = request.ID, request.ClientID, time.Now().UnixMilli(), ""
	target.Fork = &core.ForkOrigin{SessionID: source.ID, RunID: snapshot.RunID, VendorID: snapshot.VendorID, ThroughSeq: snapshot.LastSeq}
	backend, err := accounts.ResolveBinding(target.Kind, target.AuthBackend, target.ProjectID, target.Account, target.AuthBinding)
	if err != nil {
		return result, err
	}
	if err = backend.Check(accounts.AuthRoot(root, target), target.Account); err != nil {
		return result, err
	}
	dir := core.Dir(root, target.ID)
	if err = os.Mkdir(dir, 0700); err != nil {
		return result, err
	}
	profile := accounts.SessionRoot(root, target.CreateID)
	copiedHistory := ""
	profileCreated := true
	if err = os.Mkdir(profile, 0700); os.IsExist(err) {
		profileCreated = false
	} else if err != nil {
		os.RemoveAll(dir)
		return result, fmt.Errorf("fork profile already exists or cannot be created: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
			if copiedHistory != "" {
				os.RemoveAll(copiedHistory)
			}
			if profileCreated {
				os.RemoveAll(profile)
			}
		}
	}()
	if err = accounts.Prepare(profile, target.Account, target.Kind); err != nil {
		return result, err
	}
	if snapshot.VendorID != "" {
		folder := "projects"
		if source.Kind == "codex" {
			folder = "sessions"
		}
		sourceConfig := accounts.Config(accounts.SessionRoot(root, source.CreateID), source.Account)
		targetConfig := accounts.Config(profile, target.Account)
		destination := filepath.Join(targetConfig, folder)
		if _, err = os.Lstat(destination); !os.IsNotExist(err) {
			return result, fmt.Errorf("fork target already contains provider history")
		}
		copiedHistory = destination
		var native string
		native, err = copyHistory(ctx, filepath.Join(sourceConfig, folder), destination, snapshot.VendorID)
		if err != nil {
			return result, err
		}
		target.Fork.NativePath = native
	}
	// Preserve sequence numbers, timestamps and response references. The boundary
	// clears inherited control receipts/vendor IDs without rewriting the display
	// history or replaying any tool. Settings are restored separately below it.
	file, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result, err
	}
	defer file.Close()
	encode := json.NewEncoder(file)
	for _, event := range events {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		event.SessionID = target.ID
		// Vendor identity belongs to the child. Clear old control identities even
		// for readers that predate the fork boundary, so they cannot resume parent.
		if event.Kind == "vendor" {
			event.Text = ""
		}
		if event.Kind == core.HistoryCheckpointKind {
			var checkpoint core.HistoryCheckpoint
			if err = json.Unmarshal(event.Payload, &checkpoint); err != nil {
				return result, err
			}
			checkpoint.Snapshot.VendorID = ""
			event.Payload, err = json.Marshal(checkpoint)
			if err != nil {
				return result, err
			}
		}
		if err = encode.Encode(event); err != nil {
			return result, err
		}
	}
	last := snapshot.LastSeq
	appendEvent := func(kind, text string, payload json.RawMessage) error {
		last++
		return encode.Encode(core.Event{SessionID: target.ID, Seq: last, TimeMS: target.CreatedAt, Kind: kind, Text: text, Payload: payload})
	}
	origin, _ := json.Marshal(target.Fork)
	if err = appendEvent(core.ForkBoundaryKind, "", origin); err != nil {
		return result, err
	}
	// Checkpoints can retain the most recent settings even after old display
	// history has been trimmed. Only preferences cross the control boundary.
	settings := map[string]core.Event{}
	var remember func(core.Event)
	remember = func(event core.Event) {
		if event.Kind == core.HistoryCheckpointKind {
			var checkpoint core.HistoryCheckpoint
			if json.Unmarshal(event.Payload, &checkpoint) == nil {
				for _, saved := range checkpoint.Resume {
					remember(saved)
				}
			}
		}
		if event.Kind == "setting" {
			settings[event.Text] = event
		}
	}
	for _, event := range events {
		remember(event)
	}
	for _, name := range []string{"model", "effort"} {
		if setting, ok := settings[name]; ok {
			if err = appendEvent("setting", name, setting.Payload); err != nil {
				return result, err
			}
			if name == "model" {
				var value struct{ Value string }
				if json.Unmarshal(setting.Payload, &value) == nil {
					target.Model = value.Value
				}
			}
		}
	}
	if err = appendEvent("permission", snapshot.PermissionMode, nil); err != nil {
		return result, err
	}
	if err = file.Sync(); err != nil {
		return result, err
	}
	if err = file.Close(); err != nil {
		return result, err
	}
	if err = core.WriteJSON(filepath.Join(dir, "session.json"), target); err != nil {
		return result, err
	}
	return target, core.SyncDir(filepath.Join(root, "sessions"))
}

func copyHistory(ctx context.Context, source, target, vendor string) (string, error) {
	var native string
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("provider history contains a symbolic link")
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("provider history contains a special file")
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, contextReader{ctx, in})
		if err == nil {
			err = out.Sync()
		}
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if entry.Name() == vendor+".jsonl" || strings.HasSuffix(entry.Name(), "-"+vendor+".jsonl") {
			native = destination
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("copy provider history: %w", err)
	}
	if native == "" {
		return "", fmt.Errorf("provider transcript for this conversation is unavailable")
	}
	return native, nil
}

type contextReader struct {
	context.Context
	io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(buffer)
}
