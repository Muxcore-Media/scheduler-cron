package cronstore

import (
	"crypto/rand"
	"fmt"
	"log/slog"
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
}

// Store manages cron schedules. Thread-safe.
type Store struct {
	mu    sync.RWMutex
	tasks map[string]*Task
	cron  *cron.Cron
	eid   map[string]cron.EntryID // task ID → cron entry ID
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
		tasks: make(map[string]*Task),
		cron:  c,
		eid:   make(map[string]cron.EntryID),
	}, nil
}

// AddOptions configures optional Add behavior.
type AddOptions struct {
	Once bool // fire once then auto-remove (also accepted as cron_expr "@once")
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

	fire := func() {
		slog.Info("cron task fired", "task_id", id, "name", name, "expr", cronExpr)
		handler(id)
		if once {
			if err := s.Remove(id); err != nil {
				slog.Debug("one-shot remove after fire", "task_id", id, "error", err)
			}
		}
	}

	s.mu.Lock()
	s.tasks[id] = task
	if once {
		// Run shortly after schedule so HTTP /schedule returns before fire.
		timer := time.AfterFunc(10*time.Millisecond, fire)
		_ = timer
	} else {
		entryID, err := s.cron.AddFunc(cronExpr, fire)
		if err != nil {
			delete(s.tasks, id)
			s.mu.Unlock()
			return "", fmt.Errorf("add cron func: %w", err)
		}
		s.eid[id] = entryID
	}
	s.mu.Unlock()

	return id, nil
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
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// Len returns the number of scheduled tasks.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tasks)
}

// newID generates a random hex ID.
func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
