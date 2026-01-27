package agenttargets

import (
	"context"
	"testing"
	"time"
)

func TestDefaultSafetyPolicy(t *testing.T) {
	p := DefaultSafetyPolicy()
	if p.Timeout != 30*time.Minute {
		t.Errorf("timeout = %v, want 30m", p.Timeout)
	}
	if p.MaxOutputBytes != 10*1024*1024 {
		t.Errorf("max output = %d, want 10MB", p.MaxOutputBytes)
	}
	if !p.Sandbox {
		t.Error("sandbox should be true")
	}
}

func TestExecAgentRunner_Timeout(t *testing.T) {
	runner := NewExecAgentRunner()
	target := ResolvedTarget{
		Name:    "test-sleep",
		Command: "sleep",
		Args:    []string{"60"},
	}
	policy := SafetyPolicy{
		Timeout: 200 * time.Millisecond,
	}

	handle, err := runner.Run(context.Background(), target, policy, "/tmp", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	result, err := handle.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !result.TimedOut {
		t.Error("expected TimedOut=true")
	}
	if result.Duration < 150*time.Millisecond {
		t.Errorf("duration %v too short", result.Duration)
	}
}

func TestExecAgentRunner_OutputTruncation(t *testing.T) {
	runner := NewExecAgentRunner()
	// "yes" outputs endless "y\n"; we cap at 100 bytes.
	target := ResolvedTarget{
		Name:    "test-yes",
		Command: "yes",
	}
	policy := SafetyPolicy{
		Timeout:        2 * time.Second,
		MaxOutputBytes: 100,
	}

	handle, err := runner.Run(context.Background(), target, policy, "/tmp", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	result, err := handle.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !result.Truncated {
		t.Error("expected Truncated=true")
	}
	if int64(len(result.Output)) != 100 {
		t.Errorf("output len = %d, want 100", len(result.Output))
	}
}

func TestExecAgentRunner_SandboxArgs(t *testing.T) {
	// Use "echo" as the command so we can see the args in output.
	runner := NewExecAgentRunner()
	target := ResolvedTarget{
		Name:    "claude",
		Command: "echo",
	}
	policy := SafetyPolicy{
		Sandbox: true,
	}

	handle, err := runner.Run(context.Background(), target, policy, "/tmp", "hello world")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	result, err := handle.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	got := string(result.Output)
	// echo should print "--sandbox hello world\n"
	if got != "--sandbox hello world\n" {
		t.Errorf("output = %q, want %q", got, "--sandbox hello world\n")
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestExecAgentRunner_NoSandboxForUnknownAgent(t *testing.T) {
	runner := NewExecAgentRunner()
	target := ResolvedTarget{
		Name:    "custom-agent",
		Command: "echo",
	}
	policy := SafetyPolicy{
		Sandbox: true,
	}

	handle, err := runner.Run(context.Background(), target, policy, "/tmp", "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	result, err := handle.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	got := string(result.Output)
	// No sandbox flag for unknown agent, just the prompt.
	if got != "test\n" {
		t.Errorf("output = %q, want %q", got, "test\n")
	}
}

func TestExecAgentRunner_Cancel(t *testing.T) {
	runner := NewExecAgentRunner()
	target := ResolvedTarget{
		Name:    "test-sleep",
		Command: "sleep",
		Args:    []string{"60"},
	}
	policy := SafetyPolicy{}

	handle, err := runner.Run(context.Background(), target, policy, "/tmp", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Cancel after a short delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		handle.Cancel()
	}()

	result, err := handle.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code after cancel")
	}
}

func TestExecAgentRunner_EmptyCommand(t *testing.T) {
	runner := NewExecAgentRunner()
	target := ResolvedTarget{Name: "empty"}
	_, err := runner.Run(context.Background(), target, SafetyPolicy{}, "/tmp", "")
	if err == nil {
		t.Error("expected error for empty command")
	}
}
