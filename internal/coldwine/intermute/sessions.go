package intermute

import (
	"context"
	"time"

	"github.com/mistakeknot/autarch/internal/coldwine/storage"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// SessionManager defines the interface for managing sessions in Intermute.
type SessionManager interface {
	CreateSession(ctx context.Context, session intermute.Session) (intermute.Session, error)
	UpdateSession(ctx context.Context, session intermute.Session) (intermute.Session, error)
}

// SessionTracker tracks agent sessions with Intermute.
// It registers new sessions when agents start and updates their status as they work.
type SessionTracker struct {
	client  SessionManager
	project string
}

// NewSessionTracker creates a new session tracker.
// If client is nil, tracking operations become no-ops (graceful degradation).
func NewSessionTracker(client SessionManager, project string) *SessionTracker {
	return &SessionTracker{
		client:  client,
		project: project,
	}
}

// SessionStarted registers a new agent session with Intermute.
// Call this when an agent session is created in Coldwine.
func (t *SessionTracker) SessionStarted(ctx context.Context, session storage.AgentSession) (intermute.Session, error) {
	if t.client == nil {
		return intermute.Session{}, nil
	}

	intermuteSession := mapAgentSessionToIntermute(session, t.project)
	return t.client.CreateSession(ctx, intermuteSession)
}

// SessionEnded updates the session status in Intermute to indicate it's no longer active.
// The intermuteID is the ID returned from SessionStarted.
func (t *SessionTracker) SessionEnded(ctx context.Context, intermuteID string, session storage.AgentSession) error {
	if t.client == nil {
		return nil
	}

	intermuteSession := mapAgentSessionToIntermute(session, t.project)
	intermuteSession.ID = intermuteID
	intermuteSession.Status = intermute.SessionStatusIdle
	intermuteSession.UpdatedAt = time.Now()

	_, err := t.client.UpdateSession(ctx, intermuteSession)
	return err
}

// SessionStateChanged updates the session status based on state changes.
// Use this when an agent transitions between working, waiting, blocked states.
func (t *SessionTracker) SessionStateChanged(ctx context.Context, intermuteID string, session storage.AgentSession) error {
	if t.client == nil {
		return nil
	}

	intermuteSession := mapAgentSessionToIntermute(session, t.project)
	intermuteSession.ID = intermuteID
	intermuteSession.UpdatedAt = time.Now()

	_, err := t.client.UpdateSession(ctx, intermuteSession)
	return err
}

// mapAgentSessionToIntermute converts a Coldwine AgentSession to an Intermute Session.
func mapAgentSessionToIntermute(session storage.AgentSession, project string) intermute.Session {
	return intermute.Session{
		// ID is assigned by Intermute on creation
		Project:   project,
		Name:      session.ID,
		Agent:     session.AgentName,
		TaskID:    session.TaskID,
		Status:    mapAgentStateToSessionStatus(session.State),
		StartedAt: session.CreatedAt,
		UpdatedAt: session.LastActiveAt,
	}
}

// mapAgentStateToSessionStatus converts Coldwine agent states to Intermute session status.
// Mapping:
//   - working -> running
//   - waiting -> idle
//   - blocked -> error (indicates needs attention)
//   - done    -> idle
func mapAgentStateToSessionStatus(state string) intermute.SessionStatus {
	switch state {
	case "working":
		return intermute.SessionStatusRunning
	case "waiting":
		return intermute.SessionStatusIdle
	case "blocked":
		return intermute.SessionStatusError
	case "done":
		return intermute.SessionStatusIdle
	default:
		return intermute.SessionStatusIdle
	}
}
