package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
	"github.com/Muxcore-Media/scheduler-cron/internal/server"
	modulev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/module/v1"
)

func main() {
	meshAddr := flag.String("muxcore-mesh-addr", "localhost:9090", "gRPC address of the MuxCore mesh")
	moduleID := flag.String("muxcore-module-id", "scheduler-cron", "Module identifier")
	httpAddr := flag.String("http-addr", ":9200", "Address for this module's HTTP API")
	timezone := flag.String("timezone", "UTC", "Timezone for cron evaluation")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("starting scheduler-cron", "version", "0.1.0")

	// Create cron store.
	store, err := cronstore.New(*timezone)
	if err != nil {
		slog.Error("failed to create cron store", "error", err)
		os.Exit(1)
	}
	defer store.Stop()

	// Start HTTP API server.
	httpSrv := server.New(store)

	lis, err := net.Listen("tcp", *httpAddr)
	if err != nil {
		slog.Error("failed to listen", "addr", *httpAddr, "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("HTTP API server listening", "addr", *httpAddr)
		if err := http.Serve(lis, httpSrv.Handler()); err != nil {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	// Connect to core's mesh.
	conn, err := grpc.NewClient(*meshAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("failed to connect to core mesh", "addr", *meshAddr, "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Register as a sidecar module.
	regClient := modulev1.NewModuleRegistrationClient(conn)
	resp, err := regClient.Register(context.Background(), &modulev1.RegisterRequest{
		ModuleId: *moduleID,
		ModuleInfo: &modulev1.ModuleInfo{
			Id:           *moduleID,
			Name:         "Scheduler Cron",
			Version:      "0.1.0",
			Description:  "Cron-based task scheduler for periodic and recurring jobs",
			Author:       "MuxCore",
			Roles:        []string{"infrastructure"},
			Capabilities: []string{"scheduler"},
			HttpAddr:     *httpAddr,
		},
	})
	if err != nil {
		slog.Error("registration failed", "error", err)
		os.Exit(1)
	}
	if !resp.Accepted {
		slog.Error("registration rejected", "reason", resp.Error)
		os.Exit(1)
	}
	slog.Info("module registered with core",
		"id", *moduleID,
		"mesh_addr", resp.MeshAddr,
		"node_id", resp.NodeId,
	)

	// Wait for shutdown signal.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()

	slog.Info("shutting down...")
	regClient.Unregister(context.Background(), &modulev1.UnregisterRequest{ModuleId: *moduleID})
	slog.Info("shutdown complete")
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: scheduler-cron [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
}
