package cronstore

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	s, err := New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Stop()
}

func TestAdd_ValidCron(t *testing.T) {
	s, err := New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Stop()

	fired := make(chan string, 1)
	id, err := s.Add("test", "* * * * *", nil, 0, nil, func(taskID string) {
		fired <- taskID
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty task ID")
	}
	if s.Len() != 1 {
		t.Errorf("Len = %d, want 1", s.Len())
	}
}

func TestAdd_Negative(t *testing.T) {
	s, err := New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Stop()

	_, err = s.Add("", "* * * * *", nil, 0, nil, func(id string) {})
	if err == nil {
		t.Fatal("expected error for empty name")
	}

	_, err = s.Add("test", "", nil, 0, nil, func(id string) {})
	if err == nil {
		t.Fatal("expected error for empty cron expression")
	}

	_, err = s.Add("test", "invalid-cron", nil, 0, nil, func(id string) {})
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
}

func TestAdd_Predefined(t *testing.T) {
	s, err := New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Stop()

	tests := []string{"@every 5s", "@daily", "@hourly"}
	for _, expr := range tests {
		id, err := s.Add("predef-"+expr, expr, nil, 0, nil, func(id string) {})
		if err != nil {
			t.Errorf("Add(%q): %v", expr, err)
		}
		if id == "" {
			t.Errorf("Add(%q): empty ID", expr)
		}
	}
}

func TestGet(t *testing.T) {
	s, err := New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Stop()

	id, err := s.Add("get-test", "* * * * *", []byte("payload"), 0, nil, func(id string) {})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	task, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if task.Name != "get-test" {
		t.Errorf("Name = %q, want %q", task.Name, "get-test")
	}
	if task.Status != "scheduled" {
		t.Errorf("Status = %q, want %q", task.Status, "scheduled")
	}
	if task.CronExpr != "* * * * *" {
		t.Errorf("CronExpr = %q, want %q", task.CronExpr, "* * * * *")
	}

	_, err = s.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestRemove(t *testing.T) {
	s, err := New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Stop()

	id, err := s.Add("remove-test", "* * * * *", nil, 0, nil, func(id string) {})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if s.Len() != 1 {
		t.Errorf("Len = %d, want 1", s.Len())
	}

	if err := s.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if s.Len() != 0 {
		t.Errorf("Len = %d, want 0", s.Len())
	}

	if err := s.Remove(id); err == nil {
		t.Fatal("expected error removing nonexistent task")
	}
}

func TestList(t *testing.T) {
	s, err := New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Stop()

	s.Add("alpha", "* * * * *", nil, 0, nil, func(id string) {})
	s.Add("beta-one", "*/5 * * * *", nil, 0, nil, func(id string) {})
	s.Add("beta-two", "*/10 * * * *", nil, 0, nil, func(id string) {})

	if len(s.List("")) != 3 {
		t.Errorf("List() = %d, want 3", len(s.List("")))
	}
	if len(s.List("beta")) != 2 {
		t.Errorf("List(beta) = %d, want 2", len(s.List("beta")))
	}
	if len(s.List("nonexistent")) != 0 {
		t.Errorf("List(nonexistent) = %d, want 0", len(s.List("nonexistent")))
	}
}

func TestFire(t *testing.T) {
	s, err := New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Stop()

	fired := make(chan string, 1)
	_, err = s.Add("fire-test", "@every 1s", nil, 0, nil, func(taskID string) {
		fired <- taskID
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	select {
	case id := <-fired:
		if id == "" {
			t.Error("expected non-empty task ID on fire")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for task to fire")
	}
}

func TestSetStatus(t *testing.T) {
	s, err := New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Stop()

	id, err := s.Add("status-test", "0 0 * * *", nil, 0, nil, func(id string) {})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.SetStatus(id, "running"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	task, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if task.Status != "running" {
		t.Errorf("Status = %q, want running", task.Status)
	}
	if err := s.SetStatus("missing", "failed"); err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestAddOnce(t *testing.T) {
	s, err := New("UTC")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Stop()

	fired := make(chan struct{}, 1)
	id, err := s.AddWithOptions("once", "@once", nil, 0, nil, func(string) {
		fired <- struct{}{}
	}, AddOptions{Once: true})
	if err != nil {
		t.Fatalf("AddWithOptions: %v", err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("one-shot did not fire")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.Len() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("one-shot not removed, len=%d", s.Len())
}

func TestPersistAndRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	s1, err := New("UTC")
	if err != nil {
		t.Fatal(err)
	}
	s1.EnablePersist(path)
	handler := func(string) {}
	id, err := s1.Add("daily", "0 0 * * *", []byte(`{"a":1}`), 0, map[string]any{"k": "v"}, handler)
	if err != nil {
		t.Fatal(err)
	}
	s1.Stop()

	s2, err := New("UTC")
	if err != nil {
		t.Fatal(err)
	}
	s2.EnablePersist(path)
	s2.SetCatchUp(false)
	if err := s2.Restore(handler); err != nil {
		t.Fatal(err)
	}
	defer s2.Stop()
	if s2.Len() != 1 {
		t.Fatalf("len=%d", s2.Len())
	}
	task, err := s2.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if task.Name != "daily" || task.CronExpr != "0 0 * * *" {
		t.Fatalf("task=%+v", task)
	}
}

func TestCatchUpMissed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	s1, err := New("UTC")
	if err != nil {
		t.Fatal(err)
	}
	s1.EnablePersist(path)
	id, err := s1.Add("every-min", "* * * * *", nil, 0, nil, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	// backdate last fired so next is in the past
	s1.mu.Lock()
	s1.tasks[id].LastFired = time.Now().Add(-2 * time.Minute)
	s1.tasks[id].CreatedAt = time.Now().Add(-2 * time.Hour)
	_ = s1.saveLocked()
	s1.mu.Unlock()
	s1.Stop()

	fired := make(chan struct{}, 1)
	s2, err := New("UTC")
	if err != nil {
		t.Fatal(err)
	}
	s2.EnablePersist(path)
	s2.SetCatchUp(true)
	if err := s2.Restore(func(string) { fired <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	defer s2.Stop()
	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("expected catch-up fire")
	}
}
