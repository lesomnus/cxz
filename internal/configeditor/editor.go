package configeditor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/lesomnus/cxz/internal/settings"
)

const Prefix = "/configuration/"
const SocketEnv = "CXZ_CONFIGURATION_EDITOR_SOCKET"

func SocketDir(root string) string { return filepath.Join(root, "configuration-editor") }

func PrepareSocketDir(root string) (string, error) {
	dir, err := filepath.Abs(SocketDir(root))
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("configuration editor socket directory must be private: %s", dir)
	}
	return dir, nil
}

type Files = map[string]projectconfig.TemplateFile

type Document struct {
	Revision string `json:"revision"`
	Files    Files  `json:"files"`
	Builtin  bool   `json:"builtin,omitempty"`
}

type Result struct {
	Revision  string `json:"revision"`
	Published bool   `json:"published"`
	Error     string `json:"error,omitempty"`
}

// Editor exposes just two installation configuration sources. The request
// never selects a host path. Each client edits a copy of the returned files.
type Editor struct {
	Root    string
	Publish func(context.Context, projectconfig.Spec) error
}

type source struct {
	document      Document
	composePath   string
	composeTarget string
}

func composeRevision(path, target string, exists bool, data []byte) string {
	if exists && data == nil {
		data = []byte{}
	}
	return digest(struct {
		Path, Target string
		Exists       bool
		Data         []byte
	}{path, target, exists, data})
}

func (e Editor) read(kind string, cfg settings.Config) (source, error) {
	if kind == "template" {
		template, err := projectconfig.ReadTemplate(e.Root)
		if err != nil {
			return source{}, err
		}
		if template == nil {
			builtin, err := projectconfig.Builtin()
			if err != nil {
				return source{}, err
			}
			return source{document: Document{Revision: digest(nil), Files: builtin.Files, Builtin: true}}, nil
		}
		return source{document: Document{Revision: digest(template.Files), Files: template.Files}}, nil
	}
	if len(cfg.Devcontainer.LegacyInline) != 0 {
		return source{}, fmt.Errorf("move the legacy override to its own file with cxz edit docker-compose")
	}
	path, err := cfg.Devcontainer.Path(e.Root)
	if err != nil {
		return source{}, err
	}
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	} else if _, statErr := os.Lstat(path); statErr == nil || !os.IsNotExist(statErr) {
		return source{}, err
	}
	data, err := projectconfig.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return source{}, err
	}
	revision := composeRevision(path, target, err == nil, data)
	if os.IsNotExist(err) {
		data = projectconfig.Template()
	}
	return source{
		document: Document{
			Revision: revision,
			Files:    Files{projectconfig.Filename: {Data: data}},
		},
		composePath:   path,
		composeTarget: target,
	}, nil
}

func digest(v any) string {
	data, _ := json.Marshal(v)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (e Editor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Path
	if kind != Prefix+"compose" && kind != Prefix+"template" {
		http.NotFound(w, r)
		return
	}
	kind = kind[len(Prefix):]
	switch r.Method {
	case http.MethodGet:
		lock, err := core.Lock(filepath.Join(e.Root, "settings.lock"))
		if err != nil {
			http.Error(w, "Installation settings are being edited. Try again.", http.StatusConflict)
			return
		}
		defer lock.Close()
		cfg, err := settings.Load(e.Root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		current, err := e.read(kind, cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, current.document)
	case http.MethodPut:
		var next Document
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*projectconfig.MaxTemplateBytes))
		d.DisallowUnknownFields()
		if err := d.Decode(&next); err != nil {
			http.Error(w, "Invalid or oversized configuration draft", http.StatusBadRequest)
			return
		}
		if next.Revision == "" || next.Files == nil || d.Decode(new(any)) != io.EOF {
			http.Error(w, "Expected a revision and one configuration file set", http.StatusBadRequest)
			return
		}
		result, status, err := e.apply(r.Context(), kind, next)
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, result)
	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func (e Editor) apply(ctx context.Context, kind string, next Document) (Result, int, error) {
	if e.Publish == nil {
		return Result{}, http.StatusServiceUnavailable, fmt.Errorf("Manager publication is unavailable")
	}
	lock, err := core.Lock(filepath.Join(e.Root, "settings.lock"))
	if err != nil {
		return Result{}, http.StatusConflict, fmt.Errorf("Installation settings are being edited. Try again.")
	}
	defer lock.Close()
	cfg, err := settings.Load(e.Root)
	if err != nil {
		return Result{}, http.StatusUnprocessableEntity, err
	}
	current, err := e.read(kind, cfg)
	if err != nil {
		return Result{}, http.StatusUnprocessableEntity, err
	}
	if kind == "compose" {
		if err = os.MkdirAll(filepath.Dir(current.composeTarget), 0700); err != nil {
			return Result{}, http.StatusInternalServerError, err
		}
		target := current.composeTarget
		file, err := core.Lock(target + ".cxz.lock")
		if err != nil {
			return Result{}, http.StatusConflict, fmt.Errorf("The Compose file is being edited. Try again.")
		}
		defer file.Close()
		current, err = e.read(kind, cfg)
		if err != nil {
			return Result{}, http.StatusUnprocessableEntity, err
		}
		if target != current.composeTarget {
			return Result{}, http.StatusConflict, fmt.Errorf("The Compose path changed while saving. Reopen the editor.")
		}
	}
	if current.document.Revision != next.Revision {
		return Result{}, http.StatusConflict, fmt.Errorf("The host files changed since this editor opened. Close and reopen to see the new version.")
	}

	var spec projectconfig.Spec
	var revision string
	var writeErr error
	if kind == "compose" {
		f, ok := next.Files[projectconfig.Filename]
		if !ok || len(next.Files) != 1 || f.Executable {
			return Result{}, http.StatusUnprocessableEntity, fmt.Errorf("Expected one Compose YAML file")
		}
		spec, err = projectconfig.SnapshotFile(current.composePath, f.Data)
		if err == nil {
			spec.Template, err = projectconfig.SnapshotTemplate(e.Root)
		}
		if err != nil {
			return Result{}, http.StatusUnprocessableEntity, err
		}
		// Write even an unchanged starter document when its source is absent.
		before, err := projectconfig.ReadFile(current.composeTarget)
		if err != nil && !os.IsNotExist(err) {
			return Result{}, http.StatusInternalServerError, err
		}
		if err != nil || !bytes.Equal(before, f.Data) {
			writeErr = core.WriteFile(current.composeTarget, f.Data)
		}
		revision = composeRevision(current.composePath, current.composeTarget, true, f.Data)
	} else {
		if len(next.Files) != 0 {
			for name, file := range next.Files {
				if file.Data == nil {
					file.Data = []byte{}
					next.Files[name] = file
				}
			}
			spec.Template = &projectconfig.DefaultTemplate{Files: next.Files}
			if err = spec.Template.Validate(); err != nil {
				return Result{}, http.StatusUnprocessableEntity, err
			}
		}
		// Resolve Compose with the same host directory and environment as the CLI.
		spec, err = cfg.Devcontainer.WithTemplate(e.Root, spec.Template)
		if err != nil {
			return Result{}, http.StatusUnprocessableEntity, err
		}
		revision = digest(nil)
		if spec.Template != nil {
			revision = digest(spec.Template.Files)
		}
		if revision != current.document.Revision {
			writeErr = replaceTemplate(e.Root, spec.Template)
		}
	}

	saved, err := e.read(kind, cfg)
	if err != nil {
		return Result{}, http.StatusInternalServerError, err
	}
	if saved.document.Revision != revision {
		if writeErr != nil {
			return Result{}, http.StatusInternalServerError, writeErr
		}
		return Result{}, http.StatusConflict, fmt.Errorf("The source changed during save. Close and reopen to see the new version.")
	}
	result := Result{Revision: revision}
	if writeErr != nil {
		result.Error = writeErr.Error()
		return result, 0, nil
	}
	if err := e.Publish(ctx, spec); err != nil {
		result.Error = err.Error()
		return result, 0, nil
	}
	result.Published = true
	return result, 0, nil
}

// The prepared directory is a sibling of the live template, allowing one
// filesystem operation to replace every file, including removals.
func replaceTemplate(root string, template *projectconfig.DefaultTemplate) error {
	live := filepath.Join(root, projectconfig.TemplateDirectory)
	pending, err := os.MkdirTemp(root, ".cxz-template-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(pending)
	if template == nil {
		if err := os.Rename(live, filepath.Join(pending, "removed")); err != nil && !os.IsNotExist(err) {
			return err
		}
		return core.SyncDir(root)
	}
	dirs := map[string]bool{pending: true}
	for name, file := range template.Files {
		path := filepath.Join(pending, filepath.FromSlash(name))
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		for p := dir; !dirs[p]; p = filepath.Dir(p) {
			dirs[p] = true
		}
		mode := os.FileMode(0600)
		if file.Executable {
			mode = 0700
		}
		if err := writeStaged(path, file.Data, mode); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(dirs))
	for dir := range dirs {
		names = append(names, dir)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, dir := range names {
		if err := core.SyncDir(dir); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(live); os.IsNotExist(err) {
		if err := os.Rename(pending, live); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if err := exchangeTemplate(pending, live); err != nil {
		return fmt.Errorf("atomically replace devcontainer template: %w", err)
	}
	return core.SyncDir(root)
}

func writeStaged(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
