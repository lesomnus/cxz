package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/kube"
	"github.com/lesomnus/cxz/internal/projectspec"
)

// A project pod's runtime is the manager's image at the moment the pod was
// made: the tools init container copies cxz out of it. The manager itself is
// never updated by cxz on Kubernetes -- its version is the image tag of its
// Deployment, which is the deployment's to change -- but once it has been, the
// pods it made earlier still run the previous runtime. Carrying that change
// down to them is a controller's work, the way a Deployment replaces its pods
// when its template changes: the version was declared by a person, cxz only
// propagates it.
//
// A pod is replaced when every session in it has been idle for the update
// idle period, under the runtime's maintenance lease so nothing starts in
// between, and the sessions that were running are resumed afterwards. A pod
// whose runtime speaks another protocol is replaced without waiting: the
// manager cannot be relied on to talk to it. "manual" leaves both to a person
// and only reports them.

const (
	rolloutIdle   = "idle"
	rolloutManual = "manual"

	annotationRevision = "cxz.revision"
	annotationProtocol = "cxz.protocol"
)

// kubeRolloutInterval is how often outdated pods are looked for; a refusal
// (sessions not idle) is simply tried again on the next pass.
var kubeRolloutInterval = time.Minute

type runtimeAge int

const (
	runtimeCurrent runtimeAge = iota
	// runtimeOutdated is an older manager's runtime that this one still talks to.
	runtimeOutdated
	// runtimeIncompatible is a runtime of another protocol.
	runtimeIncompatible
)

// kubeRuntimeAge compares a pod's runtime with this manager's. The tools image
// is what the runtime was copied from, by digest when the node reported one;
// a pod made before the protocol annotation existed is taken to speak this
// protocol, as every runtime so far has.
func (m *Manager) kubeRuntimeAge(pod *kubePod) runtimeAge {
	if pod == nil {
		return runtimeCurrent
	}
	if v, ok := pod.Metadata.Annotations[annotationProtocol]; ok && v != strconv.Itoa(cxzupdate.Protocol) {
		return runtimeIncompatible
	}
	for _, c := range pod.Spec.InitContainers {
		if c.Name == "tools" && c.Image != m.kubeSettings.ToolsImage {
			return runtimeOutdated
		}
	}
	return runtimeCurrent
}

// RunKubeRollout replaces outdated project pods for as long as ctx lasts.
func (m *Manager) RunKubeRollout(ctx context.Context) {
	if m.kube == nil || m.kubeSettings.RuntimeRollout == rolloutManual {
		return
	}
	tick := time.NewTicker(kubeRolloutInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		m.kubeRolloutStep(ctx)
	}
}

// kubeRolloutStep replaces at most one pod, so a manager update does not
// restart every project at once.
func (m *Manager) kubeRolloutStep(ctx context.Context) {
	all, err := m.all(ctx)
	if err != nil {
		return
	}
	for _, p := range all {
		replaced, err := m.kubeRollout(ctx, p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "runtime rollout: project %s: %v\n", p.Name, err)
		}
		if replaced {
			return
		}
	}
}

// kubeRollout replaces p's pod when it runs an older runtime and may be
// replaced now. It reports whether it replaced it; a refusal because the
// project is busy is not an error.
func (m *Manager) kubeRollout(ctx context.Context, p *Project) (bool, error) {
	lock := m.projectLock(p.ID)
	if !tryLockFor(lock, time.Second) {
		return false, nil // someone is starting, stopping or building it
	}
	resume, err := m.kubeReplaceOutdated(ctx, p)
	lock.Unlock()
	if err != nil || resume == nil {
		return false, err
	}
	// The sessions that were running before are running again after; a
	// session stopped by a person stays stopped. Their agent CLIs are then
	// refreshed, as a start does.
	agents := map[string]bool{}
	defer func() {
		for agent := range agents {
			go m.kubePrepareAgent(p.ID, agent)
		}
	}()
	for id := range resume {
		conn, client, err := m.client(ctx, p)
		if err != nil {
			return true, err
		}
		v, err := client.Get(ctx, &api.SessionRef{Id: id})
		conn.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "runtime rollout: session %s: %v\n", id, err)
			continue
		}
		if v.Agent != "" {
			agents[v.Agent] = true
		}
		if _, err = m.ResumeSession(ctx, &api.Control{SessionId: id, RunId: v.RunId, ClientId: core.ID()}); err != nil {
			fmt.Fprintf(os.Stderr, "runtime rollout: resume session %s: %v\n", id, err)
		}
	}
	return true, nil
}

// kubeReplaceOutdated is the replacement itself, under the project lock. It
// returns the sessions to resume, or nil when it replaced nothing.
func (m *Manager) kubeReplaceOutdated(ctx context.Context, p *Project) (map[string]string, error) {
	p, err := m.resolve(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	pod, err := m.kubePod(ctx, p)
	if err != nil || pod == nil || !pod.ready() {
		return nil, err
	}
	age := m.kubeRuntimeAge(pod)
	if age == runtimeCurrent {
		return nil, nil
	}
	// The lease closes admission until the pod is gone, so no turn starts
	// between the check and the replacement.
	action, why := "prepare", "its runtime is an older cxz than the manager"
	if age == runtimeIncompatible {
		action, why = "prepare-force", "its runtime speaks another cxz protocol"
	}
	h, err := m.kubeMaintenance(ctx, p, action, core.ID())
	if err != nil || !h.Ready {
		return nil, nil // not idle yet; the next pass asks again
	}
	from := pod.Metadata.Annotations[annotationRevision]
	if from == "" {
		from = "an earlier build"
	}
	m.kubeEvent(ctx, p, "RuntimeReplaced", fmt.Sprintf("cxz replaced project %s's pod: %s (runtime %s, manager %s)", p.Name, why, from, cxzupdate.Current().Revision))
	if err = m.kubeDeletePod(ctx, p); err != nil {
		return nil, err
	}
	if err = m.kubeUp(ctx, p); err != nil {
		p.Error = err.Error()
		_ = m.save(context.WithoutCancel(ctx), p)
		return nil, err
	}
	p.Error = ""
	if err = m.save(ctx, p); err != nil {
		return nil, err
	}
	if h.Sessions == nil {
		h.Sessions = map[string]string{}
	}
	return h.Sessions, nil
}

// kubeMaintenance asks the project runtime's maintenance endpoint, as the
// Docker backend's ContainerHealth does through docker exec.
func (m *Manager) kubeMaintenance(ctx context.Context, p *Project, action, tx string) (cxzupdate.Health, error) {
	var h cxzupdate.Health
	args := []string{"--user", p.RemoteUser, p.ContainerID, kube.RuntimeBinary, "_maintenance", projectspec.StateDir, action, tx}
	b, err := m.containerCommand(ctx, args...).Output()
	if err != nil {
		return h, err
	}
	return h, json.Unmarshal(b, &h)
}

// kubeEvent records what cxz did to a project's pod where `kubectl describe`
// and `kubectl get events` show it. An event that cannot be written is not a
// reason to stop.
func (m *Manager) kubeEvent(ctx context.Context, p *Project, reason, message string) {
	name := m.kubeResource(p)
	now := time.Now().UTC().Format(time.RFC3339)
	event := map[string]any{
		"apiVersion": "v1", "kind": "Event",
		"metadata":       map[string]any{"name": fmt.Sprintf("%s.%x", name, time.Now().UnixNano()), "labels": m.kubeLabels(p)},
		"involvedObject": map[string]any{"kind": "Pod", "name": name, "namespace": m.kube.Namespace()},
		"reason":         reason, "message": message, "type": "Normal",
		"source":         map[string]any{"component": "cxz"},
		"firstTimestamp": now, "lastTimestamp": now, "count": 1,
	}
	if err := m.kube.Apply(ctx, m.kube.Path("events", event["metadata"].(map[string]any)["name"].(string)), event, nil); err != nil {
		fmt.Fprintf(os.Stderr, "kubernetes event %s: %v\n", reason, err)
	}
}
