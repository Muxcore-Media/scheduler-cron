package cronstore

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Task represents a scheduled task.
type Task struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	CronExpr  string         `json:"cron_expr"`
	Payload   []byte         `json:"payload,omitempty"`
	Timeout   time.Duration  `json:"timeout"`
	Meta      map[string]any `json:"meta,omitempty"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	LastFired time.Time      `json:"last_fired,omitempty"`
}

// Store manages cron schedules. Thread-safe.
type Store struct {
	mu       sync.RWMutex
	tasks    map[string]*Task
	cron     *cron.Cron
	eid      map[string]cron.EntryID
	path     string
	catchUp  bool
	location *time.Location
	handler  func(taskID string)
}

// New creates a cron store. location is an IANA timezone name; empty means UTC.
func New(location string) (*Store, error) {
	if location == "" {
		location = "UTC"
	}
	loc, err := time.LoadLocation(location)
	if err != nil {
		return nil, fmt.Errorf("load location %q: %w", location, err)
	}
	c := cron.New(cron.WithLocation(loc))
	c.Start()
	return &Store{
		tasks:    make(map[string]*Task),
		cron:     c,
		eid:      make(map[string]cron.EntryID),
		location: loc,
		catchUp:  true,
	}, nil
}

// EnablePersist writes task snapshots to path after mutations and enables Restore.
func (s *Store) EnablePersist(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
}

// SetCatchUp controls missed-fire catch-up on Restore (default true).
func (s *Store) SetCatchUp(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.catchUp = enabled
}

// AddOptions configures optional Add behavior.
type AddOptions struct {
	Once bool
}

// Add registers a new cron task. Returns the task ID.
func (s *Store) Add(name, cronExpr string, payload []byte, timeout time.Duration, meta map[string]any, handler func(taskID string)) (string, error) {
	return s.AddWithOptions(name, cronExpr, payload, timeout, meta, handler, AddOptions{})
}

// AddWithOptions registers a cron task with optional one-shot behavior.
func (s *Store) AddWithOptions(name, cronExpr string, payload []byte, timeout time.Duration, meta map[string]any, handler func(taskID string), opts AddOptions) (string, error) {
	if name == "" {
		return "", fmt.Errorf("task name is required")
	}
	if cronExpr == "" {
		return "", fmt.Errorf("cron expression is required")
	}

	once := opts.Once || strings.EqualFold(cronExpr, "@once")
	if once {
		cronExpr = "@once"
	} else {
		parser := cron.NewParser(
			cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		)
		if _, err := parser.Parse(cronExpr); err != nil {
			return "", fmt.Errorf("invalid cron expression %q: %s", cronExpr, err)
		}
	}

	id := newID()
	task := &Task{
		ID:        id,
		Name:      name,
		CronExpr:  cronExpr,
		Payload:   payload,
		Timeout:   timeout,
		Meta:      meta,
		Status:    "scheduled",
		CreatedAt: time.Now(),
	}

	s.mu.Lock()
	s.handler = handler
	if err := s.scheduleLocked(task, handler, once); err != nil {
		s.mu.Unlock()
		return "", err
	}
	s.tasks[id] = task
	if err := s.saveLocked(); err != nil {
		slog.Warn("cronstore persist after add", "error", err)
	}
	s.mu.Unlock()
	return id, nil
}

func (s *Store) scheduleLocked(task *Task, handler func(taskID string), once bool) error {
	id := task.ID
	fire := func() {
		slog.Info("cron task fired", "task_id", id, "name", task.Name, "expr", task.CronExpr)
		handler(id)
		s.mu.Lock()
		if t, ok := s.tasks[id]; ok {
			t.LastFired = time.Now()
			_ = s.saveLocked()
		}
		s.mu.Unlock()
		if once {
			if err := s.Remove(id); err != nil {
				slog.Debug("one-shot remove after fire", "task_id", id, "error", err)
			}
		}
	}

	if once {
		time.AfterFunc(10*time.Millisecond, fire)
		return nil
	}
	entryID, err := s.cron.AddFunc(task.CronExpr, fire)
	if err != nil {
		return fmt.Errorf("add cron func: %w", err)
	}
	s.eid[id] = entryID
	return nil
}

// Restore reloads tasks from the persist file and re-schedules them.
// Missed catch-up: if a recurring task's next fire after LastFired (or CreatedAt)
// is in the past, handler is invoked once before re-arming.
func (s *Store) Restore(handler func(taskID string)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read store: %w", err)
	}
	var tasks []Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return fmt.Errorf("decode store: %w", err)
	}

	now := time.Now()
	parser := cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)

	for i := range tasks {
		t := tasks[i]
		if t.CronExpr == "@once" {
			continue // one-shots are not restored
		}
		cp := t
		if t.Payload != nil {
			cp.Payload = append([]byte(nil), t.Payload...)
		}
		if t.Meta != nil {
			cp.Meta = make(map[string]any, len(t.Meta))
			for k, v := range t.Meta {
				cp.Meta[k] = v
			}
		}
		task := &cp
		s.tasks[task.ID] = task

		if s.catchUp {
			sched, err := parser.Parse(task.CronExpr)
			if err == nil {
				from := task.CreatedAt
				if !task.LastFired.IsZero() {
					from = task.LastFired
				}
				next := sched.Next(from.In(s.location))
				if next.Before(now) {
					slog.Info("cron catch-up fire", "task_id", task.ID, "missed", next)
					// Release lock for handler (may call SetStatus/Get).
					s.mu.Unlock()
					handler(task.ID)
					s.mu.Lock()
					if tt, ok := s.tasks[task.ID]; ok {
						tt.LastFired = time.Now()
					}
				}
			}
		}

		if err := s.scheduleLocked(task, handler, false); err != nil {
			slog.Warn("cron restore schedule", "task_id", task.ID, "error", err)
			delete(s.tasks, task.ID)
			continue
		}
	}
	return s.saveLocked()
}

// Remove cancels and deletes a scheduled task.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[id]; !ok {
		return fmt.Errorf("task %q not found", id)
	}
	if eid, ok := s.eid[id]; ok {
		s.cron.Remove(eid)
		delete(s.eid, id)
	}
	delete(s.tasks, id)
	if err := s.saveLocked(); err != nil {
		slog.Warn("cronstore persist after remove", "error", err)
	}
	return nil
}

// Get returns a task by ID.
func (s *Store) Get(id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %q not found", id)
	}
	cp := *t
	if t.Payload != nil {
		cp.Payload = append([]byte(nil), t.Payload...)
	}
	if t.Meta != nil {
		cp.Meta = make(map[string]any, len(t.Meta))
		for k, v := range t.Meta {
			cp.Meta[k] = v
		}
	}
	return &cp, nil
}

// SetStatus updates a task's status.
func (s *Store) SetStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	t.Status = status
	if status == "completed" || status == "failed" || status == "timeout" {
		t.LastFired = time.Now()
	}
	if err := s.saveLocked(); err != nil {
		slog.Warn("cronstore persist after status", "error", err)
	}
	return nil
}

// List returns all scheduled tasks, optionally filtered by name substring.
func (s *Store) List(nameFilter string) []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if nameFilter != "" && !strings.Contains(t.Name, nameFilter) {
			continue
		}
		result = append(result, t)
	}
	return result
}

// Stop stops the cron scheduler. Call during shutdown.
func (s *Store) Stop() {
	s.mu.Lock()
	_ = s.saveLocked()
	s.mu.Unlock()
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// Len returns the number of scheduled tasks.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tasks)
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	tasks := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if t.CronExpr == "@once" {
			continue
		}
		cp := *t
		if t.Payload != nil {
			cp.Payload = append([]byte(nil), t.Payload...)
		}
		tasks = append(tasks, cp)
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
