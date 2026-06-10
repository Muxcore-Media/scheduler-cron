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

// New creates a cron store.
func New(location string) (*Store, error) {
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

// Add registers a new cron task. Returns the task ID.
func (s *Store) Add(name, cronExpr string, payload []byte, timeout time.Duration, meta map[string]any, handler func(taskID string)) (string, error) {
	if name == "" {
		return "", fmt.Errorf("task name is required")
	}
	if cronExpr == "" {
		return "", fmt.Errorf("cron expression is required")
	}

	// Validate the expression before using it.
	parser := cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	if _, err := parser.Parse(cronExpr); err != nil {
		return "", fmt.Errorf("invalid cron expression %q: %s", cronExpr, err)
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

	entryID, err := s.cron.AddFunc(cronExpr, func() {
		slog.Info("cron task fired", "task_id", id, "name", name, "expr", cronExpr)
		handler(id)
	})
	if err != nil {
		return "", fmt.Errorf("add cron func: %w", err)
	}

	s.mu.Lock()
	s.tasks[id] = task
	s.eid[id] = entryID
	s.mu.Unlock()

	return id, nil
}

// Remove cancels and deletes a scheduled task.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	eid, ok := s.eid[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	s.cron.Remove(eid)
	delete(s.tasks, id)
	delete(s.eid, id)
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
	return t, nil
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
