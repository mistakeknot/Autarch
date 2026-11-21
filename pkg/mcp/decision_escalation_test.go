package mcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func TestMCPSameRequestEscalates(t *testing.T) {
	for _, field := range []string{"urgent", "keep_list"} {
		t.Run(field, func(t *testing.T) {
			creates := 0
			desc := ""
			commented := false
			f := &homeask.CardFiler{StateDir: t.TempDir(), LockDir: t.TempDir(), TasksProject: "P1", Now: func() time.Time { return time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC) }}
			f.Run = func(_ context.Context, _ []string, args ...string) homeask.BBResult {
				var reply any
				switch strings.Join(args[:min(len(args), 3)], " ") {
				case "home binding P1":
					reply = map[string]any{"home_project": "autarch", "state": "confirmed"}
				case "home get --request":
					reply = map[string]any{"status": "absent"}
				case "tasks project list":
					reply = map[string]any{"projects": []any{map[string]any{"id": "P1"}}}
				case "tasks label list":
					reply = map[string]any{"labels": []any{map[string]any{"name": "needs-mk"}}}
				case "tasks list --status":
					tasks := []any{}
					if creates > 0 {
						tasks = append(tasks, map[string]any{"id": "T1", "projectId": "P1", "description": desc})
					}
					reply = map[string]any{"tasks": tasks, "nextCursor": nil}
				case "tasks create --project":
					creates++
					for i, arg := range args {
						if arg == "--description-file" {
							raw, err := os.ReadFile(args[i+1])
							if err != nil {
								t.Fatal(err)
							}
							desc = string(raw)
						}
					}
					reply = map[string]any{"task": map[string]any{"id": "T1", "projectId": "P1"}}
				case "tasks comment T1":
					commented = true
					reply = map[string]any{"comment": map[string]any{"id": "c1", "kind": "agent", "threadId": "thr-m"}}
				case "tasks show T1":
					comments := []any{}
					if commented {
						comments = append(comments, map[string]any{"id": "c1", "kind": "agent", "threadId": "thr-m"})
					}
					reply = map[string]any{"task": map[string]any{"id": "T1", "projectId": "P1", "description": desc}, "comments": comments}
				default:
					t.Errorf("unexpected %v", args)
					return homeask.BBResult{Code: 1}
				}
				raw, _ := json.Marshal(reply)
				return homeask.BBResult{Stdout: raw}
			}
			t.Setenv("BB_THREAD_ID", "")
			args := decisionArgs()
			args["project"] = "autarch"
			s := newTestServer(t.TempDir()).WithFiler(f)
			ctx := WithCaller(context.Background(), CallerInfo{AgentID: "writer", Scopes: []string{"write"}})
			response := callTool(t, s, ctx, "autarch_file_decision", args)
			if !strings.Contains(marshal(response.Result), "queued") {
				t.Fatal(response)
			}
			if field == "urgent" {
				args[field] = true
			} else {
				args[field] = "spend"
			}
			response = callTool(t, s, ctx, "autarch_file_decision", args)
			if response.Error != nil || creates != 1 || !strings.Contains(marshal(response.Result), "T1") {
				t.Fatalf("same request held: %+v", response)
			}
			delete(args, field)
			response = callTool(t, s, ctx, "autarch_file_decision", args)
			if response.Error != nil || creates != 1 || !strings.Contains(marshal(response.Result), "T1") {
				t.Fatalf("old request replay failed: %+v", response)
			}
			budget, err := f.BudgetStatus("autarch")
			if err != nil || budget.Ordinary != 0 || budget.Exempt != 1 {
				t.Fatal(budget, err)
			}
		})
	}
}
