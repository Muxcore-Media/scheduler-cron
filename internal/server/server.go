package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
)

// Server provides an HTTP API for managing cron tasks.
// Endpoints:
//   POST   /schedule    — register a new task
//   DELETE /cancel/{id} — cancel a task
//   GET    /status/{id} — get task status
//   GET    /list        — list tasks (?name=filter)
type Server struct {
	store *cronstore.Store
	mux   *http.ServeMux
}

// New creates an HTTP server backed by the given cron store.
func New(store *cronstore.Store) *Server {
	s := &Server{
		store: store,
		mux:   http.NewServeMux(),
	}
	s.mux.HandleFunc("/schedule", s.handleSchedule)
	s.mux.HandleFunc("/cancel/", s.handleCancel)
	s.mux.HandleFunc("/status/", s.handleStatus)
	s.mux.HandleFunc("/list", s.handleList)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
	return s
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

	id, err := s.store.Add(req.Name, req.CronExpr, req.Payload, 0, req.Meta, func(taskID string) {})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"task_id": id})
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
