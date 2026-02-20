package tui

import (
	"strings"
	"testing"
)

func TestStatusSymbolKernelStatuses(t *testing.T) {
	// These are the status values emitted by ic CLI for runs and dispatches
	statuses := []string{
		"running",
		"completed",
		"active",
		"failed",
		"waiting",
		"idle",
		"in_progress",
		"done",
		"blocked",
	}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			result := StatusSymbol(status)
			if result == "" {
				t.Errorf("StatusSymbol(%q) returned empty string", status)
			}
			if strings.Contains(result, "?") {
				t.Errorf("StatusSymbol(%q) returned unknown symbol '?'", status)
			}
		})
	}
}

func TestStatusSymbolUnknown(t *testing.T) {
	result := StatusSymbol("nonexistent_status_xyz")
	if !strings.Contains(result, "?") {
		t.Error("expected '?' for unknown status")
	}
}

func TestStatusIndicatorKernelStatuses(t *testing.T) {
	tests := []struct {
		status string
		want   string // expected text (case-sensitive) within the styled output
	}{
		{"running", "RUNNING"},
		{"completed", "DONE"},
		{"active", "ACTIVE"},
		{"failed", "FAILED"},
		{"waiting", "WAITING"},
		{"idle", "IDLE"},
		{"blocked", "BLOCKED"},
		{"in_progress", "IN PROGRESS"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := StatusIndicator(tt.status)
			if !strings.Contains(result, tt.want) {
				t.Errorf("StatusIndicator(%q) = %q, want substring %q", tt.status, result, tt.want)
			}
		})
	}
}

func TestStatusIndicatorUnknown(t *testing.T) {
	result := StatusIndicator("nonexistent_status_xyz")
	if !strings.Contains(result, "UNKNOWN") {
		t.Error("expected 'UNKNOWN' for unrecognized status")
	}
}

func TestAgentBadgeKnownTypes(t *testing.T) {
	// AgentBadge renders with capitalized display names
	tests := []struct {
		agentType string
		wantText  string
	}{
		{"claude", "Claude"},
		{"claude-code", "Claude"},
		{"codex", "Codex"},
		{"codex-cli", "Codex"},
		{"aider", "Aider"},
		{"cursor", "Cursor"},
	}

	for _, tt := range tests {
		t.Run(tt.agentType, func(t *testing.T) {
			badge := AgentBadge(tt.agentType)
			if badge == "" {
				t.Errorf("AgentBadge(%q) returned empty string", tt.agentType)
			}
			if !strings.Contains(badge, tt.wantText) {
				t.Errorf("AgentBadge(%q) = %q, want substring %q", tt.agentType, badge, tt.wantText)
			}
		})
	}
}

func TestAgentBadgeFallback(t *testing.T) {
	badge := AgentBadge("custom-agent-v2")
	if !strings.Contains(badge, "custom-agent-v2") {
		t.Errorf("AgentBadge fallback should contain raw agent type, got %q", badge)
	}
}

func TestPriorityBadge(t *testing.T) {
	tests := []struct {
		priority int
		want     string
	}{
		{0, "P0"},
		{1, "P1"},
		{2, "P2"},
		{3, "P3+"},
		{4, "P3+"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			badge := PriorityBadge(tt.priority)
			if !strings.Contains(badge, tt.want) {
				t.Errorf("PriorityBadge(%d) = %q, want substring %q", tt.priority, badge, tt.want)
			}
		})
	}
}
