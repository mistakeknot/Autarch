package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/pkg/callerlog"
)

func TestMCP_ListsTools(t *testing.T) {
	project := t.TempDir()
	specs := filepath.Join(project, ".gurgeh", "specs")
	if err := os.MkdirAll(specs, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specs, "PRD-001.yaml"), []byte("id: PRD-001\ntitle: Scoped MCP fixture\nstatus: draft\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := mcpCmdWithCallerLog(callerlog.New(""))
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"autarch_list_prds","arguments":{}}}` + "\n")
	var out, errBuf bytes.Buffer
	cmd.SetIn(in)
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--project", project})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errBuf.String())
	}
	if !strings.Contains(out.String(), "autarch_list_prds") {
		t.Fatalf("autarch_list_prds not listed:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Scoped MCP fixture") {
		t.Fatalf("scoped tools/call did not return the project PRD:\n%s", out.String())
	}
}
