package cxzupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type dockerFixture struct {
	Containers    map[string]dockerx.Container
	ProjectLeases map[string]string
	FailCreate    bool
	Target        string
}

func readDockerFixture(path string) (dockerFixture, error) {
	var f dockerFixture
	b, e := os.ReadFile(path)
	if e == nil {
		e = json.Unmarshal(b, &f)
	}
	return f, e
}
func fixtureContainer(f dockerFixture, id string) (dockerx.Container, error) {
	for _, v := range f.Containers {
		if v.ID == id || strings.TrimPrefix(v.Name, "/") == id {
			return v, nil
		}
	}
	return dockerx.Container{}, fmt.Errorf("no such container")
}
func fixtureDocker(args []string) error {
	path := os.Getenv("CXZ_TEST_DOCKER_STATE")
	f, e := readDockerFixture(path)
	if e != nil {
		return e
	}
	save := func() error { return core.WriteJSON(path, f) }
	switch args[0] {
	case "inspect":
		v, e := fixtureContainer(f, args[1])
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode([]dockerx.Container{v})
	case "ps":
		for id := range f.Containers {
			fmt.Println(id)
		}
		return nil
	case "stop":
		v, e := fixtureContainer(f, args[len(args)-1])
		if e != nil {
			return e
		}
		v.State.Running = false
		f.Containers[v.ID] = v
		return save()
	case "rename":
		v, e := fixtureContainer(f, args[1])
		if e != nil {
			return e
		}
		v.Name = "/" + args[2]
		f.Containers[v.ID] = v
		return save()
	case "start":
		v, e := fixtureContainer(f, args[1])
		if e != nil {
			return e
		}
		v.State.Running = true
		f.Containers[v.ID] = v
		return save()
	case "rm":
		v, e := fixtureContainer(f, args[len(args)-1])
		if e != nil {
			return e
		}
		delete(f.Containers, v.ID)
		return save()
	case "exec":
		args = args[1:]
		if args[0] == "--user" {
			args = args[2:]
		}
		v, e := fixtureContainer(f, args[0])
		if e != nil {
			return e
		}
		args = args[1:]
		if args[0] == "stat" {
			fmt.Println("1000")
			return nil
		}
		if len(args) > 1 && args[1] == "_publish-tools" {
			return nil
		}
		if len(args) > 1 && args[1] == "_maintenance" {
			action, id := args[3], args[4]
			if action == "prepare" {
				f.ProjectLeases[v.ID] = id
			}
			if action == "release" {
				delete(f.ProjectLeases, v.ID)
			}
			if e = save(); e != nil {
				return e
			}
			return json.NewEncoder(os.Stdout).Encode(Health{Ready: true, Lease: f.ProjectLeases[v.ID], Sessions: map[string]string{v.ID: "stable-run"}, Build: Build{Protocol: Protocol, Schema: Schema}})
		}
	}
	return fmt.Errorf("unexpected fixture docker arguments: %v", args)
}
func managerFixture(t *testing.T, fail bool) (string, ManagerTransaction) {
	t.Helper()
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "run"), 0700)
	r := testRelease(nil)
	owner := core.ID()
	old := dockerx.Container{ID: "old", Name: "/manager"}
	old.Config.Image = "old-image"
	old.Config.Labels = map[string]string{"cxz.owner": owner, "cxz.role": "daemon"}
	old.State.Running = true
	path := filepath.Join(root, "docker.json")
	f := dockerFixture{Containers: map[string]dockerx.Container{"old": old}, ProjectLeases: map[string]string{}, Target: r.Image, FailCreate: fail}
	if e := core.WriteJSON(path, f); e != nil {
		t.Fatal(e)
	}
	exe, _ := os.Executable()
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0700)
	if e := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexec \"$CXZ_TEST_DOCKER_EXE\" \"$@\"\n"), 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CXZ_TEST_DOCKER_EXE", exe)
	t.Setenv("CXZ_TEST_DOCKER_STATE", path)
	dockerSock := filepath.Join(root, "docker.sock")
	ln, e := net.Listen("unix", dockerSock)
	if e != nil {
		t.Fatal(e)
	}
	api := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		f, e := readDockerFixture(path)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		if q.Method == "GET" {
			v, e := fixtureContainer(f, strings.TrimSuffix(strings.TrimPrefix(q.URL.Path, "/containers/"), "/json"))
			if e != nil {
				http.Error(w, e.Error(), 404)
				return
			}
			_ = json.NewEncoder(w).Encode(v)
			return
		}
		if f.FailCreate {
			http.Error(w, "injected create failure", 500)
			return
		}
		var config struct {
			Image  string
			Labels map[string]string
		}
		if e = json.NewDecoder(q.Body).Decode(&config); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		v := dockerx.Container{ID: "new", Name: "/" + q.URL.Query().Get("name")}
		v.Config.Image = config.Image
		v.Config.Labels = config.Labels
		f.Containers[v.ID] = v
		if e = core.WriteJSON(path, f); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": v.ID})
	})}
	go api.Serve(ln)
	t.Cleanup(func() { api.Close() })
	t.Setenv("DOCKER_HOST", "unix://"+dockerSock)
	gate := NewGate(root)
	maintenance, e := net.Listen("unix", filepath.Join(root, "run", "update.sock"))
	if e != nil {
		t.Fatal(e)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		f, e := readDockerFixture(path)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		revision := ""
		for _, v := range f.Containers {
			if v.State.Running && v.Config.Labels["cxz.role"] == "daemon" {
				revision = strings.Repeat("a", 40)
				if v.Config.Image == f.Target {
					revision = r.Revision
				}
			}
		}
		if revision == "" {
			http.Error(w, "no manager", 503)
			return
		}
		var request struct{ Action, Transaction string }
		_ = json.NewDecoder(q.Body).Decode(&request)
		switch request.Action {
		case "prepare":
			e = gate.hold(request.Transaction, func() error { return nil })
		case "release":
			e = gate.release(request.Transaction)
		}
		h := Health{Build: Build{Revision: revision, Protocol: Protocol, Schema: Schema}, Ready: true}
		gate.mu.RLock()
		if gate.blocked() {
			h.Lease = gate.held.ID
		}
		gate.mu.RUnlock()
		if e != nil {
			h.Reason = e.Error()
			w.WriteHeader(409)
		}
		_ = json.NewEncoder(w).Encode(h)
	})}
	go server.Serve(maintenance)
	t.Cleanup(func() { server.Close() })
	tx := ManagerTransaction{ID: core.ID(), Name: "manager", Owner: owner, OldID: "old", Release: r, State: "staged"}
	if e = core.WriteJSON(ManagerTransactionPath(root), tx); e != nil {
		t.Fatal(e)
	}
	return root, tx
}
func TestManagerReplacementAndCreateFailureRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			root, tx := managerFixture(t, fail)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if e := ReplaceManager(ctx, root); e != nil {
				t.Fatal(e)
			}
			done, e := readManagerTransaction(root)
			if e != nil {
				t.Fatal(e)
			}
			want := "healthy"
			id := "new"
			if fail {
				want = "rolled_back"
				id = "old"
			}
			if done.State != want {
				t.Fatalf("%+v", done)
			}
			f, e := readDockerFixture(filepath.Join(root, "docker.json"))
			if e != nil {
				t.Fatal(e)
			}
			v, e := fixtureContainer(f, "manager")
			if e != nil || v.ID != id || !v.State.Running {
				t.Fatal("wrong manager restored", v, e)
			}
			if leaseOwned(root, tx.ID) {
				t.Fatal("admission remained closed")
			}
			if e = ReplaceManager(ctx, root); e != nil {
				t.Fatal("repeated acknowledgement", e)
			}
		})
	}
}
func TestFreezeProjectsResumesPartialEnumeration(t *testing.T) {
	root, tx := managerFixture(t, false)
	path := filepath.Join(root, "docker.json")
	f, _ := readDockerFixture(path)
	for _, id := range []string{"one", "two"} {
		v := dockerx.Container{ID: id}
		v.State.Running = true
		v.Config.Labels = map[string]string{"cxz.owner": tx.Owner, "cxz.project": id}
		v.Mounts = append(v.Mounts, struct {
			Source, Destination, Type, Name string
			RW                              bool
		}{Destination: "/cxz/state"})
		f.Containers[id] = v
	}
	if e := core.WriteJSON(path, f); e != nil {
		t.Fatal(e)
	}
	tx.Projects = []FrozenProject{{ID: "one", Container: "one", User: "1000", Before: Health{Sessions: map[string]string{"one": "stable-run"}}}}
	if e := freezeProjects(context.Background(), root, &tx); e != nil {
		t.Fatal(e)
	}
	f, _ = readDockerFixture(path)
	if len(tx.Projects) != 2 || f.ProjectLeases["one"] != tx.ID || f.ProjectLeases["two"] != tx.ID {
		t.Fatal("partial snapshot skipped remaining projects")
	}
}
func TestManagerRecoversAfterOldStopped(t *testing.T) {
	root, tx := managerFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h, e := Call(ctx, root, "prepare", tx.ID)
	if e != nil {
		t.Fatal(e)
	}
	tx.Old = h
	tx.State = "starting"
	core.WriteJSON(ManagerTransactionPath(root), tx)
	if _, e = dockerx.Run(ctx, "stop", "old"); e != nil {
		t.Fatal(e)
	}
	if _, e = dockerx.Run(ctx, "rename", "old", tx.Name+"-previous-"+tx.ID); e != nil {
		t.Fatal(e)
	}
	if e = ReplaceManager(ctx, root); e != nil {
		t.Fatal(e)
	}
	done, _ := readManagerTransaction(root)
	if done.State != "healthy" {
		t.Fatal(done.State)
	}
}
