package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestMCP_ListsTools(t *testing.T) {
	cmd := mcpCmd()
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n")
	var out, errBuf bytes.Buffer
	cmd.SetIn(in)
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--project", t.TempDir()})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errBuf.String())
	}
	if !strings.Contains(out.String(), "autarch_list_prds") {
		t.Fatalf("autarch_list_prds not listed:\n%s", out.String())
	}
}
