package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Muxcore-Media/contracts-media/events"
	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
)

// EventPublisher publishes scheduler lifecycle events. Optional.
type EventPublisher interface {
	Publish(ctx context.Context, eventType, source string, payload []byte) error
}

// Server provides an HTTP API for managing cron tasks.
type Server struct {
	storePtr   atomic.Pointer[cronstore.Store]
	mux        *http.ServeMux
	events     EventPublisher
	moduleID   string
	httpClient *http.Client
	apiToken   string
	metrics    fireMetrics
}

type fireMetrics struct {
	fired     atomic.Uint64
	completed atomic.Uint64
	failed    atomic.Uint64
	timeout   atomic.Uint64
}

// Config configures the HTTP scheduler API.
type Config struct {
	Store    *cronstore.Store
	APIToken string
}

// New creates an HTTP server backed by the given cron store.
func New(store *cronstore.Store) *Server {
	return NewWithConfig(Config{Store: store})
}

// NewWithConfig creates a server with optional API token auth.
func NewWithConfig(cfg Config) *Server {
	s := &Server{
		mux:      http.NewServeMux(),
		moduleID: "scheduler-cron",
		httpClient: &http.Client{
			Timeout: 0,
		},
		apiToken: strings.TrimSpace(cfg.APIToken),
	}
	if cfg.APIToken == "" {
		s.apiToken = strings.TrimSpace(os.Getenv("SCHEDULER_API_TOKEN"))
	}
	s.storePtr.Store(cfg.Store)
	s.mux.HandleFunc("/schedule", s.handleSchedule)
	s.mux.HandleFunc("/cancel/", s.handleCancel)
	s.mux.HandleFunc("/status/", s.handleStatus)
	s.mux.HandleFunc("/list", s.handleList)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
	return s
}

// ReplaceStore swaps the backing cron store. Returns the previous store (caller should Stop).
func (s *Server) ReplaceStore(store *cronstore.Store) *cronstore.Store {
	return s.storePtr.Swap(store)
}

func (s *Server) store() *cronstore.Store {
	return s.storePtr.Load()
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
	_, _ = fmt.Fprintf(w, "# HELP scheduler_tasks_total Total scheduled tasks\n")
	_, _ = fmt.Fprintf(w, "# TYPE scheduler_tasks_total gauge\n")
	_, _ = fmt.Fprintf(w, "scheduler_tasks_total %d\n", s.store().Len())
	_, _ = fmt.Fprintf(w, "# HELP scheduler_tasks_fired_total Cron tasks fired\n")
	_, _ = fmt.Fprintf(w, "# TYPE scheduler_tasks_fired_total counter\n")
	_, _ = fmt.Fprintf(w, "scheduler_tasks_fired_total %d\n", s.metrics.fired.Load())
	_, _ = fmt.Fprintf(w, "# HELP scheduler_tasks_completed_total Cron tasks completed\n")
	_, _ = fmt.Fprintf(w, "# TYPE scheduler_tasks_completed_total counter\n")
	_, _ = fmt.Fprintf(w, "scheduler_tasks_completed_total %d\n", s.metrics.completed.Load())
	_, _ = fmt.Fprintf(w, "# HELP scheduler_tasks_failed_total Cron tasks failed\n")
	_, _ = fmt.Fprintf(w, "# TYPE scheduler_tasks_failed_total counter\n")
	_, _ = fmt.Fprintf(w, "scheduler_tasks_failed_total %d\n", s.metrics.failed.Load())
	_, _ = fmt.Fprintf(w, "# HELP scheduler_tasks_timeout_total Cron tasks timed out\n")
	_, _ = fmt.Fprintf(w, "# TYPE scheduler_tasks_timeout_total counter\n")
	_, _ = fmt.Fprintf(w, "scheduler_tasks_timeout_total %d\n", s.metrics.timeout.Load())
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
	if !s.requireAuth(w, r) {
		return
	}
	var req struct {
		Name     string          `json:"name"`
		CronExpr string          `json:"cron_expr"`
		Payload  json.RawMessage `json:"payload,omitempty"`
		Timeout  string          `json:"timeout,omitempty"`
		Once     bool            `json:"once,omitempty"`
		Meta     map[string]any  `json:"meta,omitempty"`
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

	id, err := s.store().AddWithOptions(req.Name, req.CronExpr, []byte(req.Payload), timeout, req.Meta, s.onFire, cronstore.AddOptions{Once: req.Once})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"task_id": id})
}

// OnFire runs when a scheduled task triggers (exported for store Restore).
func (s *Server) OnFire(taskID string) {
	s.onFire(taskID)
}

func (s *Server) onFire(taskID string) {
	task, err := s.store().Get(taskID)
	if err != nil {
		slog.Warn("cron fire: task missing", "task_id", taskID, "error", err)
		return
	}
	if task.Status == "running" {
		slog.Warn("cron fire: skip overlapping run", "task_id", taskID, "name", task.Name)
		return
	}

	s.metrics.fired.Add(1)

	if err := s.store().SetStatus(taskID, "running"); err != nil {
		slog.Warn("cron fire: set running", "task_id", taskID, "error", err)
	}

	s.publish(task, events.EventSchedulerTaskFired, nil)

	webhookURL := webhookURLFrom(task)
	fireErr := s.postWebhook(webhookURL, task)

	status := "completed"
	eventType := events.EventSchedulerTaskCompleted
	if fireErr != nil {
		if isTimeout(fireErr) {
			status = "timeout"
			eventType = events.EventSchedulerTaskTimeout
			s.metrics.timeout.Add(1)
		} else {
			status = "failed"
			eventType = events.EventSchedulerTaskFailed
			s.metrics.failed.Add(1)
		}
		slog.Warn("cron fire: webhook failed", "task_id", taskID, "error", fireErr, "status", status)
	} else {
		s.metrics.completed.Add(1)
	}
	if err := s.store().SetStatus(taskID, status); err != nil {
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
			if u, ok := v.(string); ok && u != "" {
				return u
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
		if u, ok := v.(string); ok {
			return u
		}
	}
	return ""
}

func (s *Server) postWebhook(url string, task *cronstore.Task) error {
	if url == "" {
		return nil
	}
	if err := validateWebhookURL(url); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{
		"task_id":   task.ID,
		"name":      task.Name,
		"cron_expr": task.CronExpr,
		"payload":   decodePayloadJSON(task.Payload),
		"meta":      task.Meta,
		"event":     events.EventSchedulerTaskFired,
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
	defer func() { _ = resp.Body.Close() }()
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
	if !s.requireAuth(w, r) {
		return
	}
	id := r.URL.Path[len("/cancel/"):]
	if id == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}
	if err := s.store().Remove(id); err != nil {
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
	if !s.requireAuth(w, r) {
		return
	}
	id := r.URL.Path[len("/status/"):]
	if id == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}
	task, err := s.store().Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, taskToJSON(*task, s.store()))
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	if !s.requireAuth(w, r) {
		return
	}
	filter := cronstore.ListFilter{
		Name:   r.URL.Query().Get("name"),
		Status: r.URL.Query().Get("status"),
	}
	tasks := s.store().List(filter)
	out := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskToJSON(t, s.store()))
	}
	writeJSON(w, http.StatusOK, out)
}

func taskToJSON(task cronstore.Task, store *cronstore.Store) map[string]any {
	m := map[string]any{
		"id":         task.ID,
		"name":       task.Name,
		"cron_expr":  task.CronExpr,
		"status":     task.Status,
		"once":       task.Once,
		"created_at": task.CreatedAt,
	}
	if task.Timeout > 0 {
		m["timeout"] = task.Timeout.String()
	} else {
		m["timeout"] = "0s"
	}
	if len(task.Payload) > 0 {
		m["payload"] = decodePayloadJSON(task.Payload)
	}
	if task.Meta != nil {
		m["meta"] = task.Meta
	}
	if !task.LastFired.IsZero() {
		m["last_fired_at"] = task.LastFired
	}
	if next, ok := store.NextRun(task.ID); ok {
		m["next_run"] = next
	}
	return m
}

func decodePayloadJSON(payload []byte) any {
	if len(payload) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		return string(payload)
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
