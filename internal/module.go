package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	"github.com/Muxcore-Media/core/sdk/go/client"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	manifest "github.com/Muxcore-Media/scheduler-cron"
	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
	"github.com/Muxcore-Media/scheduler-cron/internal/server"
)

type Module struct {
	store     *cronstore.Store
	srv       *server.Server
	httpSrv   *http.Server
	grpcSrv   *grpc.Server
	lis       net.Listener
	cm        cmux.CMux
	mc        *client.Client
	id        string
	httpAddr  string
	token     string
	cfgMu     sync.RWMutex
	tz        string
	storePath string
	catchUp   bool
}

type Config struct {
	ID       string
	HTTPAddr string
	// HTTPToken is the bearer token (env SCHEDULER_HTTP_TOKEN). Required for non-loopback HTTPAddr.
	HTTPToken string
	TZ        string
	StorePath string
	CatchUp   *bool
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "scheduler-cron"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = "127.0.0.1:9200"
	}
	if v := os.Getenv("SCHEDULER_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if cfg.HTTPToken == "" {
		cfg.HTTPToken = strings.TrimSpace(os.Getenv("SCHEDULER_HTTP_TOKEN"))
	}
	tz := cfg.TZ
	if tz == "" {
		tz = os.Getenv("SCHEDULER_TZ")
	}
	if tz == "" {
		tz = "UTC"
	}
	storePath := cfg.StorePath
	if storePath == "" {
		storePath = strings.TrimSpace(os.Getenv("SCHEDULER_STORE_PATH"))
	}
	catchUp := true
	if cfg.CatchUp != nil {
		catchUp = *cfg.CatchUp
	} else if v := strings.TrimSpace(os.Getenv("SCHEDULER_CATCH_UP")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			catchUp = b
		}
	}
	return &Module{
		id:        cfg.ID,
		httpAddr:  cfg.HTTPAddr,
		token:     cfg.HTTPToken,
		tz:        tz,
		storePath: storePath,
		catchUp:   catchUp,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Scheduler Cron",
		Version:      modulesdk.ManifestVersion(manifest.ManifestJSON),
		Roles:        []string{"infrastructure"},
		Description:  "Cron scheduler with persistent store and missed-fire catch-up",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityScheduler, "scheduler.cron", "settings"},
		HTTPAddr:     m.httpAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	if err := server.ValidateListen(m.httpAddr, m.token); err != nil {
		return err
	}
	m.cfgMu.RLock()
	tz := m.tz
	storePath := m.storePath
	catchUp := m.catchUp
	m.cfgMu.RUnlock()

	store, err := cronstore.New(tz)
	if err != nil {
		return fmt.Errorf("init cron store: %w", err)
	}
	store.SetCatchUp(catchUp)
	if storePath != "" {
		store.EnablePersist(storePath)
	}
	m.store = store
	m.srv = server.New(store)
	if storePath != "" {
		if err := store.Restore(m.srv.OnFire); err != nil {
			store.Stop()
			return fmt.Errorf("restore store %q: %w", storePath, err)
		}
		slog.Info("scheduler-cron restored tasks", "path", storePath, "count", store.Len())
	}
	m.lis, err = net.Listen("tcp", m.httpAddr)
	if err != nil {
		store.Stop()
		return fmt.Errorf("listen %s: %w", m.httpAddr, err)
	}
	slog.Info("scheduler-cron initialized", "addr", m.httpAddr, "tz", tz, "persist", storePath, "catch_up", catchUp)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.cm = cmux.New(m.lis)
	grpcL := m.cm.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
	httpL := m.cm.Match(cmux.Any())

	m.grpcSrv = grpc.NewServer(server.GRPCAuth(m.token)...)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)
	m.httpSrv = &http.Server{Handler: server.RequireToken(m.token, m.srv.Handler())}

	go m.dialCore(ctx)
	go func() {
		slog.Info("scheduler-cron gRPC settings started", "addr", m.httpAddr)
		if err := m.grpcSrv.Serve(grpcL); err != nil {
			slog.Error("scheduler-cron gRPC error", "error", err)
		}
	}()
	go func() {
		slog.Info("scheduler-cron HTTP started", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(httpL); err != nil && err != http.ErrServerClosed {
			slog.Error("scheduler-cron HTTP error", "error", err)
		}
	}()
	go func() {
		if err := m.cm.Serve(); err != nil {
			slog.Debug("scheduler-cron cmux closed", "error", err)
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
		_ = m.httpSrv.Shutdown(ctx)
	}
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.cm != nil {
		m.cm.Close()
	}
	if m.mc != nil {
		_ = m.mc.Close()
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
