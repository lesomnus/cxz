package webui

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	pdweb "github.com/lesomnus/payday/web"
	"google.golang.org/grpc"
)

const cookieName = "__Host-cxz"
const lifetime = 12 * time.Hour

type Config struct{ Listen, Origin, Certificate, Key, Token string }

func (c Config) Validate() error {
	u, err := url.Parse(c.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("origin must be https://host[:port] without a path")
	}
	if c.Listen == "" || c.Certificate == "" || c.Key == "" {
		return fmt.Errorf("listen, tls-cert and tls-key are required")
	}
	if len(c.Token) < 32 {
		return fmt.Errorf("web access token must contain at least 32 bytes")
	}
	return nil
}

type browserSession struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type browserAuth struct {
	origin   string
	token    [32]byte
	mu       sync.Mutex
	sessions map[[32]byte]browserSession
}

// Handler uses payday's transcoder; all RPCs still run in the installed Manager.
// The returned close function stops gateway streams, never the Manager/agents.
func Handler(c Config, conn grpc.ClientConnInterface, assets fs.FS) (http.Handler, func(), error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	g := grpc.NewServer(grpc.MaxRecvMsgSize(8<<20), grpc.MaxSendMsgSize(24<<20))
	resource.RegisterProjectServiceServer(g, &projects{client: resource.NewProjectServiceClient(conn)})
	resource.RegisterSessionServiceServer(g, &sessions{client: resource.NewSessionServiceClient(conn)})
	mux, err := pdweb.New(config.HttpConfig{AllowWeb: true}, g)
	if err != nil {
		g.Stop()
		return nil, nil, err
	}
	mux.Handle("/", http.FileServer(http.FS(assets)))
	auth := &browserAuth{origin: c.Origin, token: sha256.Sum256([]byte(c.Token)), sessions: make(map[[32]byte]browserSession)}
	return auth.wrap(mux), func() {
		auth.mu.Lock()
		for k := range auth.sessions {
			auth.revoke(k)
		}
		auth.mu.Unlock()
		g.Stop()
	}, nil
}

func (a *browserAuth) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		// Exact host and origin binding also protects authenticated Connect GETs.
		u, _ := url.Parse(a.origin)
		if r.Host != u.Host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != a.origin) {
			http.Error(w, "origin refused", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Origin") != a.origin {
			http.Error(w, "origin required", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/auth/login" && r.Method == http.MethodPost {
			a.login(w, r)
			return
		}
		if r.URL.Path == "/auth/logout" && r.Method == http.MethodPost {
			a.mu.Lock()
			if c, e := r.Cookie(cookieName); e == nil {
				a.revoke(sha256.Sum256([]byte(c.Value)))
			}
			a.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if pdweb.Rpc(r) || strings.HasPrefix(r.URL.Path, "/cxz.") || r.URL.Path == "/auth/status" {
			c, err := r.Cookie(cookieName)
			if err != nil {
				http.Error(w, "sign in required", http.StatusUnauthorized)
				return
			}
			session, ok := a.session(c.Value)
			if !ok {
				http.Error(w, "sign in required", http.StatusUnauthorized)
				return
			}
			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()
			stop := context.AfterFunc(session.ctx, cancel)
			defer stop()
			r = r.WithContext(ctx)
			if r.URL.Path == "/auth/status" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// revoke is called with the session mutex held and cancels active streams too.
func (a *browserAuth) revoke(key [32]byte) {
	if s, ok := a.sessions[key]; ok {
		s.cancel()
		delete(a.sessions, key)
	}
}
func (a *browserAuth) session(value string) (browserSession, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := sha256.Sum256([]byte(value))
	s, ok := a.sessions[key]
	if !ok {
		return browserSession{}, false
	}
	if s.ctx.Err() != nil {
		a.revoke(key)
		return browserSession{}, false
	}
	return s, true
}
func (a *browserAuth) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "invalid login", 400)
		return
	}
	digest := sha256.Sum256([]byte(req.Token))
	if subtle.ConstantTimeCompare(digest[:], a.token[:]) != 1 {
		http.Error(w, "invalid token", 401)
		return
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		http.Error(w, "session unavailable", 500)
		return
	}
	value := hex.EncodeToString(raw[:])
	a.mu.Lock()
	now := time.Now()
	for k, t := range a.sessions {
		if t.ctx.Err() != nil {
			a.revoke(k)
		}
	}
	if len(a.sessions) >= 128 {
		a.mu.Unlock()
		http.Error(w, "too many browser sessions; sign out or wait for expiry", 429)
		return
	}
	if c, e := r.Cookie(cookieName); e == nil {
		a.revoke(sha256.Sum256([]byte(c.Value)))
	}
	sessionCtx, cancel := context.WithDeadline(context.Background(), now.Add(lifetime))
	a.sessions[sha256.Sum256([]byte(value))] = browserSession{sessionCtx, cancel}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(lifetime.Seconds())})
	w.WriteHeader(http.StatusNoContent)
}

func Serve(ctx context.Context, c Config, conn grpc.ClientConnInterface, assets fs.FS) error {
	h, closeHandler, err := Handler(c, conn, assets)
	if err != nil {
		return err
	}
	defer closeHandler()
	s := &http.Server{Addr: c.Listen, Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-done:
		}
	}()
	err = s.ListenAndServeTLS(c.Certificate, c.Key)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
