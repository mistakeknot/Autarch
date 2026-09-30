package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
)

type fakeFiler struct {
	got  homeask.Ask
	err  error
	runs int
}

func (f *fakeFiler) File(_ context.Context, a homeask.Ask) (string, error) {
	f.runs++
	f.got = a
	return "dec-9", f.err
}

func callTool(t *testing.T, s *Server, ctx context.Context, name string, args map[string]any) JSONRPCResponse {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"name": name, "arguments": args})
	var out bytes.Buffer
	s.WithIO(strings.NewReader(""), &out, os.Stderr)
	s.handleToolsCall(ctx, &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: raw})
	var resp JSONRPCResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v: %s", err, out.String())
	}
	return resp
}

func decisionArgs() map[string]any {
	return map[string]any{
		"kind": "decide", "v": 1, "subject": "mcp/x: order", "question": "Which order?", "thread": "thr-m",
		"options": []map[string]any{
			{"id": "a", "label": "A first", "kind": "instruction", "reversible": true, "instruction": "Do A."},
			{"id": "b", "label": "B first", "kind": "needs-context"},
		},
	}
}

func TestMCPToolsListIncludesFileDecision(t *testing.T) {
	resp := runServerRequest(t, NewServer(t.TempDir()), `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`+"\n")
	tools := resp.Result.(map[string]any)["tools"].([]any)
	for _, tool := range tools {
		if tool.(map[string]any)["name"] == "autarch_file_decision" {
			return
		}
	}
	t.Fatal("autarch_file_decision is not listed")
}

func TestMCPFileDecisionRequiresWriteScope(t *testing.T) {
	f := &fakeFiler{}
	s := NewServer(t.TempDir()).WithFiler(f)
	ctx := WithCaller(context.Background(), CallerInfo{AgentID: "r", Scopes: []string{"read"}})
	resp := callTool(t, s, ctx, "autarch_file_decision", decisionArgs())
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "Forbidden") || f.runs != 0 {
		t.Fatalf("resp = %+v, runs %d", resp, f.runs)
	}
}

func TestMCPFileDecisionFilesWithProjectDefaultsAndThread(t *testing.T) {
	root := t.TempDir()
	f := &fakeFiler{}
	s := NewServer(root).WithFiler(f)
	t.Setenv("BB_THREAD_ID", "")
	ctx := WithCaller(context.Background(), CallerInfo{AgentID: "w", Scopes: []string{"write"}})
	resp := callTool(t, s, ctx, "autarch_file_decision", decisionArgs())
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	if f.got.Thread != "thr-m" || f.got.Asker != "thread" || f.got.ProjectRoot != root || f.got.Project == "" {
		t.Fatalf("filed %+v", f.got)
	}
	if !strings.Contains(marshal(resp.Result), "dec-9") {
		t.Fatalf("result = %v", resp.Result)
	}
}

func TestMCPFileDecisionThreadMustMatchTheCallerThread(t *testing.T) {
	f := &fakeFiler{}
	s := NewServer(t.TempDir()).WithFiler(f)
	t.Setenv("BB_THREAD_ID", "thr-real")
	ctx := WithCaller(context.Background(), CallerInfo{AgentID: "w", Scopes: []string{"write"}})
	resp := callTool(t, s, ctx, "autarch_file_decision", decisionArgs())
	result, _ := resp.Result.(map[string]any)
	if f.runs != 0 || (resp.Error == nil && result["isError"] != true) {
		t.Fatalf("filed despite a conflicting thread: %+v runs %d", resp, f.runs)
	}
}

func marshal(v any) string { b, _ := json.Marshal(v); return string(b) }
