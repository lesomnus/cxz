package cxzupdate

import (
	"context"
	"encoding/json"
	"fmt"
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

// A rollout small enough to run in a test and complete enough to be wrong in
// the ways the real one has been.
//
// Every update bug of the last three days -- cxz#224, cxz#235, cxz#244,
// cxz#249, cxz#251 -- was a decision, not a container: when a session counts as
// quiet, and what an unfinished transaction owes. Each one cost a CI run, a
// release, and a rollout window to see once, which is hours per answer. Here a
// scenario is a few milliseconds: the sweep refuses or does not, the channel
// moves or does not, the helper runs when I say so, and the transaction on disk
// is the answer.
//
// What this does not cover is deliberately narrow: no real images, no real
// processes, no project runtimes. It covers the manager handover, which is the
// part that wedges an installation. See cxz#253.
type rolloutLab struct {
	t     *testing.T
	Root  string
	Owner string

	docker string
	gate   *Gate

	mu       sync.Mutex
	release  Release
	refusal  error
	images   map[string]string // image -> revision, as a running manager reports it
	prepared []string          // every prepare action the helper asked for
}

// A manager image has to be pinned by digest, so each release in a scenario
// gets a digest of its own letter.
func labImage(letter string) string {
	return "ghcr.io/lesomnus/cxz@sha256:" + strings.Repeat(letter, 64)
}

func newRolloutLab(t *testing.T) *rolloutLab {
	t.Helper()
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	root := t.TempDir()
	if e := os.MkdirAll(filepath.Join(root, "run"), 0700); e != nil {
		t.Fatal(e)
	}
	l := &rolloutLab{t: t, Root: root, Owner: core.ID(), docker: filepath.Join(root, "docker.json"), images: map[string]string{}}
	l.release = l.publish("b")

	old := dockerx.Container{ID: "old", Name: "/manager"}
	old.Config.Image = labImage("a")
	old.Config.Labels = map[string]string{"cxz.owner": l.Owner, "cxz.role": "daemon"}
	old.State.Running = true
	// The replacement reuses the old manager's state and tools volumes, and
	// refuses to run without them.
	old.Mounts = append(old.Mounts, struct {
		Source, Destination, Type, Name string
		RW                              bool
	}{Type: "volume", Name: "cxz-state", Destination: root, RW: true})
	l.images[labImage("a")] = strings.Repeat("a", 40)
	l.save(dockerFixture{Containers: map[string]dockerx.Container{"old": old}, ProjectLeases: map[string]string{}})

	exe, _ := os.Executable()
	bin := filepath.Join(root, "bin")
	if e := os.MkdirAll(bin, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexec \"$CXZ_TEST_DOCKER_EXE\" \"$@\"\n"), 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CXZ_TEST_DOCKER_EXE", exe)
	t.Setenv("CXZ_TEST_DOCKER_STATE", l.docker)

	sock := filepath.Join(root, "docker.sock")
	ln, e := net.Listen("unix", sock)
	if e != nil {
		t.Fatal(e)
	}
	api := &http.Server{Handler: http.HandlerFunc(l.serveDockerAPI)}
	go api.Serve(ln)
	t.Cleanup(func() { api.Close() })
	t.Setenv("DOCKER_HOST", "unix://"+sock)

	l.gate = NewGate(root)
	maintenance, e := net.Listen("unix", filepath.Join(root, "run", "update.sock"))
	if e != nil {
		t.Fatal(e)
	}
	server := &http.Server{Handler: http.HandlerFunc(l.serveMaintenance)}
	go server.Serve(maintenance)
	t.Cleanup(func() { server.Close() })
	return l
}

// publish is a new release on the channel, named by one letter so a scenario
// can be read.
func (l *rolloutLab) publish(letter string) Release {
	r := testRelease(nil)
	r.Revision = strings.Repeat(letter, 40)
	r.Image = labImage(letter)
	// Asset names carry the revision, and the release refuses to be used if
	// they disagree with it.
	for platform, a := range r.Assets {
		a.Name = "cxz-" + r.Revision + "-" + strings.ReplaceAll(platform, "/", "-")
		if strings.HasPrefix(platform, "windows/") {
			a.Name += ".exe"
		}
		r.Assets[platform] = a
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.images[r.Image] = r.Revision
	l.release = r
	return r
}

// Refuse is what the readiness sweep says from now on; nil means ready.
func (l *rolloutLab) Refuse(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refusal = err
}

func (l *rolloutLab) current() Release {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.release
}

// Schedule is one rollout tick: what the manager does when it notices a release.
func (l *rolloutLab) Schedule(force bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return ScheduleManager(ctx, l.Root, "manager", l.Owner, l.current(), force)
}

// Helper is the update container the schedule starts, which is the thing that
// actually replaces the manager.
func (l *rolloutLab) Helper() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return ReplaceManager(ctx, l.Root)
}

func (l *rolloutLab) Transaction() ManagerTransaction {
	l.t.Helper()
	tx, e := readManagerTransaction(l.Root)
	if e != nil && !os.IsNotExist(e) {
		l.t.Fatal(e)
	}
	return tx
}

// Running is the image of the manager container that is up, which is how the
// test says "the installation is on the new release" without a real build.
func (l *rolloutLab) Running() string {
	l.t.Helper()
	f, e := readDockerFixture(l.docker)
	if e != nil {
		l.t.Fatal(e)
	}
	for _, v := range f.Containers {
		if v.State.Running && v.Config.Labels["cxz.role"] == "daemon" {
			return v.Config.Image
		}
	}
	return ""
}

func (l *rolloutLab) Prepares() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.prepared...)
}

func (l *rolloutLab) save(f dockerFixture) {
	l.t.Helper()
	f.Target = l.current().Image
	if e := core.WriteJSON(l.docker, f); e != nil {
		l.t.Fatal(e)
	}
}

func (l *rolloutLab) serveDockerAPI(w http.ResponseWriter, q *http.Request) {
	f, e := readDockerFixture(l.docker)
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
	var config struct {
		Image  string
		Labels map[string]string
	}
	if e = json.NewDecoder(q.Body).Decode(&config); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	id := "new-" + config.Image
	v := dockerx.Container{ID: id, Name: "/" + q.URL.Query().Get("name")}
	v.Config.Image = config.Image
	v.Config.Labels = config.Labels
	f.Containers[id] = v
	if e = core.WriteJSON(l.docker, f); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
}

func (l *rolloutLab) serveMaintenance(w http.ResponseWriter, q *http.Request) {
	f, e := readDockerFixture(l.docker)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	revision := ""
	for _, v := range f.Containers {
		if v.State.Running && v.Config.Labels["cxz.role"] == "daemon" {
			l.mu.Lock()
			revision = l.images[v.Config.Image]
			l.mu.Unlock()
		}
	}
	if revision == "" {
		http.Error(w, "no manager", 503)
		return
	}
	var request struct{ Action, Transaction string }
	_ = json.NewDecoder(q.Body).Decode(&request)
	h := Health{Build: Build{Revision: revision, Protocol: Protocol, Schema: Schema}, Ready: true}
	l.mu.Lock()
	refusal := l.refusal
	if strings.HasPrefix(request.Action, "prepare") {
		l.prepared = append(l.prepared, request.Action)
	}
	l.mu.Unlock()
	switch request.Action {
	case "prepare":
		e = l.gate.hold(request.Transaction, func() error { return refusal })
	case "prepare-force":
		e = l.gate.hold(request.Transaction, func() error { return nil })
		h.Reason = "forced"
	case "release":
		e = l.gate.release(request.Transaction)
	}
	l.gate.mu.RLock()
	if l.gate.blocked() {
		h.Lease = l.gate.held.ID
	}
	l.gate.mu.RUnlock()
	if e != nil {
		h.Ready = false
		h.Reason = e.Error()
		w.WriteHeader(409)
	}
	_ = json.NewEncoder(w).Encode(h)
}

// The afternoon of 2026-10-20, in milliseconds. A rollout of one release was
// staged and then refused by a session with a browser tab attached; two fixes
// were merged, which moved the channel; and from then on the installation
// answered "previous manager update needs recovery" and could not be moved off
// the revision it was already trying to leave. See cxz#251.
func TestARolloutSurvivesTheChannelMovingWhileItWaits(t *testing.T) {
	l := newRolloutLab(t)
	first := l.current()
	l.Refuse(fmt.Errorf("session 5aab0fb3: waiting for 5 minutes of continuous idle"))
	if e := l.Schedule(false); e != nil {
		t.Fatal("scheduling a rollout failed before anything was refused:", e)
	}
	if tx := l.Transaction(); tx.State != "staged" || tx.Release.Revision != first.Revision {
		t.Fatalf("the transaction is not staged for the release being rolled out: %+v", tx)
	}
	// The helper tries, is refused, and leaves the reservation behind. Four
	// attempts is what `--restart on-failure:3` gives it.
	for i := 0; i < 4; i++ {
		if e := l.Helper(); e == nil {
			t.Fatal("a refusing sweep let the manager be replaced")
		}
	}
	if tx := l.Transaction(); tx.State != "staged" {
		t.Fatalf("a refused attempt left something other than a reservation: %+v", tx)
	}
	if l.Running() != labImage("a") {
		t.Fatal("the manager was replaced while the sweep was refusing:", l.Running())
	}

	// Meanwhile the channel moves on, which used to be the end of it.
	second := l.publish("c")
	if e := l.Schedule(false); e != nil {
		t.Fatal("the channel moving while a rollout waited wedged the installation:", e)
	}
	tx := l.Transaction()
	if tx.Release.Revision != second.Revision {
		t.Fatalf("the rollout still aims at the superseded release: %+v", tx)
	}
	if tx.State != "staged" {
		t.Fatalf("the re-staged transaction is not a reservation: %+v", tx)
	}

	// And once the session settles, the installation lands on the new release.
	l.Refuse(nil)
	if e := l.Helper(); e != nil {
		t.Fatal("the rollout did not complete once the sweep was happy:", e)
	}
	if got := l.Transaction().State; got != "healthy" {
		t.Fatal("the completed rollout is recorded as", got)
	}
	if l.Running() != second.Image {
		t.Fatal("the installation is running", l.Running(), "rather than", second.Image)
	}
}

// Forcing is the lever that did not exist: the sweep keeps refusing, nobody
// stops any sessions, and the rollout still lands. See cxz#253.
func TestAForcedRolloutLandsWhileTheSweepKeepsRefusing(t *testing.T) {
	l := newRolloutLab(t)
	r := l.current()
	l.Refuse(fmt.Errorf("session 4d683e9c: agent not idle"))
	if e := l.Schedule(true); e != nil {
		t.Fatal(e)
	}
	if !l.Transaction().Force {
		t.Fatal("the transaction does not remember that it was forced")
	}
	if e := l.Helper(); e != nil {
		t.Fatal("a forced rollout was refused:", e)
	}
	if l.Running() != r.Image || l.Transaction().State != "healthy" {
		t.Fatalf("the forced rollout did not land: running %s, %+v", l.Running(), l.Transaction())
	}
	// It asked for the forced handover rather than the ordinary one, and only
	// once: a force that silently falls back to the sweep would pass the test
	// above by accident.
	for _, action := range l.Prepares() {
		if action != "prepare-force" {
			t.Fatal("a forced rollout asked for", action)
		}
	}
}

// Work that has actually begun is not a reservation. Anything past staging has
// touched the old manager, and discarding it for a newer release would leave
// two managers half-swapped -- so that case still asks for recovery, which is
// what cxz#221 built the rollback for.
func TestAHalfAppliedRolloutIsNotDiscardedForANewerRelease(t *testing.T) {
	l := newRolloutLab(t)
	if e := l.Schedule(false); e != nil {
		t.Fatal(e)
	}
	tx := l.Transaction()
	tx.State = "starting"
	if e := core.WriteJSON(ManagerTransactionPath(l.Root), tx); e != nil {
		t.Fatal(e)
	}
	l.publish("d")
	e := l.Schedule(false)
	if e == nil {
		t.Fatal("a half-applied rollout was thrown away for a newer release")
	}
	if !strings.Contains(e.Error(), "needs recovery") {
		t.Fatal("the refusal does not say what is wrong:", e)
	}
	if got := l.Transaction(); got.State != "starting" || got.Release.Revision != tx.Release.Revision {
		t.Fatalf("the half-applied transaction was rewritten: %+v", got)
	}
}

// A transaction nobody can finish is rolled back rather than kept forever: its
// old manager is gone, so ReplaceManager would resolve tx.OldID, fail, and
// write "staged" back on every attempt. A manual self-update is enough to get
// here, because it replaces the container a staged transaction points at.
// See cxz#221.
func TestATransactionWhoseOldManagerIsGoneRollsBack(t *testing.T) {
	l := newRolloutLab(t)
	if e := l.Schedule(false); e != nil {
		t.Fatal(e)
	}
	before := l.Transaction()
	if before.State != "staged" {
		t.Fatalf("%+v", before)
	}
	// Somebody replaced the manager by hand: the container the transaction
	// names no longer exists, and a new one is running in its place.
	f, e := readDockerFixture(l.docker)
	if e != nil {
		t.Fatal(e)
	}
	replacement := f.Containers["old"]
	replacement.ID = "by-hand"
	replacement.Name = "/manager"
	delete(f.Containers, "old")
	f.Containers["by-hand"] = replacement
	l.save(f)

	l.publish("e")
	if e := l.Schedule(false); e != nil {
		t.Fatal("an unfinishable transaction was not let go:", e)
	}
	tx := l.Transaction()
	if tx.ID == before.ID {
		t.Fatal("the dead transaction was reused")
	}
	if tx.State != "staged" || tx.OldID != "by-hand" {
		t.Fatalf("the new transaction does not point at the manager that is actually running: %+v", tx)
	}
}

// While a helper is retrying the release that is current, its transaction is
// left exactly as it is. Rewriting it would hand the helper a lease it does not
// hold, and the gate would answer "another update owns this server" until the
// old lease expired ten minutes later.
func TestAStagedTransactionForTheCurrentReleaseIsLeftAlone(t *testing.T) {
	l := newRolloutLab(t)
	l.Refuse(fmt.Errorf("session 5aab0fb3: waiting for 5 minutes of continuous idle"))
	if e := l.Schedule(false); e != nil {
		t.Fatal(e)
	}
	first := l.Transaction()
	if e := l.Helper(); e == nil {
		t.Fatal("a refusing sweep let the manager be replaced")
	}
	// The tick comes round again with the same release on the channel.
	if e := l.Schedule(false); e != nil {
		t.Fatal(e)
	}
	if got := l.Transaction(); got.ID != first.ID || got.Release.Revision != first.Release.Revision {
		t.Fatalf("the helper's own transaction was rewritten underneath it: %+v", got)
	}
}
