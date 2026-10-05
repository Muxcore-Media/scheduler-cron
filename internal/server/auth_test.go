package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:9200": true, "[::1]:9200": true, "localhost:9200": true,
		":9200": false, "0.0.0.0:9200": false, "[::]:9200": false, "10.0.0.5:9200": false, "bogus": false,
	} {
		if got := IsLoopbackAddr(addr); got != want {
			t.Errorf("%q: got %v want %v", addr, got, want)
		}
	}
}

func TestValidateListen(t *testing.T) {
	if err := ValidateListen(":9200", ""); err == nil {
		t.Fatal("expected error for non-loopback without token")
	}
	if err := ValidateListen(":9200", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateListen("127.0.0.1:9200", ""); err != nil {
		t.Fatal(err)
	}
}

func TestRequireToken(t *testing.T) {
	h := RequireToken("s3cret", newTestServer(t).Handler())
	do := func(method, path, auth string) int {
		r := httptest.NewRequest(method, path, nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if c := do(http.MethodGet, "/health", ""); c != 200 {
		t.Fatalf("health: %d", c)
	}
	for _, p := range []string{"/list", "/metrics", "/status/x", "/schedule"} {
		if c := do(http.MethodGet, p, ""); c != 401 {
			t.Errorf("%s no token: %d", p, c)
		}
		if c := do(http.MethodGet, p, "Bearer wrong"); c != 401 {
			t.Errorf("%s wrong token: %d", p, c)
		}
	}
	if c := do(http.MethodDelete, "/cancel/x", ""); c != 401 {
		t.Errorf("cancel no token: %d", c)
	}
	if c := do(http.MethodGet, "/metrics", "Bearer s3cret"); c != 200 {
		t.Fatalf("metrics with token: %d", c)
	}
}
