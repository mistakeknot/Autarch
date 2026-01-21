package coordination

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
)

func TestLLMSummaryCommand(t *testing.T) {
	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "summary.sh")
	script := "#!/bin/sh\necho '{\"summary\":{\"participants\":[\"alice\"],\"key_points\":[\"p1\"],\"action_items\":[]},\"examples\":[{\"id\":\"m1\",\"subject\":\"Hello\",\"body\":\"Body\"}]}'\n"
	if err := os.WriteFile(cmdPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.LLMSummaryConfig{Command: cmdPath, TimeoutSeconds: 5}
	input := LLMSummaryInput{ThreadID: "t1", Messages: []LLMMessage{{ID: "m1", Sender: "alice", Subject: "Hello", Body: "Body"}}}

	out, err := RunLLMSummaryCommand(context.Background(), cfg, input)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out.Summary.KeyPoints) != 1 || len(out.Examples) != 1 {
		t.Fatalf("expected summary and example")
	}
}

func TestLLMSummaryCommandInvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "summary.sh")
	script := "#!/bin/sh\necho 'not-json'\n"
	if err := os.WriteFile(cmdPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.LLMSummaryConfig{Command: cmdPath, TimeoutSeconds: 5}
	input := LLMSummaryInput{ThreadID: "t1", Messages: []LLMMessage{{ID: "m1", Sender: "alice", Subject: "Hello", Body: "Body"}}}

	_, err := RunLLMSummaryCommand(context.Background(), cfg, input)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "llm command output invalid") {
		t.Fatalf("expected invalid output error, got %v", err)
	}
}
