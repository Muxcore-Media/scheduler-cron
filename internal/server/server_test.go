package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	json.NewDecoder(w.Body).Decode(&resp)

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
	json.NewDecoder(w.Body).Decode(&resp)

	// Check status.
	req2 := httptest.NewRequest(http.MethodGet, "/status/"+resp.TaskID, nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET /status: %d, body: %s", w2.Code, w2.Body.String())
	}

	var task struct {
		Name  string `json:"name"`
		ID    string `json:"id"`
		Status string `json:"status"`
	}
	json.NewDecoder(w2.Body).Decode(&task)
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
	json.NewDecoder(w.Body).Decode(&tasks)
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}
