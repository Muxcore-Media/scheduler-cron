package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/Muxcore-Media/core/pkg/contracts"
	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
	"github.com/Muxcore-Media/scheduler-cron/internal/server"
)

type Module struct {
	store    *cronstore.Store
	srv      *server.Server
	httpSrv  *http.Server
	lis      net.Listener
	id       string
	httpAddr string
}

type Config struct {
	ID       string
	HTTPAddr string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "scheduler-cron"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9200"
	}
	if v := os.Getenv("SCHEDULER_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	return &Module{
		id:       cfg.ID,
		httpAddr: cfg.HTTPAddr,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Scheduler Cron",
		Version:      "0.1.0",
		Roles:        []string{"infrastructure"},
		Description:  "Cron-based task scheduler for periodic and recurring jobs",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityScheduler},
		HTTPAddr:     m.httpAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	var err error
	m.store, err = cronstore.New("")
	if err != nil {
		return fmt.Errorf("init cron store: %w", err)
	}
	m.srv = server.New(m.store)
	m.lis, err = net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.httpAddr, err)
	}
	slog.Info("scheduler-cron initialized", "addr", m.httpAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.httpSrv = &http.Server{Handler: m.srv.Handler()}
	go func() {
		slog.Info("scheduler-cron HTTP started", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(m.lis); err != nil && err != http.ErrServerClosed {
			slog.Error("scheduler-cron HTTP error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.httpSrv != nil {
		m.httpSrv.Shutdown(ctx)
	}
	slog.Info("scheduler-cron stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}
