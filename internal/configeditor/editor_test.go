package configeditor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/projectconfig"
)

func request(t *testing.T, e Editor, method, kind string, body any, status int, reply any) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(method, Prefix+kind, bytes.NewReader(data)))
	if w.Code != status {
		t.Fatalf("%s %s: status %d, want %d: %s", method, kind, w.Code, status, w.Body.String())
	}
	if reply != nil {
		if err := json.Unmarshal(w.Body.Bytes(), reply); err != nil {
			t.Fatal(err)
		}
	}
}

func write(t *testing.T, filename, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestComposePreservesHostSourceAndRetriesPublication(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config", "override.yaml")
	write(t, filepath.Join(root, "settings.jsonc"), `{"devcontainer":{"compose":"config/override.yaml"}}`)
	write(t, filepath.Join(root, "config", "extra.yaml"), "services:\n  cache:\n    image: redis:7\n")
	original := "# Original comment\ninclude: [extra.yaml]\n"
	write(t, path, original)
	var calls int
	e := Editor{Root: root, Publish: func(_ context.Context, spec projectconfig.Spec) error {
		calls++
		if !strings.Contains(string(spec.Compose), "redis:7") || strings.Contains(string(spec.Compose), "extra.yaml") {
			t.Errorf("include was not resolved on the host: %s", spec.Compose)
		}
		if calls == 1 {
			return errors.New("manager unavailable")
		}
		return nil
	}}
	var draft Document
	request(t, e, http.MethodGet, "compose", nil, http.StatusOK, &draft)
	if string(draft.Files[projectconfig.Filename].Data) != original {
		t.Fatalf("GET must return original YAML: %#v", draft)
	}
	draft.Files[projectconfig.Filename] = projectconfig.TemplateFile{Data: []byte("services: [\n")}
	request(t, e, http.MethodPut, "compose", draft, http.StatusUnprocessableEntity, nil)
	if b, err := os.ReadFile(path); err != nil || string(b) != original || calls != 0 {
		t.Fatalf("invalid edit changed source or published: %q, %v, %d", b, err, calls)
	}
	saved := original + "# New comment\n"
	draft.Files[projectconfig.Filename] = projectconfig.TemplateFile{Data: []byte(saved)}
	var result Result
	request(t, e, http.MethodPut, "compose", draft, http.StatusOK, &result)
	if result.Published || result.Revision == draft.Revision || result.Error != "manager unavailable" {
		t.Fatalf("partial save not reported: %+v", result)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != saved {
		t.Fatalf("raw source not persisted: %q, %v", b, err)
	}
	draft.Revision = result.Revision
	request(t, e, http.MethodPut, "compose", draft, http.StatusOK, &result)
	if !result.Published || calls != 2 {
		t.Fatalf("retry did not publish: %+v, calls %d", result, calls)
	}
	write(t, path, original+"# CLI edit\n")
	request(t, e, http.MethodPut, "compose", draft, http.StatusConflict, nil)
	if b, err := os.ReadFile(path); err != nil || string(b) != original+"# CLI edit\n" || calls != 2 {
		t.Fatalf("stale draft overwrote a host edit: %q, %v, %d", b, err, calls)
	}
}

func TestTemplateCommitReplacesWholeDirectoryAndCanReset(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("directory exchange requires Linux")
	}
	root := t.TempDir()
	dir := filepath.Join(root, projectconfig.TemplateDirectory)
	config := `{"image":"alpine"}`
	write(t, filepath.Join(dir, "devcontainer.json"), config)
	write(t, filepath.Join(dir, "old.sh"), "old")
	var published projectconfig.Spec
	e := Editor{Root: root, Publish: func(_ context.Context, spec projectconfig.Spec) error {
		published = spec
		return nil
	}}
	var draft Document
	request(t, e, http.MethodGet, "template", nil, http.StatusOK, &draft)
	delete(draft.Files, "old.sh")
	draft.Files["../escape"] = projectconfig.TemplateFile{Data: []byte("invalid")}
	request(t, e, http.MethodPut, "template", draft, http.StatusUnprocessableEntity, nil)
	if _, err := os.Stat(filepath.Join(dir, "old.sh")); err != nil {
		t.Fatal(err)
	}
	delete(draft.Files, "../escape")
	draft.Files["scripts/setup.sh"] = projectconfig.TemplateFile{Data: []byte("#!/bin/sh\n"), Executable: true}
	var result Result
	request(t, e, http.MethodPut, "template", draft, http.StatusOK, &result)
	if !result.Published || published.Template == nil || len(published.Template.Files) != 2 {
		t.Fatalf("template publication failed: %+v, %+v", result, published)
	}
	if st, err := os.Stat(filepath.Join(dir, "scripts/setup.sh")); err != nil || st.Mode()&0100 == 0 {
		t.Fatalf("executable mode lost: %v, %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.sh")); !os.IsNotExist(err) {
		t.Fatalf("removed file remains: %v", err)
	}
	draft.Revision, draft.Files = result.Revision, Files{}
	request(t, e, http.MethodPut, "template", draft, http.StatusOK, &result)
	var defaults Document
	request(t, e, http.MethodGet, "template", nil, http.StatusOK, &defaults)
	if !result.Published || published.Template != nil || !defaults.Builtin || len(defaults.Files) == 0 {
		t.Fatalf("reset did not restore built-in template: %+v, %+v", result, defaults)
	}
}

func TestOversizedDraftDoesNotWriteHostFiles(t *testing.T) {
	root := t.TempDir()
	e := Editor{Root: root, Publish: func(context.Context, projectconfig.Spec) error {
		t.Fatal("oversized draft must not publish")
		return nil
	}}
	var draft Document
	request(t, e, http.MethodGet, "compose", nil, http.StatusOK, &draft)
	draft.Files[projectconfig.Filename] = projectconfig.TemplateFile{Data: make([]byte, projectconfig.MaxBytes+1)}
	request(t, e, http.MethodPut, "compose", draft, http.StatusUnprocessableEntity, nil)
	if _, err := os.Stat(filepath.Join(root, projectconfig.Filename)); !os.IsNotExist(err) {
		t.Fatalf("oversized draft wrote a file: %v", err)
	}
}
