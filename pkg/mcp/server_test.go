package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestServer_Initialize(t *testing.T) {
	tmpDir := t.TempDir()
	server := NewServer(tmpDir)

	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	var output bytes.Buffer

	server.WithIO(strings.NewReader(input), &output, os.Stderr)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go server.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	var resp JSONRPCResponse
	if err := json.NewDecoder(&output).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

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
	tmpDir := t.TempDir()
	server := NewServer(tmpDir)

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n"
	var output bytes.Buffer

	server.WithIO(strings.NewReader(input), &output, os.Stderr)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go server.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	var resp JSONRPCResponse
	if err := json.NewDecoder(&output).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

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

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"autarch_list_prds","arguments":{}}}` + "\n"
	var output bytes.Buffer

	server.WithIO(strings.NewReader(input), &output, os.Stderr)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go server.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	var resp JSONRPCResponse
	if err := json.NewDecoder(&output).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

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
	tmpDir := t.TempDir()
	server := NewServer(tmpDir)

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
			params := map[string]interface{}{"name": "autarch_suggest_hunters", "arguments": map[string]interface{}{"query": tt.query}}
			paramsJSON, _ := json.Marshal(params)
			input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` + string(paramsJSON) + `}` + "\n"

			var output bytes.Buffer
			server.WithIO(strings.NewReader(input), &output, os.Stderr)

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			go server.Run(ctx)
			time.Sleep(50 * time.Millisecond)

			var resp JSONRPCResponse
			if err := json.NewDecoder(&output).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

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

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"autarch_project_status","arguments":{}}}` + "\n"
	var output bytes.Buffer

	server.WithIO(strings.NewReader(input), &output, os.Stderr)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go server.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	var resp JSONRPCResponse
	if err := json.NewDecoder(&output).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

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
