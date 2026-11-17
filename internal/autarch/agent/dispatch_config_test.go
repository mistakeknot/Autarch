package agent

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateWithOutputRejectsConfiguredBackend(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "api")
	_, err := (&Agent{Type: TypeClaude}).GenerateWithOutput(context.Background(), GenerateRequest{Prompt: "hello"}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported dispatch backend") {
		t.Fatalf("dispatch did not apply env: %v", err)
	}
}
