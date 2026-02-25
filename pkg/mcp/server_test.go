package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
	"gopkg.in/yaml.v3"
)

// runServerRequest sends a single JSON-RPC request and decodes the response.
// Uses io.Pipe to synchronize the server goroutine with the reader, avoiding
// data races on shared buffers and eliminating flaky time.Sleep waits.
func runServerRequest(t *testing.T, server *Server, input string) JSONRPCResponse {
	t.Helper()
	pr, pw := io.Pipe()
	server.WithIO(strings.NewReader(input), pw, os.Stderr)

	go func() {
		defer pw.Close()
		server.Run(context.Background())
	}()

	var resp JSONRPCResponse
	if err := json.NewDecoder(pr).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	pr.Close()
	return resp
}

func TestServer_Initialize(t *testing.T) {
	server := NewServer(t.TempDir())
	resp := runServerRequest(t, server,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`+"\n")

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not a map")
	}

	if result["protocolVersion"] == nil {
		t.Error("missing protocolVersion in response")
	}

	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("missing serverInfo in response")
	}

	if serverInfo["name"] != "autarch" {
		t.Errorf("serverInfo.name = %v, want 'autarch'", serverInfo["name"])
	}
}

func TestServer_ToolsList(t *testing.T) {
	server := NewServer(t.TempDir())
	resp := runServerRequest(t, server,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`+"\n")

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not a map")
	}

	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("missing tools array in response")
	}

	if len(tools) < 7 {
		t.Errorf("expected at least 7 tools, got %d", len(tools))
	}

	// Check for expected tools
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := toolMap["name"].(string); ok {
			toolNames[name] = true
		}
	}

	expectedTools := []string{
		"autarch_list_prds",
		"autarch_get_prd",
		"autarch_list_tasks",
		"autarch_update_task",
		"autarch_research",
		"autarch_suggest_hunters",
		"autarch_project_status",
	}

	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("missing expected tool: %s", expected)
		}
	}
}

func TestServer_ListPRDs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create specs directory with a sample PRD
	specsDir := filepath.Join(tmpDir, ".gurgeh", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}

	prd := map[string]interface{}{
		"id":     "PRD-001",
		"title":  "Test Feature",
		"status": "draft",
	}
	prdData, _ := yaml.Marshal(prd)
	if err := os.WriteFile(filepath.Join(specsDir, "PRD-001.yaml"), prdData, 0644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(tmpDir)
	resp := runServerRequest(t, server,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"autarch_list_prds","arguments":{}}}`+"\n")

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	// Parse the content from the tool result
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}

	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("missing content in response")
	}

	contentBlock, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatal("content block is not a map")
	}

	text, ok := contentBlock["text"].(string)
	if !ok {
		t.Fatal("missing text in content block")
	}

	var prdsResult map[string]interface{}
	if err := json.Unmarshal([]byte(text), &prdsResult); err != nil {
		t.Fatalf("failed to parse PRDs result: %v", err)
	}

	if count, ok := prdsResult["count"].(float64); !ok || count != 1 {
		t.Errorf("expected count=1, got %v", prdsResult["count"])
	}
}

func TestServer_SuggestHunters(t *testing.T) {
	tests := []struct {
		query           string
		expectedHunters []string
	}{
		{"github repository for react", []string{"github-scout"}},
		{"medical research on diabetes", []string{"pubmed"}},
		{"academic papers on machine learning", []string{"openalex"}},
		{"react framework documentation", []string{"context7"}},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			server := NewServer(t.TempDir())
			params := map[string]interface{}{"name": "autarch_suggest_hunters", "arguments": map[string]interface{}{"query": tt.query}}
			paramsJSON, _ := json.Marshal(params)
			input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` + string(paramsJSON) + `}` + "\n"

			resp := runServerRequest(t, server, input)

			if resp.Error != nil {
				t.Errorf("unexpected error: %v", resp.Error)
			}

			// Verify expected hunters are suggested
			result := resp.Result.(map[string]interface{})
			content := result["content"].([]interface{})
			contentBlock := content[0].(map[string]interface{})
			text := contentBlock["text"].(string)

			for _, expected := range tt.expectedHunters {
				if !strings.Contains(text, expected) {
					t.Errorf("expected hunter %q not found in response", expected)
				}
			}
		})
	}
}

func TestServer_ProjectStatus(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some data
	specsDir := filepath.Join(tmpDir, ".gurgeh", "specs")
	tasksDir := filepath.Join(tmpDir, ".coldwine", "tasks")
	os.MkdirAll(specsDir, 0755)
	os.MkdirAll(tasksDir, 0755)

	// Add PRDs
	for i, status := range []string{"draft", "draft", "approved"} {
		prd := map[string]interface{}{"id": i, "status": status}
		data, _ := yaml.Marshal(prd)
		os.WriteFile(filepath.Join(specsDir, "PRD-00"+string(rune('1'+i))+".yaml"), data, 0644)
	}

	// Add tasks
	for i, status := range []string{"pending", "in_progress", "completed", "completed"} {
		task := map[string]interface{}{"id": i, "status": status}
		data, _ := yaml.Marshal(task)
		os.WriteFile(filepath.Join(tasksDir, "TASK-00"+string(rune('1'+i))+".yaml"), data, 0644)
	}

	server := NewServer(tmpDir)
	resp := runServerRequest(t, server,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"autarch_project_status","arguments":{}}}`+"\n")

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	result := resp.Result.(map[string]interface{})
	content := result["content"].([]interface{})
	contentBlock := content[0].(map[string]interface{})
	text := contentBlock["text"].(string)

	var status map[string]interface{}
	if err := json.Unmarshal([]byte(text), &status); err != nil {
		t.Fatalf("failed to parse status: %v", err)
	}

	prds := status["prds"].(map[string]interface{})
	if total := prds["total"].(float64); total != 3 {
		t.Errorf("expected 3 PRDs, got %v", total)
	}

	tasks := status["tasks"].(map[string]interface{})
	if total := tasks["total"].(float64); total != 4 {
		t.Errorf("expected 4 tasks, got %v", total)
	}
}

func TestMCP_WriteToolRequiresWriteScope(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".coldwine", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	taskData := []byte("id: TASK-001\nstatus: pending\n")
	if err := os.WriteFile(filepath.Join(tasksDir, "TASK-001.yaml"), taskData, 0o644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(tmpDir)
	ctx := WithCaller(context.Background(), CallerInfo{
		AgentID: "reader-agent",
		Scopes:  []string{"read"},
	})

	call := map[string]any{
		"name": "autarch_update_task",
		"arguments": map[string]any{
			"id":     "TASK-001",
			"status": "completed",
		},
	}
	raw, _ := json.Marshal(call)
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  raw,
	}
	var output bytes.Buffer
	server.WithIO(strings.NewReader(""), &output, os.Stderr)

	server.handleToolsCall(ctx, req)

	var resp JSONRPCResponse
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected forbidden error")
	}
	if resp.Error.Code != -32603 {
		t.Fatalf("expected error code -32603, got %d", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "Forbidden") {
		t.Fatalf("expected forbidden message, got %q", resp.Error.Message)
	}
}

func TestMCP_ReadToolAllowedWithReadScope(t *testing.T) {
	tmpDir := t.TempDir()
	specsDir := filepath.Join(tmpDir, ".gurgeh", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prdData := []byte("id: PRD-001\ntitle: Test\nstatus: draft\n")
	if err := os.WriteFile(filepath.Join(specsDir, "PRD-001.yaml"), prdData, 0o644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(tmpDir)
	ctx := WithCaller(context.Background(), CallerInfo{
		AgentID: "reader-agent",
		Scopes:  []string{"read"},
	})

	call := map[string]any{
		"name":      "autarch_list_prds",
		"arguments": map[string]any{},
	}
	raw, _ := json.Marshal(call)
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  raw,
	}
	var output bytes.Buffer
	server.WithIO(strings.NewReader(""), &output, os.Stderr)

	server.handleToolsCall(ctx, req)

	var resp JSONRPCResponse
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestMCP_NoCallerInfo_DefaultAllowsLegacyBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".coldwine", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	taskData := []byte("id: TASK-001\nstatus: pending\n")
	taskPath := filepath.Join(tasksDir, "TASK-001.yaml")
	if err := os.WriteFile(taskPath, taskData, 0o644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(tmpDir)

	call := map[string]any{
		"name": "autarch_update_task",
		"arguments": map[string]any{
			"id":     "TASK-001",
			"status": "completed",
		},
	}
	raw, _ := json.Marshal(call)
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  raw,
	}
	var output bytes.Buffer
	server.WithIO(strings.NewReader(""), &output, os.Stderr)

	server.handleToolsCall(context.Background(), req)

	var resp JSONRPCResponse
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	updated, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatalf("read updated task: %v", err)
	}
	if !strings.Contains(string(updated), "status: completed") {
		t.Fatalf("expected status update in task file, got:\n%s", string(updated))
	}
}

func TestMCP_PathTraversal_GetPRDRejected(t *testing.T) {
	server := NewServer(t.TempDir())
	_, err := server.handleGetPRD(context.Background(), map[string]interface{}{
		"id": "../../etc/passwd",
	})
	if err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestMCP_PathTraversal_UpdateTaskRejected(t *testing.T) {
	server := NewServer(t.TempDir())
	_, err := server.handleUpdateTask(context.Background(), map[string]interface{}{
		"id":     "../../malicious",
		"status": "completed",
	})
	if err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestMCP_SendMessageUsesCallerIdentity(t *testing.T) {
	tmpDir := t.TempDir()
	server := NewServer(tmpDir)

	ctx := WithCaller(context.Background(), CallerInfo{
		AgentID: "agent-007",
		Scopes:  []string{"write"},
	})

	_, err := server.handleSendMessage(ctx, map[string]interface{}{
		"to":      "gurgeh",
		"subject": "hello",
		"body":    "world",
	})
	if err != nil {
		t.Fatalf("handleSendMessage: %v", err)
	}

	queueDir := filepath.Join(tmpDir, ".intermute", "queues", "gurgeh")
	entries, err := os.ReadDir(queueDir)
	if err != nil {
		t.Fatalf("read queue dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 queued message, got %d", len(entries))
	}

	raw, err := os.ReadFile(filepath.Join(queueDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read queued message: %v", err)
	}
	var msg map[string]interface{}
	if err := yamlsafe.Decode(raw, &msg); err != nil {
		t.Fatalf("unmarshal queued message: %v", err)
	}
	if from := fmt.Sprint(msg["from"]); from != "agent-007" {
		t.Fatalf("expected sender agent-007, got %q", from)
	}
}
