package conversation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

const Socket = "/cxz/tools/conversation-registry.sock"

type RegistryRequest struct {
	Project string `json:"project"`
	Caller  string `json:"caller"`
}

// Only session metadata crosses this authenticated, project-scoped endpoint.
// Journals and exported bodies stay inside the project runtime.
func StartRegistry(socket string, authorize func(context.Context, RegistryRequest, string) bool, list func(context.Context, string) ([]Session, error)) (io.Closer, error) {
	if st, err := os.Lstat(socket); err == nil {
		if st.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("registry path is not a socket")
		}
		if err = os.Remove(socket); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	ln, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(socket, 0666); err != nil {
		ln.Close()
		return nil, err
	}
	srv := &http.Server{ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 5 * time.Second}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q RegistryRequest
		if r.Method != "POST" || r.URL.Path != "/sessions" || json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&q) != nil || !authorize(r.Context(), q, r.Header.Get("Authorization")) {
			http.Error(w, "unauthorized registry request", http.StatusForbidden)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		sessions, err := list(ctx, q.Project)
		if err != nil {
			http.Error(w, "session registry unavailable", http.StatusServiceUnavailable)
			return
		}
		// Runtime IDs are internal routing data, never returned by MCP itself.
		type entry struct {
			Session
			Runtime string `json:"runtime_id"`
			Project string `json:"project_id"`
		}
		var entries []entry
		for _, s := range sessions {
			if s.ProjectID == q.Project {
				entries = append(entries, entry{s, s.RuntimeID, s.ProjectID})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(entries)
	})
	go srv.Serve(ln)
	return srv, nil
}
func RemoteRegistry(ctx context.Context, socket, token string, q RegistryRequest) ([]Session, error) {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer tr.CloseIdleConnections()
	b, _ := json.Marshal(q)
	req, err := http.NewRequestWithContext(ctx, "POST", "http://registry/sessions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", token)
	res, err := (&http.Client{Transport: tr, Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("Manager conversation registry unavailable; update the Manager: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conversation registry: %s", res.Status)
	}
	var entries []struct {
		Session
		Runtime string `json:"runtime_id"`
		Project string `json:"project_id"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&entries); err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(entries))
	for _, v := range entries {
		v.Session.RuntimeID = v.Runtime
		v.Session.ProjectID = v.Project
		out = append(out, v.Session)
	}
	return out, nil
}
