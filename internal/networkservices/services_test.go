//go:build !windows

package networkservices

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

type fixtureContainer struct {
	ID         string `json:"Id"`
	Name       string
	Config     map[string]any
	HostConfig map[string]any
	Mounts     []map[string]any
	State      struct {
		Running bool
		Status  string
	}
	NetworkSettings map[string]any
}
type fixture struct {
	mu                                           sync.Mutex
	containers                                   map[string]*fixtureContainer
	creates                                      []map[string]any
	calls                                        [][]string
	owner, root                                  string
	failCreate, lostCreate, failStart, failReady bool
}

func TestMain(m *testing.M) {
	if os.Getenv("CXZ_NETWORK_FIXTURE_MAIN") == "1" {
		if err := fixtureCLI(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fixtureCLI(args []string) error {
	socket := os.Getenv("CXZ_NETWORK_FIXTURE_SOCKET")
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}
	defer client.CloseIdleConnections()
	b, _ := json.Marshal(args)
	res, err := client.Post("http://docker/cli", "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err = io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("%s", b)
	}
	if len(args) > 0 && args[0] == "run" && strings.Contains(strings.Join(args, " "), "cxz.role=network-lock") {
		lock, err := core.Lock(filepath.Join(os.Getenv("CXZ_NETWORK_FIXTURE_ROOT"), LockFile))
		if err != nil {
			return err
		}
		defer lock.Close()
		if pending(os.Getenv("CXZ_NETWORK_FIXTURE_ROOT")) {
			fmt.Println("locked pending")
		} else {
			fmt.Println("locked")
		}
		_, err = io.Copy(io.Discard, os.Stdin)
		return err
	}
	_, err = os.Stdout.Write(b)
	return err
}
func (f *fixture) find(id string) (*fixtureContainer, error) {
	for _, v := range f.containers {
		if v.ID == id || strings.TrimPrefix(v.Name, "/") == id {
			return v, nil
		}
	}
	return nil, fmt.Errorf("No such object: %s", id)
}
func (f *fixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out any
	var err error
	if r.URL.Path == "/cli" {
		var args []string
		_ = json.NewDecoder(r.Body).Decode(&args)
		f.calls = append(f.calls, args)
		switch args[0] {
		case "inspect":
			var v *fixtureContainer
			v, err = f.find(args[1])
			if err == nil {
				out = []*fixtureContainer{v}
			}
		case "rename":
			var v *fixtureContainer
			v, err = f.find(args[1])
			if err == nil {
				if _, exists := f.find(args[2]); exists == nil {
					err = fmt.Errorf("name in use")
				} else {
					v.Name = "/" + args[2]
				}
			}
		case "stop", "start":
			var v *fixtureContainer
			v, err = f.find(args[len(args)-1])
			if err == nil {
				if args[0] == "start" && f.failStart && strings.HasPrefix(v.ID, "new-") {
					err = fmt.Errorf("injected start failure")
				} else {
					v.State.Running = args[0] == "start"
				}
			}
		case "rm":
			var v *fixtureContainer
			v, err = f.find(args[len(args)-1])
			if err == nil {
				delete(f.containers, v.ID)
			}
		case "exec":
			var v *fixtureContainer
			v, err = f.find(args[1])
			if err == nil && f.failReady && strings.HasPrefix(v.ID, "new-") {
				err = fmt.Errorf("injected readiness failure")
			}
		case "run": // preflight or the stdin-held lock helper
		default:
			err = fmt.Errorf("unexpected Docker command %v", args)
		}
	} else if r.Method == "GET" {
		out, err = f.find(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/containers/"), "/json"))
	} else if r.Method == "POST" && r.URL.Path == "/containers/create" {
		var config map[string]any
		err = json.NewDecoder(r.Body).Decode(&config)
		if err == nil && f.failCreate {
			err = fmt.Errorf("injected create failure")
		}
		if err == nil {
			f.creates = append(f.creates, config)
			id := fmt.Sprintf("new-%d", len(f.creates))
			host := config["HostConfig"].(map[string]any)
			delete(config, "HostConfig")
			f.containers[id] = &fixtureContainer{ID: id, Name: "/" + r.URL.Query().Get("name"), Config: config, HostConfig: host}
			if f.lostCreate {
				err = fmt.Errorf("lost create acknowledgement")
			} else {
				out = map[string]string{"Id": id}
			}
		}
	} else {
		err = fmt.Errorf("unexpected endpoint %s", r.URL.Path)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if out != nil {
		_ = json.NewEncoder(w).Encode(out)
	}
}
func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	root := t.TempDir()
	f := &fixture{root: root, owner: core.ID(), containers: map[string]*fixtureContainer{}}
	for _, role := range []string{"daemon", "web", "remote"} {
		name := "manager"
		image := "image:new"
		if role != "daemon" {
			name = serviceName(name, role)
			image = "image:old"
		}
		v := &fixtureContainer{ID: role, Name: "/" + name, Config: map[string]any{"Image": image, "Labels": map[string]string{"cxz.owner": f.owner, "cxz.role": role}, "Env": []string{"PRESERVE=yes"}, "Cmd": []string{"--state", "/var/lib/cxz", "_web-serve", "--origin", "https://example.test:7443"}}, HostConfig: map[string]any{"ReadonlyRootfs": true, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges"}, "PortBindings": map[string]any{"7350/tcp": []map[string]string{{"HostIp": "127.0.0.1", "HostPort": "7443"}}}, "Binds": []string{"/host/token:/web/token:ro"}}}
		v.State.Running = true
		if role == "daemon" {
			v.Mounts = []map[string]any{{"Type": "volume", "Name": "fixture-state", "Destination": "/var/lib/cxz"}}
		}
		f.containers[v.ID] = v
	}
	socket := filepath.Join(root, "d.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(f.serve)}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	t.Setenv("DOCKER_HOST", "unix://"+socket)
	t.Setenv("CXZ_NETWORK_FIXTURE_MAIN", "1")
	t.Setenv("CXZ_NETWORK_FIXTURE_SOCKET", socket)
	t.Setenv("CXZ_NETWORK_FIXTURE_ROOT", root)
	exe, _ := os.Executable()
	t.Setenv("CXZ_NETWORK_FIXTURE_EXE", exe)
	bin := filepath.Join(root, "bin")
	_ = os.Mkdir(bin, 0700)
	_ = os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexec \"$CXZ_NETWORK_FIXTURE_EXE\" \"$@\"\n"), 0700)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return f
}
func (f *fixture) current(t *testing.T, role string) *fixtureContainer {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	v, err := f.find(serviceName("manager", role))
	if err != nil {
		t.Fatal(err)
	}
	// Do not expose mutable server state to the test goroutine.
	b, _ := json.Marshal(v)
	var copy fixtureContainer
	_ = json.Unmarshal(b, &copy)
	return &copy
}
func (f *fixture) createCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.creates)
}
func TestRefreshClonesConfigurationAndIsIdempotent(t *testing.T) {
	f := newFixture(t)
	before, _ := json.Marshal(f.current(t, "web").HostConfig)
	report, err := Refresh(t.Context(), f.root, "manager", f.owner)
	if err != nil || report.Web.NeedsRefresh || report.Relay.NeedsRefresh {
		t.Fatalf("%+v %v", report, err)
	}
	for _, role := range []string{"web", "remote"} {
		v := f.current(t, role)
		if v.ID == role || v.Config["Image"] != "image:new" || !v.State.Running {
			t.Fatal("service not replaced", v)
		}
		after, _ := json.Marshal(v.HostConfig)
		if string(before) != string(after) {
			t.Fatal("HostConfig changed", string(after))
		}
	}
	count := f.createCount()
	if _, err = Refresh(t.Context(), f.root, "manager", f.owner); err != nil || f.createCount() != count {
		t.Fatal("retry was not a no-op", err)
	}
	if pending(f.root) {
		t.Fatal("successful refresh left recovery work")
	}
}
func TestStoppedRemovedAndForeignServicesArePreserved(t *testing.T) {
	for _, scenario := range []string{"stopped", "removed", "foreign"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			f.mu.Lock()
			f.containers["remote"].State.Running = false
			switch scenario {
			case "stopped":
				f.containers["web"].State.Running = false
			case "removed":
				delete(f.containers, "web")
			case "foreign":
				f.containers["web"].Config["Labels"] = map[string]string{"cxz.owner": "other", "cxz.role": "web"}
			}
			f.mu.Unlock()
			_, err := Refresh(t.Context(), f.root, "manager", f.owner)
			if (err != nil) != (scenario == "foreign") || f.createCount() != 0 {
				t.Fatal(err)
			}
		})
	}
}
func TestFailedReplacementRestoresPreviousServiceAndRetries(t *testing.T) {
	for _, failure := range []string{"create", "lost-create", "start", "readiness"} {
		t.Run(failure, func(t *testing.T) {
			f := newFixture(t)
			f.mu.Lock()
			f.containers["remote"].State.Running = false
			f.failCreate = failure == "create"
			f.lostCreate = failure == "lost-create"
			f.failStart = failure == "start"
			f.failReady = failure == "readiness"
			f.mu.Unlock()
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if _, err := Refresh(ctx, f.root, "manager", f.owner); err == nil {
				t.Fatal("failure hidden")
			}
			old := f.current(t, "web")
			if old.ID != "web" || !old.State.Running {
				t.Fatal("old service not restored", old)
			}
			if pending(f.root) {
				t.Fatal("successful rollback left a journal")
			}
			f.mu.Lock()
			f.failCreate = false
			f.lostCreate = false
			f.failStart = false
			f.failReady = false
			f.mu.Unlock()
			if _, err := Refresh(t.Context(), f.root, "manager", f.owner); err != nil {
				t.Fatal("retry failed", err)
			}
		})
	}
}
func TestInterruptedReplacementRecoversBeforeRefresh(t *testing.T) {
	for _, phase := range []string{"before-create", "created", "old-renamed", "old-stopped", "new-running", "old-removed"} {
		t.Run(phase, func(t *testing.T) {
			f := newFixture(t)
			f.mu.Lock()
			f.containers["remote"].State.Running = false
			f.mu.Unlock()
			tx := transaction{Owner: f.owner, Name: "manager-web", Role: "web", OldID: "web", NewName: "manager-web-next-fixture", Image: "image:new"}
			if err := saveTransaction(f.root, tx); err != nil {
				t.Fatal(err)
			}
			if phase != "before-create" {
				var err error
				tx.NewID, err = dockerx.CloneOwned(t.Context(), "web", tx.NewName, f.owner, "web", tx.Image)
				if err != nil {
					t.Fatal(err)
				}
				// Leave NewID unacknowledged in the journal: recover by durable name.
				if phase != "created" {
					_, _ = dockerx.Run(t.Context(), "rename", "web", "manager-web-previous-fixture")
				}
				if phase == "old-stopped" || phase == "new-running" || phase == "old-removed" {
					_, _ = dockerx.Run(t.Context(), "stop", "web")
				}
				if phase == "new-running" || phase == "old-removed" {
					_, _ = dockerx.Run(t.Context(), "rename", tx.NewID, tx.Name)
					_, _ = dockerx.Run(t.Context(), "start", tx.NewID)
					_ = saveTransaction(f.root, tx)
				}
				if phase == "old-removed" {
					_, _ = dockerx.Run(t.Context(), "rm", "web")
				}
			}
			if _, err := Refresh(t.Context(), f.root, "manager", f.owner); err != nil {
				t.Fatal(err)
			}
			v := f.current(t, "web")
			if !v.State.Running || v.Config["Image"] != "image:new" || pending(f.root) {
				t.Fatal("recovery incomplete", v)
			}
		})
	}
}
func TestHostVolumeLockCoordinatesWithAutomaticRefresh(t *testing.T) {
	f := newFixture(t)
	work, release, err := LockHost(t.Context(), "manager", f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if work.Err() != nil {
		t.Fatal(work.Err())
	}
	report, err := Refresh(t.Context(), f.root, "manager", f.owner)
	if err == nil || !strings.Contains(report.Error, "busy") || f.createCount() != 0 {
		t.Fatal("automatic refresh bypassed host lock", report, err)
	}
	release()
	if _, err = Refresh(t.Context(), f.root, "manager", f.owner); err != nil {
		t.Fatal("lock not released", err)
	}
}

func TestRecoveryRefusesChangedCanonicalContainer(t *testing.T) {
	f := newFixture(t)
	tx := transaction{Owner: f.owner, Name: "manager-web", Role: "web", OldID: "web", NewName: "manager-web-next-fixture", Image: "image:new"}
	if err := saveTransaction(f.root, tx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.containers["web"].Name = "/manager-web-previous-fixture"
	other := &fixtureContainer{ID: "other", Name: "/manager-web", Config: map[string]any{"Image": "image:custom", "Labels": map[string]string{"cxz.owner": f.owner, "cxz.role": "web"}}}
	other.State.Running = true
	f.containers[other.ID] = other
	f.mu.Unlock()
	if err := RecoverLocked(t.Context(), f.root, "manager", f.owner); err == nil {
		t.Fatal("changed canonical identity accepted")
	}
	if f.current(t, "web").ID != other.ID || !pending(f.root) {
		t.Fatal("changed container overwritten or recovery forgotten")
	}
}

func TestCanceledHostReleasesVolumeLock(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	work, release, err := LockHost(ctx, "manager", f.owner)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-work.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("operation not canceled")
	}
	release()
	if _, err = Refresh(t.Context(), f.root, "manager", f.owner); err != nil {
		t.Fatal("canceled host left volume locked", err)
	}
}

func TestHostCanLockWithStoppedManagerWithoutRecovery(t *testing.T) {
	f := newFixture(t)
	f.mu.Lock()
	f.containers["daemon"].State.Running = false
	f.mu.Unlock()
	_, release, err := LockHost(t.Context(), "manager", f.owner)
	if err != nil {
		t.Fatal(err)
	}
	release()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.calls {
		if call[0] == "exec" {
			t.Fatal("stopped manager unnecessarily executed", call)
		}
	}
}
