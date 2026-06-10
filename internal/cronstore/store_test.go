package cronstore

import (
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
