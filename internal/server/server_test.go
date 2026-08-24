package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := cronstore.New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(store.Stop)
	return New(store)
}

func TestSchedule(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.Handler()

	body, _ := json.Marshal(map[string]string{
		"name":      "test-task",
		"cron_expr": "* * * * *",
	})
	req := httptest.NewRequest(http.MethodPost, "/schedule", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /schedule: %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TaskID == "" {
		t.Fatal("expected non-empty task_id")
	}
}

func TestSchedule_MissingFields(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.Handler()

	body, _ := json.Marshal(map[string]string{"name": "test"})
	req := httptest.NewRequest(http.MethodPost, "/schedule", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSchedule_WrongMethod(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/schedule", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestCancel(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.Handler()

	// Schedule a task.
	body, _ := json.Marshal(map[string]string{
		"name":      "cancel-me",
		"cron_expr": "* * * * *",
	})
	req := httptest.NewRequest(http.MethodPost, "/schedule", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var resp struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Cancel it.
	req2 := httptest.NewRequest(http.MethodDelete, "/cancel/"+resp.TaskID, nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("DELETE /cancel: %d, body: %s", w2.Code, w2.Body.String())
	}

	// Cancel again - should 404.
	req3 := httptest.NewRequest(http.MethodDelete, "/cancel/"+resp.TaskID, nil)
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for second cancel, got %d", w3.Code)
	}
}

func TestStatus(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.Handler()

	// Schedule a task.
	body, _ := json.Marshal(map[string]string{
		"name":      "status-check",
		"cron_expr": "0 0 * * *",
	})
	req := httptest.NewRequest(http.MethodPost, "/schedule", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var resp struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Check status.
	req2 := httptest.NewRequest(http.MethodGet, "/status/"+resp.TaskID, nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET /status: %d, body: %s", w2.Code, w2.Body.String())
	}

	var task struct {
		Name   string `json:"name"`
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&task); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	if task.Name != "status-check" {
		t.Errorf("Name = %q, want %q", task.Name, "status-check")
	}
	if task.Status != "scheduled" {
		t.Errorf("Status = %q, want %q", task.Status, "scheduled")
	}
}

func TestList(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.Handler()

	// Schedule two tasks.
	for _, name := range []string{"task-a", "task-b"} {
		body, _ := json.Marshal(map[string]string{
			"name":      name,
			"cron_expr": "0 0 * * *",
		})
		req := httptest.NewRequest(http.MethodPost, "/schedule", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("schedule %s: %d", name, w.Code)
		}
	}

	// List all.
	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /list: %d", w.Code)
	}
	var tasks []any
	if err := json.NewDecoder(w.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

type recordingPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	Type    string
	Source  string
	Payload []byte
}

func (p *recordingPublisher) Publish(ctx context.Context, eventType, source string, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, publishedEvent{Type: eventType, Source: source, Payload: append([]byte(nil), payload...)})
	return nil
}

func (p *recordingPublisher) last() (publishedEvent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) == 0 {
		return publishedEvent{}, false
	}
	return p.events[len(p.events)-1], true
}

func TestOnFire_UpdatesStatusAndPublishes(t *testing.T) {
	srv := newTestServer(t)
	pub := &recordingPublisher{}
	srv.SetEventPublisher(pub, "scheduler-cron")

	id, err := srv.store().Add("fire-me", "0 0 * * *", []byte(`{"k":"v"}`), 0, map[string]any{"tag": "x"}, srv.onFire)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	srv.onFire(id)

	task, err := srv.store().Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if task.Status != "completed" {
		t.Fatalf("Status = %q, want completed", task.Status)
	}

	pub.mu.Lock()
	types := make([]string, len(pub.events))
	for i, e := range pub.events {
		types[i] = e.Type
	}
	pub.mu.Unlock()
	if len(types) < 2 {
		t.Fatalf("events = %v, want fired+completed", types)
	}
	if types[0] != "scheduler.task.fired" {
		t.Errorf("first event = %q", types[0])
	}
	if types[len(types)-1] != "scheduler.task.completed" {
		t.Errorf("last event = %q", types[len(types)-1])
	}
	ev, ok := pub.last()
	if !ok {
		t.Fatal("expected published event")
	}
	if ev.Source != "scheduler-cron" {
		t.Errorf("source = %q, want scheduler-cron", ev.Source)
	}
	var payload map[string]any
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if payload["task_id"] != id {
		t.Errorf("payload task_id = %v, want %s", payload["task_id"], id)
	}
	if payload["name"] != "fire-me" {
		t.Errorf("payload name = %v, want fire-me", payload["name"])
	}
}

func TestOnFire_Webhook(t *testing.T) {
	srv := newTestServer(t)

	var gotBody map[string]any
	var gotMethod string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	id, err := srv.store().Add("hook-me", "0 0 * * *", nil, 0, map[string]any{
		"webhook_url": hook.URL,
	}, srv.onFire)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	srv.onFire(id)

	if gotMethod != http.MethodPost {
		t.Fatalf("webhook method = %q, want POST", gotMethod)
	}
	if gotBody["task_id"] != id {
		t.Errorf("webhook task_id = %v, want %s", gotBody["task_id"], id)
	}
	if gotBody["event"] != "scheduler.task.fired" {
		t.Errorf("webhook event = %v", gotBody["event"])
	}

	task, err := srv.store().Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if task.Status != "completed" {
		t.Fatalf("Status = %q, want completed", task.Status)
	}
}

func TestOnFire_WebhookFailureMarksFailed(t *testing.T) {
	srv := newTestServer(t)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer hook.Close()

	id, err := srv.store().Add("fail-hook", "0 0 * * *", nil, 0, map[string]any{
		"webhook_url": hook.URL,
	}, srv.onFire)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	srv.onFire(id)

	task, err := srv.store().Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if task.Status != "failed" {
		t.Fatalf("Status = %q, want failed", task.Status)
	}
}

func TestOnFire_WebhookFromPayload(t *testing.T) {
	srv := newTestServer(t)
	called := false
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hook.Close()

	payload, _ := json.Marshal(map[string]string{"webhook_url": hook.URL})
	id, err := srv.store().Add("payload-hook", "0 0 * * *", payload, 0, nil, srv.onFire)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	srv.onFire(id)
	if !called {
		t.Fatal("expected webhook from payload")
	}
}

func TestOnFire_WebhookTimeout(t *testing.T) {
	srv := newTestServer(t)
	pub := &recordingPublisher{}
	srv.SetEventPublisher(pub, "scheduler-cron")

	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	id, err := srv.store().Add("slow-hook", "0 0 * * *", nil, 50*time.Millisecond, map[string]any{
		"webhook_url": hook.URL,
	}, srv.onFire)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	srv.onFire(id)

	task, err := srv.store().Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if task.Status != "timeout" {
		t.Fatalf("Status = %q, want timeout", task.Status)
	}
	ev, ok := pub.last()
	if !ok || ev.Type != "scheduler.task.timeout" {
		t.Fatalf("last event = %+v, want scheduler.task.timeout", ev)
	}
}

func TestSchedule_ParsesTimeoutAndOnce(t *testing.T) {
	srv := newTestServer(t)
	body := `{"name":"t","cron_expr":"@once","timeout":"2s","once":true}`
	req := httptest.NewRequest(http.MethodPost, "/schedule", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	// one-shot removes shortly after fire
	time.Sleep(100 * time.Millisecond)
	if srv.store().Len() != 0 {
		t.Fatalf("expected one-shot removed, len=%d", srv.store().Len())
	}
}
