package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestTLSProxyOrigin(t *testing.T) {
	token := strings.Repeat("t", 32)
	ok := Config{Listen: "0.0.0.0:7350", Origin: "https://alice.cxz.example", Token: token, TLSProxy: true}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]Config{
		"an http origin":     {Listen: ":7350", Origin: "http://alice.cxz.example", Token: token, TLSProxy: true},
		"its own cert too":   {Listen: ":7350", Origin: "https://alice.cxz.example", Token: token, TLSProxy: true, Certificate: "c.pem", Key: "k.pem"},
		"https with no cert": {Listen: ":7350", Origin: "https://alice.cxz.example", Token: token},
	} {
		if c.Validate() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	h, stop, err := Handler(ok, nil, fstest.MapFS{"index.html": {Data: []byte("ui")}}, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://gateway/", nil))
	// The browser reached it over the proxy's TLS: the same promise as a
	// gateway's own certificate.
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("no HSTS behind a TLS proxy")
	}
}
