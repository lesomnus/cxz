package quotashare

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

const Socket = "/cxz/tools/quota.sock"

func Start(root, socket string, authorize func(context.Context, Request, string) bool) (io.Closer, error) {
	if st, err := os.Lstat(socket); err == nil {
		if st.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("quota path is not a socket")
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
	srv := &http.Server{ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 5 * time.Second}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if r.Method != "POST" || r.URL.Path != "/quota" || json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&req) != nil || !authorize(r.Context(), req, r.Header.Get("Authorization")) {
			http.Error(w, "unauthorized quota request", http.StatusForbidden)
			return
		}
		result, err := Exchange(root, req, time.Now())
		if err != nil {
			http.Error(w, "quota unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(result)
	})
	go srv.Serve(ln)
	return srv, nil
}
func Remote(ctx context.Context, socket, token string, r Request) (Response, error) {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	raw, err := json.Marshal(r)
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://quota/quota", bytes.NewReader(raw))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", token)
	res, err := (&http.Client{Transport: transport, Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return Response{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("quota coordinator: %s", res.Status)
	}
	var out Response
	err = json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&out)
	return out, err
}
