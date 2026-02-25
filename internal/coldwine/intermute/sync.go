package intermute

import (
	"context"
	"fmt"

	"github.com/mistakeknot/autarch/pkg/autarch"
)

// Syncer provides bidirectional synchronization between Coldwine's local
// SQLite state and the Intermute coordination API via autarch.Client.
// Follows the same push-first upsert pattern as Gurgeh's spec syncer.
type Syncer struct {
	client *autarch.Client
}

// NewSyncer creates a Coldwine syncer.
// If client is nil, all operations are no-ops (offline mode).
func NewSyncer(client *autarch.Client) *Syncer {
	return &Syncer{client: client}
}

// SyncResult summarizes the outcome of a sync operation.
type SyncResult struct {
	Pushed   int // items successfully pushed
	Pulled   int // items successfully pulled
	Errors   []error
	Skipped  int // items unchanged
}

// PushEpics pushes local epics to Intermute. Uses upsert: create if missing, update if exists.
func (s *Syncer) PushEpics(ctx context.Context, epics []autarch.Epic) SyncResult {
	var r SyncResult
	if s.client == nil {
		return r
	}
	for _, e := range epics {
		_, err := s.client.GetEpic(e.ID)
		if err != nil {
			// Doesn't exist remotely — create
			if _, err := s.client.CreateEpic(e); err != nil {
				r.Errors = append(r.Errors, fmt.Errorf("push epic %s: %w", e.ID, err))
				continue
			}
		} else {
			// Exists — update
			if _, err := s.client.UpdateEpic(e); err != nil {
				r.Errors = append(r.Errors, fmt.Errorf("update epic %s: %w", e.ID, err))
				continue
			}
		}
		r.Pushed++
	}
	return r
}

// PushStories pushes local stories to Intermute.
func (s *Syncer) PushStories(ctx context.Context, stories []autarch.Story) SyncResult {
	var r SyncResult
	if s.client == nil {
		return r
	}
	for _, st := range stories {
		_, err := s.client.GetStory(st.ID)
		if err != nil {
			if _, err := s.client.CreateStory(st); err != nil {
				r.Errors = append(r.Errors, fmt.Errorf("push story %s: %w", st.ID, err))
				continue
			}
		} else {
			if _, err := s.client.UpdateStory(st); err != nil {
				r.Errors = append(r.Errors, fmt.Errorf("update story %s: %w", st.ID, err))
				continue
			}
		}
		r.Pushed++
	}
	return r
}

// PushTasks pushes local tasks to Intermute.
func (s *Syncer) PushTasks(ctx context.Context, tasks []autarch.Task) SyncResult {
	var r SyncResult
	if s.client == nil {
		return r
	}
	for _, t := range tasks {
		_, err := s.client.GetTask(t.ID)
		if err != nil {
			if _, err := s.client.CreateTask(t); err != nil {
				r.Errors = append(r.Errors, fmt.Errorf("push task %s: %w", t.ID, err))
				continue
			}
		} else {
			if _, err := s.client.UpdateTask(t); err != nil {
				r.Errors = append(r.Errors, fmt.Errorf("update task %s: %w", t.ID, err))
				continue
			}
		}
		r.Pushed++
	}
	return r
}

// PullEpics downloads epics from Intermute for the given spec.
func (s *Syncer) PullEpics(ctx context.Context, specID string) ([]autarch.Epic, error) {
	if s.client == nil {
		return nil, nil
	}
	return s.client.ListEpics(specID)
}

// PullStories downloads stories from Intermute for the given epic.
func (s *Syncer) PullStories(ctx context.Context, epicID string) ([]autarch.Story, error) {
	if s.client == nil {
		return nil, nil
	}
	return s.client.ListStories(epicID)
}

// PullTasks downloads tasks from Intermute with optional filters.
func (s *Syncer) PullTasks(ctx context.Context, status, agent string) ([]autarch.Task, error) {
	if s.client == nil {
		return nil, nil
	}
	return s.client.ListTasks(status, agent)
}

// PushAll pushes all local epics, stories, and tasks to Intermute.
// Returns a combined SyncResult with aggregate counts.
func (s *Syncer) PushAll(ctx context.Context, epics []autarch.Epic, stories []autarch.Story, tasks []autarch.Task) SyncResult {
	var combined SyncResult
	if s.client == nil {
		return combined
	}

	er := s.PushEpics(ctx, epics)
	combined.Pushed += er.Pushed
	combined.Errors = append(combined.Errors, er.Errors...)

	sr := s.PushStories(ctx, stories)
	combined.Pushed += sr.Pushed
	combined.Errors = append(combined.Errors, sr.Errors...)

	tr := s.PushTasks(ctx, tasks)
	combined.Pushed += tr.Pushed
	combined.Errors = append(combined.Errors, tr.Errors...)

	return combined
}
