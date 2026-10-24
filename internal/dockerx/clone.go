package dockerx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func Socket() (string, error) {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		return "/var/run/docker.sock", nil
	}
	if !strings.HasPrefix(host, "unix://") {
		return "", fmt.Errorf("automatic manager replacement requires a local Unix Docker endpoint")
	}
	return strings.TrimPrefix(host, "unix://"), nil
}
func Request(ctx context.Context, method, path string, in, out any) error {
	sock, e := Socket()
	if e != nil {
		return e
	}
	var body []byte
	if in != nil {
		body, e = json.Marshal(in)
		if e != nil {
			return e
		}
	}
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}
	defer tr.CloseIdleConnections()
	req, e := http.NewRequestWithContext(ctx, method, "http://docker"+path, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	res, e := (&http.Client{Transport: tr, Timeout: 45 * time.Second}).Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("Docker %s: %s", res.Status, string(b))
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out)
	}
	return nil
}

// CloneOwned preserves an installation container's full Config and HostConfig
// while changing its image and allocated network identity. Project containers
// are rejected. The caller owns stopping, readiness and rollback.
func CloneOwned(ctx context.Context, oldID, name, owner, role, image string) (string, error) {
	var inspected struct {
		ID              string `json:"Id"`
		Config          map[string]any
		HostConfig      map[string]any
		NetworkSettings struct {
			Networks map[string]struct{ Aliases []string }
		}
	}
	if e := Request(ctx, "GET", "/containers/"+url.PathEscape(oldID)+"/json", nil, &inspected); e != nil {
		return "", e
	}
	labels, ok := inspected.Config["Labels"].(map[string]any)
	if !ok || labels["cxz.owner"] != owner || labels["cxz.role"] != role || labels["cxz.project"] != nil && labels["cxz.project"] != "" {
		return "", fmt.Errorf("container ownership/role mismatch")
	}
	inspected.Config["Image"] = image
	if env, ok := inspected.Config["Env"].([]any); ok {
		for i, v := range env {
			if s, ok := v.(string); ok && role == "daemon" && strings.HasPrefix(s, "CXZ_MANAGER_IMAGE=") {
				env[i] = "CXZ_MANAGER_IMAGE=" + image
			}
		}
	}
	// Docker's default hostname is the old container ID. Let the engine allocate it.
	inspected.Config["Hostname"] = ""
	inspected.Config["HostConfig"] = inspected.HostConfig
	endpoints := map[string]any{}
	for network, ep := range inspected.NetworkSettings.Networks {
		if network == "bridge" || network == "host" || network == "none" {
			continue
		}
		aliases := []string{}
		for _, a := range ep.Aliases {
			if a != oldID && !strings.HasPrefix(oldID, a) {
				aliases = append(aliases, a)
			}
		}
		endpoints[network] = map[string]any{"Aliases": aliases}
	}
	if len(endpoints) > 0 {
		inspected.Config["NetworkingConfig"] = map[string]any{"EndpointsConfig": endpoints}
	}
	var created struct {
		ID string `json:"Id"`
	}
	if e := Request(ctx, "POST", "/containers/create?name="+url.QueryEscape(name), inspected.Config, &created); e != nil {
		return "", e
	}
	return created.ID, nil
}
