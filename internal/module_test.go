package internal

import (
	"context"
	"testing"
)

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID != "scheduler-cron" {
		t.Fatalf("id=%q", info.ID)
	}
	if info.Version != "0.1.5" {
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
