package internal

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
	meshv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/mesh/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
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
	if err := m.Health(ctx); err != nil {
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

func TestModule_InitRejectsNonLoopbackWithoutToken(t *testing.T) {
	m := NewModule(Config{HTTPAddr: ":9204"})
	if err := m.Init(context.Background()); err == nil {
		t.Fatal("expected error without API token on wildcard bind")
	}
}

func TestApplyTimezoneChangePreservesTasks(t *testing.T) {
	m := NewModule(Config{HTTPAddr: "127.0.0.1:0", TZ: "UTC"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(ctx) }()

	id, err := m.store.Add("keep-me", "0 0 * * *", nil, 0, nil, m.srv.OnFire)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("timezone", "America/New_York"); err != nil {
		t.Fatal(err)
	}
	if m.store.Len() != 1 {
		t.Fatalf("len=%d", m.store.Len())
	}
	task, err := m.store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if task.Name != "keep-me" {
		t.Fatalf("task=%+v", task)
	}
}

func TestSchedulerMeshScheduleList(t *testing.T) {
	m := NewModule(Config{HTTPAddr: "127.0.0.1:0"})
	if err := m.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(context.Background()) }()

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	RegisterSchedulerMesh(srv, m.id, m)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	mesh := meshv1.NewModuleMeshClient(conn)
	ctx := context.Background()

	task := contracts.SchedulerTask{Name: "mesh-task", CronExpr: "0 0 * * *"}
	raw, _ := json.Marshal(task)
	resp, err := mesh.Call(ctx, &meshv1.CallRequest{Method: meshMethodSchedule, Payload: raw})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetError() != "" {
		t.Fatalf("schedule error: %s", resp.GetError())
	}
	var schedResp struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &schedResp); err != nil {
		t.Fatal(err)
	}
	if schedResp.TaskID == "" {
		t.Fatal("empty task_id")
	}

	listResp, err := mesh.Call(ctx, &meshv1.CallRequest{Method: meshMethodList, Payload: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	if listResp.GetError() != "" {
		t.Fatalf("list error: %s", listResp.GetError())
	}
	var listed []contracts.SchedulerTask
	if err := json.Unmarshal(listResp.GetPayload(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Name != "mesh-task" {
		t.Fatalf("listed=%+v", listed)
	}

	statusResp, err := mesh.Call(ctx, &meshv1.CallRequest{
		Method:  meshMethodStatus,
		Payload: []byte(`{"task_id":"` + schedResp.TaskID + `"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if statusResp.GetError() != "" {
		t.Fatalf("status error: %s", statusResp.GetError())
	}

	cancelResp, err := mesh.Call(ctx, &meshv1.CallRequest{
		Method:  meshMethodCancel,
		Payload: []byte(`{"task_id":"` + schedResp.TaskID + `"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelResp.GetError() != "" {
		t.Fatalf("cancel error: %s", cancelResp.GetError())
	}
}

func TestDialCoreRetriesUntilStop(t *testing.T) {
	t.Setenv("MUXCORE_GRPC_ADDR", "127.0.0.1:1")
	m := NewModule(Config{HTTPAddr: "127.0.0.1:0"})
	if err := m.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		m.dialCore()
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	_ = m.Stop(context.Background())
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("dialCore did not exit after stop")
	}
}
