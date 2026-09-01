package internal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Muxcore-Media/core/pkg/contracts"
	meshv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/mesh/v1"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
	"github.com/Muxcore-Media/scheduler-cron/internal/server"
	"google.golang.org/grpc"
)

const (
	meshMethodSettings      = "Settings"
	meshMethodUpdateSetting = "UpdateSetting"
	meshMethodSchedule      = "Schedule"
	meshMethodCancel        = "Cancel"
	meshMethodStatus        = "Status"
	meshMethodList          = "List"
)

type schedulerMeshServer struct {
	meshv1.UnimplementedModuleMeshServer
	moduleID string
	mod      *Module
	settings modulesdk.SettingsHandler
}

// RegisterSchedulerMesh wires ModuleMesh Settings and contracts.Scheduler methods.
func RegisterSchedulerMesh(srv *grpc.Server, moduleID string, mod *Module) {
	meshv1.RegisterModuleMeshServer(srv, &schedulerMeshServer{
		moduleID: moduleID,
		mod:      mod,
		settings: modulesdk.SettingsHandlerFromProvider(mod),
	})
}

func (s *schedulerMeshServer) Call(ctx context.Context, req *meshv1.CallRequest) (*meshv1.CallResponse, error) {
	if req.GetTargetModule() != "" && req.GetTargetModule() != s.moduleID {
		return &meshv1.CallResponse{Error: fmt.Sprintf("wrong target module %q", req.GetTargetModule())}, nil
	}
	switch req.GetMethod() {
	case meshMethodSchedule:
		return s.handleSchedule(ctx, req.GetPayload())
	case meshMethodCancel:
		return s.handleCancel(ctx, req.GetPayload())
	case meshMethodStatus:
		return s.handleStatus(ctx, req.GetPayload())
	case meshMethodList:
		return s.handleList(ctx, req.GetPayload())
	case meshMethodSettings:
		return s.callSettings(ctx, req)
	case meshMethodUpdateSetting:
		return s.callUpdateSetting(ctx, req)
	default:
		return &meshv1.CallResponse{Error: fmt.Sprintf("unknown method %q", req.GetMethod())}, nil
	}
}

func (s *schedulerMeshServer) callSettings(ctx context.Context, req *meshv1.CallRequest) (*meshv1.CallResponse, error) {
	_ = ctx
	if s.settings.List == nil {
		return &meshv1.CallResponse{Error: "settings list not implemented"}, nil
	}
	raw, err := json.Marshal(s.settings.List())
	if err != nil {
		return &meshv1.CallResponse{Error: err.Error()}, nil
	}
	return &meshv1.CallResponse{Payload: raw}, nil
}

func (s *schedulerMeshServer) callUpdateSetting(ctx context.Context, req *meshv1.CallRequest) (*meshv1.CallResponse, error) {
	_ = ctx
	if s.settings.Update == nil {
		return &meshv1.CallResponse{Error: "settings update not implemented"}, nil
	}
	var body struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	}
	if len(req.GetPayload()) > 0 {
		if err := json.Unmarshal(req.GetPayload(), &body); err != nil {
			return &meshv1.CallResponse{Error: fmt.Sprintf("invalid UpdateSetting payload: %v", err)}, nil
		}
	}
	if body.Key == "" {
		return &meshv1.CallResponse{Error: "UpdateSetting requires Key"}, nil
	}
	if err := s.settings.Update(body.Key, body.Value); err != nil {
		return &meshv1.CallResponse{Error: err.Error()}, nil
	}
	return &meshv1.CallResponse{Payload: []byte(`{"ok":true}`)}, nil
}

func (s *schedulerMeshServer) handleSchedule(ctx context.Context, payload []byte) (*meshv1.CallResponse, error) {
	_ = ctx
	var task contracts.SchedulerTask
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &task); err != nil {
			return &meshv1.CallResponse{Error: fmt.Sprintf("invalid SchedulerTask payload: %v", err)}, nil
		}
	}
	if task.Name == "" || task.CronExpr == "" {
		return &meshv1.CallResponse{Error: "Schedule requires Name and CronExpr"}, nil
	}
	once := false
	if task.Meta != nil {
		if v, ok := task.Meta["once"]; ok {
			switch t := v.(type) {
			case bool:
				once = t
			case string:
				once = t == "true" || t == "1"
			}
		}
	}
	id, err := s.mod.scheduleTask(task, once)
	if err != nil {
		return &meshv1.CallResponse{Error: err.Error()}, nil
	}
	raw, _ := json.Marshal(map[string]string{"task_id": id})
	return &meshv1.CallResponse{Payload: raw}, nil
}

func (s *schedulerMeshServer) handleCancel(ctx context.Context, payload []byte) (*meshv1.CallResponse, error) {
	_ = ctx
	var body struct {
		TaskID string `json:"task_id"`
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &body)
	}
	if body.TaskID == "" {
		body.TaskID = string(payload)
	}
	if body.TaskID == "" {
		return &meshv1.CallResponse{Error: "Cancel requires task_id"}, nil
	}
	if err := s.mod.store.Remove(body.TaskID); err != nil {
		return &meshv1.CallResponse{Error: err.Error()}, nil
	}
	return &meshv1.CallResponse{Payload: []byte(`{"status":"cancelled"}`)}, nil
}

func (s *schedulerMeshServer) handleStatus(ctx context.Context, payload []byte) (*meshv1.CallResponse, error) {
	_ = ctx
	var body struct {
		TaskID string `json:"task_id"`
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &body)
	}
	if body.TaskID == "" {
		body.TaskID = string(payload)
	}
	if body.TaskID == "" {
		return &meshv1.CallResponse{Error: "Status requires task_id"}, nil
	}
	task, err := s.mod.store.Get(body.TaskID)
	if err != nil {
		return &meshv1.CallResponse{Error: err.Error()}, nil
	}
	raw, err := json.Marshal(map[string]string{"status": task.Status})
	if err != nil {
		return &meshv1.CallResponse{Error: err.Error()}, nil
	}
	return &meshv1.CallResponse{Payload: raw}, nil
}

func (s *schedulerMeshServer) handleList(ctx context.Context, payload []byte) (*meshv1.CallResponse, error) {
	_ = ctx
	var filter contracts.SchedulerTaskFilter
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &filter); err != nil {
			return &meshv1.CallResponse{Error: fmt.Sprintf("invalid SchedulerTaskFilter: %v", err)}, nil
		}
	}
	tasks := s.mod.store.List(cronstore.ListFilter{
		Name:   filter.Name,
		Status: string(filter.Status),
	})
	out := make([]contracts.SchedulerTask, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, contracts.SchedulerTask{
			ID:       t.ID,
			Name:     t.Name,
			CronExpr: t.CronExpr,
			Payload:  append([]byte(nil), t.Payload...),
			Timeout:  t.Timeout,
			Meta:     t.Meta,
		})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return &meshv1.CallResponse{Error: err.Error()}, nil
	}
	return &meshv1.CallResponse{Payload: raw}, nil
}

func (s *schedulerMeshServer) StreamCall(stream meshv1.ModuleMesh_StreamCallServer) error {
	return fmt.Errorf("StreamCall not supported for scheduler-cron mesh handler")
}

func (m *Module) scheduleTask(task contracts.SchedulerTask, once bool) (string, error) {
	if m.srv == nil || m.store == nil {
		return "", fmt.Errorf("scheduler not initialized")
	}
	return m.store.AddWithOptions(task.Name, task.CronExpr, task.Payload, task.Timeout, task.Meta, m.srv.OnFire, cronstore.AddOptions{Once: once})
}

// Server returns the HTTP server for tests.
func (m *Module) Server() *server.Server {
	return m.srv
}
