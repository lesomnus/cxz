package cxzupdate

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

	"github.com/lesomnus/cxz/internal/dockerx"
)

// ContainerHealth uses the private maintenance socket as the container user.
// Docker ownership is checked by callers against their installation/project.
func ContainerHealth(ctx context.Context, container, user, root, action, tx string) (Health, error) {
	var h Health
	args := []string{"exec"}
	if user != "" {
		args = append(args, "--user", user)
	}
	args = append(args, container, "/cxz/tools/cxz", "_maintenance", root, action, tx)
	b, e := dockerx.Run(ctx, args...)
	if e != nil {
		return h, e
	}
	e = json.Unmarshal(b, &h)
	return h, e
}
func dockerSocket() (string, error) {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		return "/var/run/docker.sock", nil
	}
	if !strings.HasPrefix(host, "unix://") {
		return "", fmt.Errorf("automatic manager replacement requires a local Unix Docker endpoint")
	}
	return strings.TrimPrefix(host, "unix://"), nil
}
func dockerRequest(ctx context.Context, method, path string, in, out any) error {
	sock, e := dockerSocket()
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

// CloneManager preserves the inspected container's complete Config and
// HostConfig. Only immutable image/build metadata and allocated network identity
// change. No project container is recreated by this operation.
func CloneManager(ctx context.Context, oldID, name, owner, image string) (string, error) {
	var inspected struct {
		ID              string `json:"Id"`
		Config          map[string]any
		HostConfig      map[string]any
		NetworkSettings struct {
			Networks map[string]struct{ Aliases []string }
		}
	}
	if e := dockerRequest(ctx, "GET", "/containers/"+url.PathEscape(oldID)+"/json", nil, &inspected); e != nil {
		return "", e
	}
	labels, ok := inspected.Config["Labels"].(map[string]any)
	if !ok || labels["cxz.owner"] != owner || labels["cxz.role"] != "daemon" {
		return "", fmt.Errorf("manager ownership mismatch")
	}
	inspected.Config["Image"] = image
	if env, ok := inspected.Config["Env"].([]any); ok {
		for i, v := range env {
			if s, ok := v.(string); ok && strings.HasPrefix(s, "CXZ_MANAGER_IMAGE=") {
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
	if e := dockerRequest(ctx, "POST", "/containers/create?name="+url.QueryEscape(name), inspected.Config, &created); e != nil {
		return "", e
	}
	return created.ID, nil
}
