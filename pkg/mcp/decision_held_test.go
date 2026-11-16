package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func TestMCPDecisionReportsHeldRequest(t *testing.T) {
	f := &homeask.CardFiler{StateDir: t.TempDir(), LockDir: t.TempDir(), TasksProject: "P1", Now: func() time.Time {
		return time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC)
	}, Run: func(_ context.Context, _ []string, args ...string) homeask.BBResult {
		response := ""
		switch strings.Join(args[:min(len(args), 3)], " ") {
		case "home binding P1":
			response = `{"home_project":"autarch","state":"confirmed"}`
		case "tasks project list":
			response = `{"projects":[{"id":"P1"}]}`
		case "home get --request":
			response = `{"status":"absent"}`
		case "tasks list --status":
			response = `{"tasks":[],"nextCursor":null}`
		default:
			t.Errorf("held request made unexpected call: %v", args)
			return homeask.BBResult{Code: 1}
		}
		return homeask.BBResult{Stdout: []byte(response)}
	}}
	t.Setenv("BB_THREAD_ID", "")
	args := decisionArgs()
	args["project"], args["request_id"] = "autarch", "held-request"
	s := NewServer(t.TempDir()).WithFiler(f)
	ctx := WithCaller(context.Background(), CallerInfo{AgentID: "writer", Scopes: []string{"write"}})
	response := callTool(t, s, ctx, "autarch_file_decision", args)
	text := marshal(response.Result)
	if response.Error != nil || !strings.Contains(text, `\"queued\": true`) || !strings.Contains(text, "held-request") || !strings.Contains(text, "queued for the Thursday batch") || strings.Contains(text, `\"id\"`) {
		t.Fatalf("held request reported as filed: %+v", response)
	}
}

func TestMCPMergeFilesImmediatelyDuringHold(t *testing.T) {
	creates := 0
	f := &homeask.CardFiler{StateDir: t.TempDir(), LockDir: t.TempDir(), TasksProject: "P1", Now: func() time.Time { return time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC) }, Run: func(_ context.Context, _ []string, args ...string) homeask.BBResult {
		response := ""
		switch strings.Join(args[:min(len(args), 3)], " ") {
		case "home binding P1":
			response = `{"home_project":"autarch","state":"confirmed"}`
		case "home get --request":
			response = `{"status":"absent"}`
		case "tasks project list":
			response = `{"projects":[{"id":"P1"}]}`
		case "tasks list --status":
			response = `{"tasks":[],"nextCursor":null}`
		case "tasks label list":
			response = `{"labels":[{"name":"needs-mk"}]}`
		case "tasks create --project":
			creates++
			response = `{"task":{"id":"T1","projectId":"P1"}}`
		case "tasks comment T1":
			response = `{"comment":{"id":"c1","kind":"agent","threadId":"thr-m"}}`
		default:
			t.Errorf("unexpected call: %v", args)
			return homeask.BBResult{Code: 1}
		}
		return homeask.BBResult{Stdout: []byte(response)}
	}}
	t.Setenv("BB_THREAD_ID", "")
	args := decisionArgs()
	args["project"] = "autarch"
	args["options"].([]map[string]any)[0]["label"] = "Manually merge it at abc123"
	s := NewServer(t.TempDir()).WithFiler(f)
	response := callTool(t, s, WithCaller(context.Background(), CallerInfo{AgentID: "writer", Scopes: []string{"write"}}), "autarch_file_decision", args)
	if response.Error != nil || creates != 1 || !strings.Contains(marshal(response.Result), "T1") {
		t.Fatalf("merge held: %+v creates=%d", response, creates)
	}
}
