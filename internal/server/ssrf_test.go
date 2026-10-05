package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
)

var unsafeHooks = []string{
	"http://127.0.0.1:8080/hook",
	"http://localhost/hook",
	"http://10.0.0.5/hook",
	"http://192.168.1.1/hook",
	"http://172.16.0.1/hook",
	"http://169.254.169.254/latest/meta-data/",
	"http://[::1]/hook",
	"http://[fd00::1]/hook",
	"http://metadata.google.internal/",
	"http://0x7f000001/",
	"file:///etc/passwd",
	"gopher://example.com/",
	"http://user:pw@example.com/",
}

func TestScheduleRejectsUnsafeWebhook(t *testing.T) {
	for _, u := range unsafeHooks {
		for _, where := range []string{"meta", "payload"} {
			srv := newGuardedTestServer(t)
			req := map[string]any{"name": "n", "cron_expr": "0 0 * * *"}
			if where == "meta" {
				req["meta"] = map[string]any{"webhook_url": u}
			} else {
				req["payload"] = []byte(`{"webhook_url":"` + u + `"}`)
			}
			b, _ := json.Marshal(req)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/schedule", bytes.NewReader(b)))
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s %q: status %d, want 400", where, u, w.Code)
			}
		}
	}
}

func TestScheduleAcceptsPublicWebhook(t *testing.T) {
	srv := newGuardedTestServer(t)
	b, _ := json.Marshal(map[string]any{"name": "n", "cron_expr": "0 0 * * *",
		"meta": map[string]any{"webhook_url": "https://hooks.example.com/x"}})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/schedule", bytes.NewReader(b)))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}

// Tasks restored from disk skip /schedule validation; fire-time must still block.
func TestFireBlocksUnsafeWebhook(t *testing.T) {
	reached := false
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer hook.Close()
	srv := newGuardedTestServer(t)
	for _, u := range append([]string{hook.URL}, unsafeHooks[:6]...) {
		err := srv.postWebhook(u, &cronstore.Task{ID: "t", Name: "n"})
		if err == nil || !strings.Contains(err.Error(), "blocked") {
			t.Errorf("postWebhook(%q) = %v, want blocked", u, err)
		}
	}
	if reached {
		t.Fatal("request reached loopback server")
	}
}
