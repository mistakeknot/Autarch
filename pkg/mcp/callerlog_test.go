package mcp

import (
	"context"
	"encoding/json"
	"github.com/mistakeknot/autarch/pkg/callerlog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopeLoggingPreservesDecision(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	log := callerlog.New(filepath.Join(t.TempDir(), "audit"))
	s := NewServer(t.TempDir()).WithCallerLog(log)
	runs := 0
	s.RegisterTool(Tool{Name: "probe", RequiredScope: "write", Handler: func(context.Context, map[string]interface{}) (interface{}, error) { runs++; return "ok", nil }})
	for _, scopes := range [][]string{nil, {"read"}, {"write"}, {"admin"}} {
		c := CallerInfo{AgentID: "claimed", Scopes: scopes}
		resp := callTool(t, s, WithCaller(context.Background(), c), "probe", map[string]any{"secret": "argument-probe"})
		if (resp.Error == nil) != c.HasScope("write") {
			t.Fatal("scope decision changed")
		}
	}
	resp := callTool(t, s, context.Background(), "probe", nil)
	if resp.Error == nil || runs != 2 {
		t.Fatal("missing caller allowed")
	}
	log.Close()
	data, err := os.ReadFile(filepath.Join(log.Dir(), "caller-log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "argument-probe") {
		t.Fatal("arguments leaked")
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 {
		t.Fatal(len(lines))
	}
	for i, line := range lines {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		if row["tool"] != "probe" || row["required_scope"] != "write" || row["draft_would_allow"] != (i < 4) {
			t.Fatal(row)
		}
	}
}
