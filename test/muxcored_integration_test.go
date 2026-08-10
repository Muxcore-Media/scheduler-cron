//go:build integration

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/module/v1"
	"github.com/Muxcore-Media/core/sdk/go/client"
	"github.com/Muxcore-Media/scheduler-cron/internal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestSchedulerWithMuxcored boots a real muxcored, registers scheduler-cron,
// discovers it by capability, schedules an @once webhook, and asserts fire.
func TestSchedulerWithMuxcored(t *testing.T) {
	muxcored := resolveMuxcored(t)
	tmp := t.TempDir()

	grpcAddr := freeListenAddr(t)
	httpAddr := freeListenAddr(t)
	schedAddr := freeListenAddr(t)

	cfgPath := filepath.Join(tmp, "muxcore.json")
	cfg := fmt.Sprintf(`{
  "server": {"addr": %q},
  "grpc": {"addr": %q},
  "log": {"level": "error", "format": "text"}
}`, httpAddr, grpcAddr)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	coreCmd := exec.Command(muxcored)
	coreCmd.Env = append(os.Environ(),
		"MUXCORE_CONFIG="+cfgPath,
		"MUXCORE_INSECURE_DISABLE_TLS=true",
	)
	coreCmd.Stdout = os.Stdout
	coreCmd.Stderr = os.Stderr
	if err := coreCmd.Start(); err != nil {
		t.Fatalf("start muxcored: %v", err)
	}
	defer func() {
		_ = coreCmd.Process.Kill()
		_, _ = coreCmd.Process.Wait()
	}()

	waitTCP(t, grpcAddr, 15*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mod := internal.NewModule(internal.Config{
		ID:       "scheduler-cron-itest",
		HTTPAddr: schedAddr,
	})
	t.Setenv("MUXCORE_GRPC_ADDR", grpcAddr)
	t.Setenv("MUXCORE_INSECURE_DISABLE_TLS", "true")

	if err := mod.Init(ctx); err != nil {
		t.Fatalf("scheduler Init: %v", err)
	}
	if err := mod.Start(ctx); err != nil {
		t.Fatalf("scheduler Start: %v", err)
	}
	defer func() { _ = mod.Stop(context.Background()) }()

	regConn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial register: %v", err)
	}
	defer regConn.Close()
	reg := modulev1.NewModuleRegistrationClient(regConn)

	info := mod.Info()
	regResp, err := reg.Register(ctx, &modulev1.RegisterRequest{
		ModuleId: info.ID,
		ModuleInfo: &modulev1.ModuleInfo{
			Id:           info.ID,
			Name:         info.Name,
			Version:      info.Version,
			Roles:        info.Roles,
			Capabilities: info.Capabilities,
			HttpAddr:     info.HTTPAddr,
		},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !regResp.GetAccepted() {
		t.Fatalf("registration rejected: %s", regResp.GetError())
	}
	defer func() {
		_, _ = reg.Unregister(context.Background(), &modulev1.UnregisterRequest{ModuleId: info.ID})
	}()

	mc, err := client.Dial(grpcAddr, client.WithInsecure())
	if err != nil {
		t.Fatalf("client.Dial: %v", err)
	}
	defer mc.Close()

	var foundAddr string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mods, err := mc.Discovery.FindByCapability(ctx, contracts.CapabilityScheduler)
		if err == nil {
			for _, m := range mods {
				if m.GetId() == info.ID && m.GetHttpAddr() != "" {
					foundAddr = m.GetHttpAddr()
					break
				}
			}
		}
		if foundAddr == "" {
			mods, err = mc.Discovery.FindByCapability(ctx, "scheduler.cron")
			if err == nil {
				for _, m := range mods {
					if m.GetId() == info.ID && m.GetHttpAddr() != "" {
						foundAddr = m.GetHttpAddr()
						break
					}
				}
			}
		}
		if foundAddr != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if foundAddr == "" {
		t.Fatal("scheduler module not discovered by capability")
	}

	healthURL := "http://" + normalizeHTTPHost(foundAddr) + "/health"
	resp, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status %d", resp.StatusCode)
	}

	fired := make(chan struct{}, 1)
	hookLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer hookLis.Close()
	hookSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case fired <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	})}
	go hookSrv.Serve(hookLis)
	defer hookSrv.Close()

	scheduleURL := "http://" + normalizeHTTPHost(foundAddr) + "/schedule"
	body, _ := json.Marshal(map[string]any{
		"name":      "itest-once",
		"cron_expr": "@once",
		"once":      true,
		"meta": map[string]string{
			"webhook_url": "http://" + hookLis.Addr().String() + "/hook",
		},
	})
	sresp, err := http.Post(scheduleURL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	defer sresp.Body.Close()
	payload, _ := io.ReadAll(sresp.Body)
	if sresp.StatusCode != http.StatusOK && sresp.StatusCode != http.StatusCreated {
		t.Fatalf("schedule status %d: %s", sresp.StatusCode, payload)
	}

	select {
	case <-fired:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for @once webhook fire")
	}
}

func resolveMuxcored(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("MUXCORED_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
		t.Fatalf("MUXCORED_BIN=%s not found", bin)
	}
	candidates := []string{
		os.Getenv("CORE_DIR"),
		filepath.Join("..", "core"),
		"/home/ender/Projects/muxcore/core",
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		cmdDir := dir
		out := filepath.Join(t.TempDir(), "muxcored")
		cmd := exec.Command("go", "build", "-o", out, "./cmd/muxcored")
		cmd.Dir = cmdDir
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Logf("build muxcored in %s failed: %v\n%s", cmdDir, err, b)
			continue
		}
		return out
	}
	t.Skip("muxcored not available (set MUXCORED_BIN or CORE_DIR, or checkout core at ../core)")
	return ""
}

func freeListenAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	return addr
}

func waitTCP(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", addr)
}

func normalizeHTTPHost(addr string) string {
	if addr == "" {
		return addr
	}
	if addr[0] == ':' {
		return "127.0.0.1" + addr
	}
	return addr
}
