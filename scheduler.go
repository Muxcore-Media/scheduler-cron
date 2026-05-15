package schedulercron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Muxcore-Media/core/pkg/contracts"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

func init() {
	contracts.Register(func(deps contracts.ModuleDeps) contracts.Module {
		return NewModule(deps.EventBus)
	})
}

type taskEntry struct {
	Task   contracts.Task
	CronID cron.EntryID
}

type Module struct {
	mu    sync.RWMutex
	cron  *cron.Cron
	tasks map[string]taskEntry
	bus   contracts.EventBus
}

func NewModule(bus contracts.EventBus) *Module {
	return &Module{
		cron:  cron.New(cron.WithSeconds()),
		tasks: make(map[string]taskEntry),
		bus:   bus,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           "scheduler-cron",
		Name:         "Cron Scheduler",
		Version:      "1.0.0",
		Kinds:        []contracts.ModuleKind{contracts.ModuleKindScheduler},
		Description:  "Cron-based task scheduler",
		Author:       "MuxCore",
		Capabilities: []string{"scheduler.cron", "scheduler.schedule"},
	}
}

func (m *Module) Init(ctx context.Context) error { return nil }

func (m *Module) Start(ctx context.Context) error {
	m.cron.Start()
	slog.Info("cron scheduler started")
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	m.cron.Stop()
	return nil
}

func (m *Module) Health(ctx context.Context) error { return nil }

func (m *Module) Schedule(ctx context.Context, task contracts.Task) (string, error) {
	if task.ID == "" {
		task.ID = uuid.New().String()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tasks[task.ID]; exists {
		return "", fmt.Errorf("task %q already scheduled", task.ID)
	}

	cronID, err := m.cron.AddFunc(task.CronExpr, func() {
		event := contracts.Event{
			ID:        uuid.New().String(),
			Type:      "scheduler.task.execute",
			Source:    "scheduler-cron",
			Payload:   task.Payload,
			Metadata:  map[string]string{"task_id": task.ID, "task_name": task.Name},
		}
		m.bus.Publish(context.Background(), event)
	})
	if err != nil {
		return "", fmt.Errorf("invalid cron expression %q: %w", task.CronExpr, err)
	}

	m.tasks[task.ID] = taskEntry{Task: task, CronID: cronID}
	return task.ID, nil
}

func (m *Module) Cancel(ctx context.Context, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}

	m.cron.Remove(entry.CronID)
	delete(m.tasks, taskID)
	return nil
}

func (m *Module) Status(ctx context.Context, taskID string) (contracts.TaskStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.tasks[taskID]
	if !ok {
		return "", fmt.Errorf("task %q not found", taskID)
	}
	return contracts.TaskScheduled, nil
}
