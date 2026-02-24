package agenttargets

import (
	"testing"
)

func TestParseClaudeStreamLine_Empty(t *testing.T) {
	events := ParseClaudeStreamLine("")
	if len(events) != 0 {
		t.Errorf("expected 0 events for empty line, got %d", len(events))
	}
}

func TestParseClaudeStreamLine_InvalidJSON(t *testing.T) {
	events := ParseClaudeStreamLine("not json")
	if len(events) != 1 {
		t.Fatalf("expected 1 error event, got %d", len(events))
	}
	if events[0].Type != StreamError {
		t.Errorf("expected StreamError, got %d", events[0].Type)
	}
	if events[0].Backend != "claude" {
		t.Errorf("expected backend claude, got %q", events[0].Backend)
	}
}

func TestParseClaudeStreamLine_SessionInit(t *testing.T) {
	line := `{"type":"system","subtype":"init","session_id":"abc-123"}`
	events := ParseClaudeStreamLine(line)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != StreamSessionID {
		t.Errorf("expected StreamSessionID, got %d", events[0].Type)
	}
	if events[0].SessionID != "abc-123" {
		t.Errorf("session ID = %q, want %q", events[0].SessionID, "abc-123")
	}
}

func TestParseClaudeStreamLine_TextContent(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`
	events := ParseClaudeStreamLine(line)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != StreamText {
		t.Errorf("expected StreamText, got %d", events[0].Type)
	}
	if events[0].Text != "Hello world" {
		t.Errorf("text = %q, want %q", events[0].Text, "Hello world")
	}
}

func TestParseClaudeStreamLine_ThinkingCycle(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"content":[{"type":"thinking_start"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Let me think..."}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking_end"}]}}`,
	}

	expectedTypes := []StreamEventType{StreamThinkingStart, StreamThinking, StreamThinkingEnd}
	for i, line := range lines {
		events := ParseClaudeStreamLine(line)
		if len(events) != 1 {
			t.Fatalf("line %d: expected 1 event, got %d", i, len(events))
		}
		if events[0].Type != expectedTypes[i] {
			t.Errorf("line %d: expected type %d, got %d", i, expectedTypes[i], events[0].Type)
		}
	}
}

func TestParseClaudeStreamLine_ThinkingFallback(t *testing.T) {
	// When "thinking" field is empty, falls back to "text" field.
	line := `{"type":"assistant","message":{"content":[{"type":"thinking","text":"fallback text"}]}}`
	events := ParseClaudeStreamLine(line)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Text != "fallback text" {
		t.Errorf("text = %q, want %q", events[0].Text, "fallback text")
	}
}

func TestParseClaudeStreamLine_ToolUse(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/test.go"}}]}}`
	events := ParseClaudeStreamLine(line)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != StreamToolUse {
		t.Errorf("expected StreamToolUse, got %d", events[0].Type)
	}
	if events[0].ToolName != "Read" {
		t.Errorf("tool name = %q, want %q", events[0].ToolName, "Read")
	}
	fp, ok := events[0].ToolInput["file_path"]
	if !ok || fp != "/tmp/test.go" {
		t.Errorf("tool input file_path = %v, want /tmp/test.go", fp)
	}
}

func TestParseClaudeStreamLine_Result(t *testing.T) {
	line := `{"type":"result","result":"All done!","is_error":false}`
	events := ParseClaudeStreamLine(line)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != StreamResult {
		t.Errorf("expected StreamResult, got %d", events[0].Type)
	}
	if events[0].Text != "All done!" {
		t.Errorf("text = %q, want %q", events[0].Text, "All done!")
	}
	if events[0].IsError {
		t.Error("expected IsError=false")
	}
}

func TestParseClaudeStreamLine_ResultError(t *testing.T) {
	line := `{"type":"result","result":"something went wrong","is_error":true}`
	events := ParseClaudeStreamLine(line)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if !events[0].IsError {
		t.Error("expected IsError=true")
	}
}

func TestParseClaudeStreamLine_MultipleBlocks(t *testing.T) {
	// A single assistant message can have multiple content blocks.
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]}}`
	events := ParseClaudeStreamLine(line)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Text != "first" {
		t.Errorf("event 0 text = %q, want %q", events[0].Text, "first")
	}
	if events[1].Text != "second" {
		t.Errorf("event 1 text = %q, want %q", events[1].Text, "second")
	}
}

func TestParseClaudeStreamLine_EmptyText(t *testing.T) {
	// Empty text blocks should be skipped.
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":""}]}}`
	events := ParseClaudeStreamLine(line)
	if len(events) != 0 {
		t.Errorf("expected 0 events for empty text block, got %d", len(events))
	}
}

func TestParseClaudeStreamLine_UnknownType(t *testing.T) {
	// Unknown message types should produce no events.
	line := `{"type":"unknown","data":"stuff"}`
	events := ParseClaudeStreamLine(line)
	if len(events) != 0 {
		t.Errorf("expected 0 events for unknown type, got %d", len(events))
	}
}

func TestBuildClaudeArgs_Defaults(t *testing.T) {
	cfg := DefaultDispatchConfig()
	args := buildClaudeArgs(cfg, "hello")
	assertContains(t, args, "-p")
	assertContains(t, args, "hello")
	assertContains(t, args, "--output-format")
	assertContains(t, args, "stream-json")
	assertContains(t, args, "--verbose")
	assertContains(t, args, "--print")
}

func TestBuildClaudeArgs_WithModel(t *testing.T) {
	cfg := DefaultDispatchConfig()
	cfg.Model = "claude-sonnet-4-5-20250929"
	args := buildClaudeArgs(cfg, "test")
	assertContains(t, args, "--model")
	assertContains(t, args, "claude-sonnet-4-5-20250929")
}

func TestBuildClaudeArgs_WithResume(t *testing.T) {
	cfg := DefaultDispatchConfig()
	cfg.SessionID = "sess-123"
	args := buildClaudeArgs(cfg, "test")
	assertContains(t, args, "--resume")
	assertContains(t, args, "sess-123")
}

func TestBuildClaudeArgs_DangerousSandbox(t *testing.T) {
	cfg := DefaultDispatchConfig()
	cfg.Sandbox = "danger-full-access"
	args := buildClaudeArgs(cfg, "test")
	assertContains(t, args, "--dangerously-skip-permissions")
}

func TestBuildClaudeArgs_ExtraArgs(t *testing.T) {
	cfg := DefaultDispatchConfig()
	cfg.ExtraArgs = []string{"--max-turns", "5"}
	args := buildClaudeArgs(cfg, "test")
	assertContains(t, args, "--max-turns")
	assertContains(t, args, "5")
}

func assertContains(t *testing.T, slice []string, item string) {
	t.Helper()
	for _, s := range slice {
		if s == item {
			return
		}
	}
	t.Errorf("expected %v to contain %q", slice, item)
}
