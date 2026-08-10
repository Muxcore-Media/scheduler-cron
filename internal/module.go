package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/Muxcore-Media/core/pkg/contracts"
	"github.com/Muxcore-Media/core/sdk/go/client"
	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
	"github.com/Muxcore-Media/scheduler-cron/internal/server"
)

type Module struct {
	store    *cronstore.Store
	srv      *server.Server
	httpSrv  *http.Server
	lis      net.Listener
	mc       *client.Client
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
		Version:      "0.1.1",
		Roles:        []string{"infrastructure"},
		Description:  "Cron-based task scheduler for periodic and recurring jobs",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityScheduler, "scheduler.cron"},
		HTTPAddr:     m.httpAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	var err error
	tz := os.Getenv("SCHEDULER_TZ")
	m.store, err = cronstore.New(tz)
	if err != nil {
		return fmt.Errorf("init cron store: %w", err)
	}
	m.srv = server.New(m.store)
	m.lis, err = net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.httpAddr, err)
	}
	slog.Info("scheduler-cron initialized", "addr", m.httpAddr, "tz", firstNonEmpty(tz, "UTC"))
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func (m *Module) Start(ctx context.Context) error {
	m.httpSrv = &http.Server{Handler: m.srv.Handler()}
	go m.dialCore(ctx)
	go func() {
		slog.Info("scheduler-cron HTTP started", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(m.lis); err != nil && err != http.ErrServerClosed {
			slog.Error("scheduler-cron HTTP error", "error", err)
		}
	}()
	return nil
}

func (m *Module) dialCore(ctx context.Context) {
	meshAddr := os.Getenv("MUXCORE_GRPC_ADDR")
	if meshAddr == "" {
		meshAddr = "localhost:9090"
	}
	insecureMode := os.Getenv("MUXCORE_INSECURE_DISABLE_TLS") == "true" || os.Getenv("MUXCORE_GRPC_INSECURE") == "true"
	var opts []client.Option
	if insecureMode {
		opts = append(opts, client.WithInsecure())
	}
	c, err := client.Dial(meshAddr, opts...)
	if err != nil {
		slog.Warn("scheduler-cron: dial core (events unavailable)", "error", err)
		return
	}
	m.mc = c
	m.srv.SetEventPublisher(c.Events, m.id)
	slog.Info("scheduler-cron: connected to core mesh", "addr", meshAddr)
}

func (m *Module) Stop(ctx context.Context) error {
	if m.httpSrv != nil {
		m.httpSrv.Shutdown(ctx)
	}
	if m.mc != nil {
		m.mc.Close()
	}
	if m.store != nil {
		m.store.Stop()
	}
	slog.Info("scheduler-cron stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}
