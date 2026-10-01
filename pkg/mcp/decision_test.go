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

func TestMCPFileDecisionWithoutACallerIsDenied(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "")
	f := &fakeFiler{}
	s := NewServer(t.TempDir()).WithFiler(f)
	resp := callTool(t, s, context.Background(), "autarch_file_decision", decisionArgs())
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "Forbidden") || f.runs != 0 {
		t.Fatalf("a write tool ran without a caller: %+v runs %d", resp, f.runs)
	}
}

func TestMCPStdioCallerIsExplicitAndBounded(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr-m")
	c := StdioCaller()
	if c.AgentID != "thr-m" || !c.HasScope("read") || !c.HasScope("write") || c.HasScope("admin") {
		t.Fatalf("stdio caller = %+v", c)
	}
	f := &fakeFiler{}
	s := NewServer(t.TempDir()).WithFiler(f)
	resp := callTool(t, s, WithCaller(context.Background(), c), "autarch_file_decision", decisionArgs())
	if resp.Error != nil || f.runs != 1 {
		t.Fatalf("stdio caller could not file: %+v runs %d", resp, f.runs)
	}
}

func TestMCPFileDecisionNonStringProjectFieldsAreErrorsNotPanics(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "")
	f := &fakeFiler{}
	s := NewServer(t.TempDir()).WithFiler(f)
	ctx := WithCaller(context.Background(), CallerInfo{AgentID: "w", Scopes: []string{"write"}})
	for _, bad := range []map[string]any{{"project_root": 5}, {"project_root": nil}, {"project_root": "/tmp/x", "project": []string{"a"}}} {
		args := decisionArgs()
		for k, v := range bad {
			args[k] = v
		}
		resp := callTool(t, s, ctx, "autarch_file_decision", args)
		result, _ := resp.Result.(map[string]any)
		if f.runs != 0 || (resp.Error == nil && result["isError"] != true) {
			t.Fatalf("%v: %+v runs %d", bad, resp, f.runs)
		}
	}
}

// With no injected filer the server files a bb tasks card, never a `bb home ask`.
func TestMCPDefaultFilerIsACardFiler(t *testing.T) {
	var calls [][]string
	var env []string
	old := newDefaultFiler
	t.Cleanup(func() { newDefaultFiler = old })
	newDefaultFiler = func() homeask.Filer {
		return &homeask.CardFiler{LockDir: t.TempDir(), TasksProject: "P1", Run: func(_ context.Context, e []string, args ...string) homeask.BBResult {
			calls = append(calls, args)
			j := func(v string) homeask.BBResult { return homeask.BBResult{Stdout: []byte(v)} }
			switch strings.Join(args[:min(len(args), 3)], " ") {
			case "home get --request":
				return j(`{"status":"absent"}`)
			case "tasks label list":
				return j(`{"labels":[{"name":"needs-mk"}]}`)
			case "tasks project list":
				return j(`{"projects":[{"id":"P1"}]}`)
			case "tasks list --project":
				return j(`{"tasks":[],"nextCursor":null}`)
			case "tasks create --project":
				for _, kv := range e {
					if strings.HasPrefix(kv, "BB_THREAD_ID=") {
						env = append(env, kv)
					}
				}
				return j(`{"task":{"id":"T1","projectId":"P1"}}`)
			case "tasks comment T1":
				return j(`{"comment":{"id":"c1","kind":"agent","threadId":"thr-m"}}`)
			}
			return homeask.BBResult{Code: 1, Stderr: []byte("unexpected " + strings.Join(args, " "))}
		}}
	}
	t.Setenv("BB_THREAD_ID", "")
	s := NewServer(t.TempDir())
	ctx := WithCaller(context.Background(), CallerInfo{AgentID: "w", Scopes: []string{"write"}})
	resp := callTool(t, s, ctx, "autarch_file_decision", decisionArgs())
	if resp.Error != nil || !strings.Contains(marshal(resp.Result), "T1") {
		t.Fatalf("resp = %+v", resp)
	}
	var seq []string
	for _, c := range calls {
		seq = append(seq, strings.Join(c[:min(len(c), 2)], " "))
		if c[0] == "home" && c[1] == "ask" {
			t.Fatalf("legacy home ask ran: %v", c)
		}
	}
	if !strings.Contains(strings.Join(seq, ","), "tasks create,tasks comment") || len(env) != 1 || env[0] != "BB_THREAD_ID=thr-m" {
		t.Fatalf("seq %v env %v", seq, env)
	}
}
