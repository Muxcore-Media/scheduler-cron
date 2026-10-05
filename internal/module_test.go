package internal

import (
	"context"
	"strings"
	"testing"

	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	manifest "github.com/Muxcore-Media/scheduler-cron"
)

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID != "scheduler-cron" {
		t.Fatalf("id=%q", info.ID)
	}
	if info.Version != modulesdk.ManifestVersion(manifest.ManifestJSON) {
		t.Fatalf("version=%q", info.Version)
	}
	foundSettings := false
	for _, c := range info.Capabilities {
		if c == "settings" {
			foundSettings = true
		}
	}
	if !foundSettings {
		t.Fatal("expected settings capability")
	}
}

func TestSettings_PreInit(t *testing.T) {
	m := NewModule(Config{TZ: "UTC"})
	if err := m.UpdateSetting("timezone", "America/Chicago"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("store_path", "/tmp/sched.json"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("catch_up", "false"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("timezone", "Not/AZone"); err == nil {
		t.Fatal("expected invalid tz")
	}
	defs := m.Settings()
	by := map[string]string{}
	for _, d := range defs {
		by[d.Key] = d.Value
	}
	if by["timezone"] != "America/Chicago" || by["store_path"] != "/tmp/sched.json" || by["catch_up"] != "false" {
		t.Fatalf("%v", by)
	}
}

func TestModuleLifecycle_Cmux(t *testing.T) {
	m := NewModule(Config{HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("catch_up", "false"); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestNewModule_DefaultsLoopback(t *testing.T) {
	t.Setenv("SCHEDULER_HTTP_ADDR", "")
	m := NewModule(Config{})
	if m.httpAddr != "127.0.0.1:9200" {
		t.Fatalf("default addr=%q", m.httpAddr)
	}
}

func TestInit_NonLoopbackRequiresToken(t *testing.T) {
	t.Setenv("SCHEDULER_HTTP_TOKEN", "")
	m := NewModule(Config{HTTPAddr: "0.0.0.0:0"})
	err := m.Init(context.Background())
	if err == nil || !strings.Contains(err.Error(), "SCHEDULER_HTTP_TOKEN") {
		t.Fatalf("expected token error, got %v", err)
	}
}

func TestInit_NonLoopbackWithTokenFromEnv(t *testing.T) {
	t.Setenv("SCHEDULER_HTTP_TOKEN", "s3cret")
	m := NewModule(Config{HTTPAddr: "0.0.0.0:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	_ = m.Stop(ctx)
}
