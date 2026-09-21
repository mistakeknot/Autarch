// Package drafts provides persistence for in-progress onboarding state.
// Drafts are saved to ~/.autarch/drafts/{projectID}/ and can be resumed
// across sessions.
package drafts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Draft represents the complete state of an in-progress onboarding flow.
type Draft struct {
	ProjectID   string          `json:"project_id"`
	ProjectName string          `json:"project_name"`
	Description string          `json:"description"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Interview   *InterviewState `json:"interview,omitempty"`
	Research    *ResearchState  `json:"research,omitempty"`
	Epics       *EpicsState     `json:"epics,omitempty"`
}

// InterviewState captures the progress of the Gurgeh interview.
type InterviewState struct {
	CurrentStep int             `json:"current_step"`
	TotalSteps  int             `json:"total_steps"`
	Answers     map[string]any  `json:"answers"`
	Questions   []QuestionState `json:"questions"`
}

// QuestionState tracks the state of a single interview question.
type QuestionState struct {
	QuestionID     string `json:"question_id"`
	TopicKey       string `json:"topic_key"`       // e.g., "platform", "storage"
	Touched        bool   `json:"touched"`         // User has interacted
	DefaultApplied bool   `json:"default_applied"` // Research default was applied
	Answer         any    `json:"answer,omitempty"`
}

// ResearchState captures cached Pollard findings.
type ResearchState struct {
	RunID     string         `json:"run_id"`
	StartedAt time.Time      `json:"started_at"`
	Complete  bool           `json:"complete"`
	Findings  []FindingCache `json:"findings"`
	Hunters   []HunterCache  `json:"hunters"`
}

// FindingCache stores a research finding for persistence.
type FindingCache struct {
	ID          string    `json:"id"`
	TopicKey    string    `json:"topic_key"`
	HunterName  string    `json:"hunter_name"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Source      string    `json:"source"`
	Relevance   float64   `json:"relevance"`
	CollectedAt time.Time `json:"collected_at"`
}

// HunterCache stores hunter status for persistence.
type HunterCache struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Findings   int       `json:"findings"`
	Error      string    `json:"error,omitempty"`
}

// EpicsState captures proposed epics before acceptance.
type EpicsState struct {
	Proposed []EpicProposal `json:"proposed"`
	Accepted bool           `json:"accepted"`
}

// EpicProposal represents a proposed epic from Coldwine.
type EpicProposal struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Size         string   `json:"size"` // S, M, L
	Dependencies []string `json:"dependencies"`
	TaskCount    int      `json:"task_count"`
	Edited       bool     `json:"edited"` // User has modified
}

// Store manages draft persistence.
type Store struct {
	basePath string
}

// NewStore creates a new draft store.
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	basePath := filepath.Join(home, ".autarch", "drafts")
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, err
	}

	return &Store{basePath: basePath}, nil
}

// NewStoreWithPath creates a store with a custom base path (for testing).
func NewStoreWithPath(basePath string) *Store {
	return &Store{basePath: basePath}
}

// Save persists a draft to disk.
func (s *Store) Save(draft *Draft) error {
	draft.UpdatedAt = time.Now()

	draftDir := filepath.Join(s.basePath, draft.ProjectID)
	if err := os.MkdirAll(draftDir, 0755); err != nil {
		return err
	}

	// Save interview state
	if draft.Interview != nil {
		if err := s.saveJSON(filepath.Join(draftDir, "interview.json"), draft.Interview); err != nil {
			return err
		}
	}

	// Save research state
	if draft.Research != nil {
		if err := s.saveJSON(filepath.Join(draftDir, "research.json"), draft.Research); err != nil {
			return err
		}
	}

	// Save epics state
	if draft.Epics != nil {
		if err := s.saveJSON(filepath.Join(draftDir, "epics.json"), draft.Epics); err != nil {
			return err
		}
	}

	// Save main draft metadata
	return s.saveJSON(filepath.Join(draftDir, "draft.json"), draft)
}

// Load reads a draft from disk.
func (s *Store) Load(projectID string) (*Draft, error) {
	draftDir := filepath.Join(s.basePath, projectID)

	// Load main draft metadata
	draft := &Draft{}
	draftPath := filepath.Join(draftDir, "draft.json")
	if err := s.loadJSON(draftPath, draft); err != nil {
		return nil, err
	}

	// Load interview state
	interview := &InterviewState{}
	interviewPath := filepath.Join(draftDir, "interview.json")
	if err := s.loadJSON(interviewPath, interview); err == nil {
		draft.Interview = interview
	}

	// Load research state
	research := &ResearchState{}
	researchPath := filepath.Join(draftDir, "research.json")
	if err := s.loadJSON(researchPath, research); err == nil {
		draft.Research = research
	}

	// Load epics state
	epics := &EpicsState{}
	epicsPath := filepath.Join(draftDir, "epics.json")
	if err := s.loadJSON(epicsPath, epics); err == nil {
		draft.Epics = epics
	}

	return draft, nil
}

// Exists checks if a draft exists for a project.
func (s *Store) Exists(projectID string) bool {
	draftPath := filepath.Join(s.basePath, projectID, "draft.json")
	_, err := os.Stat(draftPath)
	return err == nil
}

// Delete removes a draft from disk.
func (s *Store) Delete(projectID string) error {
	draftDir := filepath.Join(s.basePath, projectID)
	return os.RemoveAll(draftDir)
}

// List returns all draft project IDs.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.basePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			draftPath := filepath.Join(s.basePath, entry.Name(), "draft.json")
			if _, err := os.Stat(draftPath); err == nil {
				ids = append(ids, entry.Name())
			}
		}
	}
	return ids, nil
}

// ListDrafts returns all drafts with metadata.
func (s *Store) ListDrafts() ([]*Draft, error) {
	ids, err := s.List()
	if err != nil {
		return nil, err
	}

	var drafts []*Draft
	for _, id := range ids {
		draft, err := s.Load(id)
		if err != nil {
			continue // Skip corrupted drafts
		}
		drafts = append(drafts, draft)
	}
	return drafts, nil
}

func (s *Store) saveJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (s *Store) loadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// NewDraft creates a new draft for a project.
func NewDraft(projectID, projectName, description string) *Draft {
	now := time.Now()
	return &Draft{
		ProjectID:   projectID,
		ProjectName: projectName,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// UpdateInterview updates the interview state.
func (d *Draft) UpdateInterview(step int, totalSteps int, answers map[string]any) {
	if d.Interview == nil {
		d.Interview = &InterviewState{
			Answers: make(map[string]any),
		}
	}
	d.Interview.CurrentStep = step
	d.Interview.TotalSteps = totalSteps
	for k, v := range answers {
		d.Interview.Answers[k] = v
	}
}

// SetQuestionTouched marks a question as touched by user interaction.
func (d *Draft) SetQuestionTouched(questionID string) {
	if d.Interview == nil {
		return
	}
	for i, q := range d.Interview.Questions {
		if q.QuestionID == questionID {
			d.Interview.Questions[i].Touched = true
			return
		}
	}
}

// IsQuestionTouched returns true if the user has interacted with the question.
func (d *Draft) IsQuestionTouched(questionID string) bool {
	if d.Interview == nil {
		return false
	}
	for _, q := range d.Interview.Questions {
		if q.QuestionID == questionID {
			return q.Touched
		}
	}
	return false
}

// CanApplyDefault returns true if a research default can be applied.
// Defaults are only applied once, and only if the user hasn't interacted.
func (d *Draft) CanApplyDefault(questionID string) bool {
	if d.Interview == nil {
		return true // No state, defaults allowed
	}
	for _, q := range d.Interview.Questions {
		if q.QuestionID == questionID {
			return !q.Touched && !q.DefaultApplied
		}
	}
	return true // Question not tracked yet, defaults allowed
}

// MarkDefaultApplied records that a research default was applied.
func (d *Draft) MarkDefaultApplied(questionID string) {
	if d.Interview == nil {
		d.Interview = &InterviewState{}
	}
	for i, q := range d.Interview.Questions {
		if q.QuestionID == questionID {
			d.Interview.Questions[i].DefaultApplied = true
			return
		}
	}
	// Add new question state
	d.Interview.Questions = append(d.Interview.Questions, QuestionState{
		QuestionID:     questionID,
		DefaultApplied: true,
	})
}
