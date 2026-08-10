package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
)

// EventPublisher publishes scheduler lifecycle events. Optional.
type EventPublisher interface {
	Publish(ctx context.Context, eventType, source string, payload []byte) error
}

// Server provides an HTTP API for managing cron tasks.
// Endpoints:
//
//	POST   /schedule    — register a new task
//	DELETE /cancel/{id} — cancel a task
//	GET    /status/{id} — get task status
//	GET    /list        — list tasks (?name=filter)
type Server struct {
	store      *cronstore.Store
	mux        *http.ServeMux
	events     EventPublisher
	moduleID   string
	httpClient *http.Client
}

// New creates an HTTP server backed by the given cron store.
func New(store *cronstore.Store) *Server {
	s := &Server{
		store:    store,
		mux:      http.NewServeMux(),
		moduleID: "scheduler-cron",
		httpClient: &http.Client{
			// Per-request timeouts come from task Timeout via context.
			Timeout: 0,
		},
	}
	s.mux.HandleFunc("/schedule", s.handleSchedule)
	s.mux.HandleFunc("/cancel/", s.handleCancel)
	s.mux.HandleFunc("/status/", s.handleStatus)
	s.mux.HandleFunc("/list", s.handleList)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
	return s
}

// SetEventPublisher sets an optional events client for lifecycle events.
func (s *Server) SetEventPublisher(p EventPublisher, moduleID string) {
	s.events = p
	if moduleID != "" {
		s.moduleID = moduleID
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP scheduler_tasks_total Total scheduled tasks\n")
	fmt.Fprintf(w, "# TYPE scheduler_tasks_total gauge\n")
	fmt.Fprintf(w, "scheduler_tasks_total %d\n", s.store.Len())
}

// Handler returns the HTTP handler for mounting on a custom mux.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		Name     string         `json:"name"`
		CronExpr string         `json:"cron_expr"`
		Payload  []byte         `json:"payload,omitempty"`
		Timeout  string         `json:"timeout,omitempty"`
		Once     bool           `json:"once,omitempty"`
		Meta     map[string]any `json:"meta,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" || req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "name and cron_expr are required")
		return
	}

	var timeout time.Duration
	if req.Timeout != "" {
		d, err := time.ParseDuration(req.Timeout)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid timeout: "+err.Error())
			return
		}
		if d < 0 {
			writeError(w, http.StatusBadRequest, "timeout must be non-negative")
			return
		}
		timeout = d
	}

	id, err := s.store.AddWithOptions(req.Name, req.CronExpr, req.Payload, timeout, req.Meta, s.onFire, cronstore.AddOptions{Once: req.Once})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"task_id": id})
}

// onFire runs when a scheduled task triggers.
func (s *Server) onFire(taskID string) {
	task, err := s.store.Get(taskID)
	if err != nil {
		slog.Warn("cron fire: task missing", "task_id", taskID, "error", err)
		return
	}

	if err := s.store.SetStatus(taskID, "running"); err != nil {
		slog.Warn("cron fire: set running", "task_id", taskID, "error", err)
	}

	s.publish(task, "scheduler.task.fired", nil)

	webhookURL := webhookURLFrom(task)
	fireErr := s.postWebhook(webhookURL, task)

	status := "completed"
	eventType := "scheduler.task.completed"
	if fireErr != nil {
		if isTimeout(fireErr) {
			status = "timeout"
			eventType = "scheduler.task.timeout"
		} else {
			status = "failed"
			eventType = "scheduler.task.failed"
		}
		slog.Warn("cron fire: webhook failed", "task_id", taskID, "error", fireErr, "status", status)
	}
	if err := s.store.SetStatus(taskID, status); err != nil {
		slog.Warn("cron fire: set status", "task_id", taskID, "status", status, "error", err)
	}
	extra := map[string]any{}
	if fireErr != nil {
		extra["error"] = fireErr.Error()
	}
	s.publish(task, eventType, extra)
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout")
}

func (s *Server) publish(task *cronstore.Task, eventType string, extra map[string]any) {
	if s.events == nil || task == nil {
		return
	}
	payload := map[string]any{
		"task_id":   task.ID,
		"name":      task.Name,
		"cron_expr": task.CronExpr,
		"payload":   task.Payload,
		"meta":      task.Meta,
	}
	for k, v := range extra {
		payload[k] = v
	}
	b, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.events.Publish(ctx, eventType, s.moduleID, b); err != nil {
		slog.Warn("cron fire: publish event", "task_id", task.ID, "type", eventType, "error", err)
	}
}

func webhookURLFrom(task *cronstore.Task) string {
	if task.Meta != nil {
		if v, ok := task.Meta["webhook_url"]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	if len(task.Payload) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return ""
	}
	if v, ok := payload["webhook_url"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (s *Server) postWebhook(url string, task *cronstore.Task) error {
	if url == "" {
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"task_id":   task.ID,
		"name":      task.Name,
		"cron_expr": task.CronExpr,
		"payload":   task.Payload,
		"meta":      task.Meta,
		"event":     "scheduler.task.fired",
	})
	if err != nil {
		return err
	}
	timeout := task.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	id := r.URL.Path[len("/cancel/"):]
	if id == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}
	if err := s.store.Remove(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	id := r.URL.Path[len("/status/"):]
	if id == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}
	task, err := s.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	filter := r.URL.Query().Get("name")
	tasks := s.store.List(filter)
	writeJSON(w, http.StatusOK, tasks)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
