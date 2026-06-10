package test

import (
	"context"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSchedulerCron_HTTP(t *testing.T) {
	bin := buildModule(t)
	addr := ":19301"
	baseURL := "http://127.0.0.1" + addr

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "--http-addr", addr)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	t.Log("starting module binary", bin, "on", addr)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start module: %v", err)
	}
	defer func() {
		t.Log("module output:", outBuf.String())
		cmd.Process.Kill()
	}()

	// Wait for HTTP server to be ready (retry with backoff).
	var ready bool
	for i := 0; i < 10; i++ {
		if resp, err := http.Get(baseURL + "/list"); err == nil {
			resp.Body.Close()
			ready = true
			break
		}
		t.Logf("waiting for module... attempt %d", i+1)
		time.Sleep(500 * time.Millisecond)
	}
	if !ready {
		t.Fatal("module did not become ready in time")
	}

	// Schedule a task.
	body, _ := json.Marshal(map[string]string{
		"name": "integration-test", "cron_expr": "0 0 1 1 *",
	})
	resp, err := http.Post(baseURL+"/schedule", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /schedule: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result struct{ TaskID string `json:"task_id"` }
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if result.TaskID == "" {
		t.Fatal("expected non-empty task_id")
	}

	// List tasks.
	resp2, err := http.Get(baseURL + "/list")
	if err != nil {
		t.Fatalf("GET /list: %v", err)
	}
	var tasks []any
	json.NewDecoder(resp2.Body).Decode(&tasks)
	resp2.Body.Close()
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}

	// Get status.
	resp3, err := http.Get(baseURL + "/status/" + result.TaskID)
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	if resp3.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	resp3.Body.Close()

	// Cancel.
	req, _ := http.NewRequest(http.MethodDelete, baseURL+"/cancel/"+result.TaskID, nil)
	resp4, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /cancel: %v", err)
	}
	if resp4.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp4.StatusCode)
	}
	resp4.Body.Close()
}

func buildModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "scheduler-cron")
	t.Log("building module binary to", bin)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/module")
	cmd.Dir = findRepoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("binary not found at %s: %v", bin, err)
	}
	t.Log("module binary built successfully")
	return bin
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	dir, _ := os.Getwd()
	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("cannot find repo root")
	return ""
}

func init() {
	fmt.Fprintln(os.Stderr, "integration tests: building module binary...")
}
