package patrol

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

func TestClassifyFailure(t *testing.T) {
	tests := []struct {
		name         string
		agent        mycroft.AgentView
		gitDirty     bool
		gitCorrupted bool
		phase        string
		want         mycroft.FailureClass
	}{
		{
			name:  "active agent is healthy",
			agent: mycroft.AgentView{Status: "active"},
			want:  mycroft.FailureHealthy,
		},
		{
			name:  "idle agent is healthy",
			agent: mycroft.AgentView{Status: "idle"},
			want:  mycroft.FailureHealthy,
		},
		{
			name:         "corrupted git state",
			agent:        mycroft.AgentView{Status: "crashed"},
			gitCorrupted: true,
			want:         mycroft.FailureCorrupted,
		},
		{
			name:  "crashed with clean git",
			agent: mycroft.AgentView{Status: "crashed"},
			want:  mycroft.FailureClean,
		},
		{
			name:     "stuck with dirty git",
			agent:    mycroft.AgentView{Status: "stuck"},
			gitDirty: true,
			want:     mycroft.FailureDirty,
		},
		{
			name:     "stuck but healthy with dirty git",
			agent:    mycroft.AgentView{Status: "stuck", Health: mycroft.HealthReport{IsHealthy: true}},
			gitDirty: true,
			want:     mycroft.FailureDegraded,
		},
		{
			name:     "crashed with dirty git",
			agent:    mycroft.AgentView{Status: "crashed"},
			gitDirty: true,
			want:     mycroft.FailureDirty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyFailure(tt.agent, tt.gitDirty, tt.gitCorrupted, tt.phase)
			if got != tt.want {
				t.Errorf("ClassifyFailure() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsStuck(t *testing.T) {
	tests := []struct {
		name     string
		lastSeen time.Duration // how long ago
		phase    string
		want     bool
	}{
		{"build phase just started", 1 * time.Minute, "build", false},
		{"build phase stuck", 6 * time.Minute, "build", true},
		{"brainstorm phase not stuck", 10 * time.Minute, "brainstorm", false},
		{"brainstorm phase stuck", 16 * time.Minute, "brainstorm", true},
		{"unknown phase uses default", 6 * time.Minute, "unknown", true},
		{"research phase not stuck", 12 * time.Minute, "research", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lastSeen := time.Now().Add(-tt.lastSeen)
			got := IsStuck(lastSeen, tt.phase)
			if got != tt.want {
				t.Errorf("IsStuck(%v, %q) = %v, want %v", tt.lastSeen, tt.phase, got, tt.want)
			}
		})
	}
}

func TestIsClaimStale(t *testing.T) {
	tests := []struct {
		name         string
		age          time.Duration
		hasHeartbeat bool
		want         bool
	}{
		{"fresh dispatch, no heartbeat", 30 * time.Second, false, false},
		{"stale dispatch, no heartbeat", 2 * time.Minute, false, true},
		{"running with heartbeat, fresh", 10 * time.Minute, true, false},
		{"running with heartbeat, stale", 46 * time.Minute, true, true},
		{"dispatch boundary (just under)", 89 * time.Second, false, false},
		{"running boundary (just under)", 44 * time.Minute, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claimedAt := time.Now().Add(-tt.age)
			got := IsClaimStale(claimedAt, tt.hasHeartbeat)
			if got != tt.want {
				t.Errorf("IsClaimStale(%v, %v) = %v, want %v", tt.age, tt.hasHeartbeat, got, tt.want)
			}
		})
	}
}
