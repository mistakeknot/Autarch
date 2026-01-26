package intermute

import (
	"context"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/coldwine/storage"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// mockSessionClient implements SessionManager for testing
type mockSessionClient struct {
	sessions  []intermute.Session
	createErr error
	updateErr error
}

func (m *mockSessionClient) CreateSession(ctx context.Context, session intermute.Session) (intermute.Session, error) {
	if m.createErr != nil {
		return intermute.Session{}, m.createErr
	}
	session.ID = "int-sess-" + session.Name
	m.sessions = append(m.sessions, session)
	return session, nil
}

func (m *mockSessionClient) UpdateSession(ctx context.Context, session intermute.Session) (intermute.Session, error) {
	if m.updateErr != nil {
		return intermute.Session{}, m.updateErr
	}
	for i, s := range m.sessions {
		if s.ID == session.ID {
			m.sessions[i] = session
			return session, nil
		}
	}
	return session, nil
}

func TestSessionTracker_SessionStarted(t *testing.T) {
	mock := &mockSessionClient{sessions: make([]intermute.Session, 0)}
	tracker := NewSessionTracker(mock, "autarch")

	agentSession := storage.AgentSession{
		ID:           "sess-001",
		TaskID:       "TASK-001",
		AgentName:    "claude",
		AgentProgram: "claude-code",
		State:        "working",
		CreatedAt:    time.Now(),
	}

	intermuteSession, err := tracker.SessionStarted(context.Background(), agentSession)
	if err != nil {
		t.Fatalf("SessionStarted failed: %v", err)
	}

	if intermuteSession.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if len(mock.sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(mock.sessions))
	}

	created := mock.sessions[0]
	if created.Agent != "claude" {
		t.Errorf("expected agent 'claude', got %s", created.Agent)
	}
	if created.TaskID != "TASK-001" {
		t.Errorf("expected task ID 'TASK-001', got %s", created.TaskID)
	}
	if created.Status != intermute.SessionStatusRunning {
		t.Errorf("expected status 'running', got %s", created.Status)
	}
}

func TestSessionTracker_SessionEnded(t *testing.T) {
	mock := &mockSessionClient{sessions: make([]intermute.Session, 0)}
	tracker := NewSessionTracker(mock, "autarch")

	// First start a session
	agentSession := storage.AgentSession{
		ID:        "sess-002",
		TaskID:    "TASK-002",
		AgentName: "codex",
		State:     "working",
		CreatedAt: time.Now(),
	}

	intermuteSession, _ := tracker.SessionStarted(context.Background(), agentSession)

	// Then end it
	agentSession.State = "done"
	err := tracker.SessionEnded(context.Background(), intermuteSession.ID, agentSession)
	if err != nil {
		t.Fatalf("SessionEnded failed: %v", err)
	}

	// Verify status was updated
	if mock.sessions[0].Status != intermute.SessionStatusIdle {
		t.Errorf("expected status 'idle', got %s", mock.sessions[0].Status)
	}
}

func TestSessionTracker_MapAgentState(t *testing.T) {
	testCases := []struct {
		state    string
		expected intermute.SessionStatus
	}{
		{"working", intermute.SessionStatusRunning},
		{"waiting", intermute.SessionStatusIdle},
		{"blocked", intermute.SessionStatusError},
		{"done", intermute.SessionStatusIdle},
	}

	for _, tc := range testCases {
		t.Run(tc.state, func(t *testing.T) {
			result := mapAgentStateToSessionStatus(tc.state)
			if result != tc.expected {
				t.Errorf("mapAgentStateToSessionStatus(%s) = %s, want %s",
					tc.state, result, tc.expected)
			}
		})
	}
}

func TestSessionTracker_NilClientGracefulDegradation(t *testing.T) {
	tracker := NewSessionTracker(nil, "autarch")

	agentSession := storage.AgentSession{
		ID:        "sess-003",
		AgentName: "claude",
	}

	// Should return empty session but no error
	session, err := tracker.SessionStarted(context.Background(), agentSession)
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if session.ID != "" {
		t.Error("expected empty session for nil client")
	}
}
