package intermute

import (
	"time"

	ic "github.com/mistakeknot/intermute/client"
)

// SpecStatus represents the status of a specification
type SpecStatus string

const (
	SpecStatusDraft     SpecStatus = "draft"
	SpecStatusResearch  SpecStatus = "research"
	SpecStatusValidated SpecStatus = "validated"
	SpecStatusArchived  SpecStatus = "archived"
)

// EpicStatus represents the status of an epic
type EpicStatus string

const (
	EpicStatusOpen       EpicStatus = "open"
	EpicStatusInProgress EpicStatus = "in_progress"
	EpicStatusDone       EpicStatus = "done"
)

// StoryStatus represents the status of a story
type StoryStatus string

const (
	StoryStatusTodo       StoryStatus = "todo"
	StoryStatusInProgress StoryStatus = "in_progress"
	StoryStatusReview     StoryStatus = "review"
	StoryStatusDone       StoryStatus = "done"
)

// TaskStatus represents the status of a task
type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusBlocked TaskStatus = "blocked"
	TaskStatusDone    TaskStatus = "done"
)

// SessionStatus represents the status of an agent session
type SessionStatus string

const (
	SessionStatusRunning SessionStatus = "running"
	SessionStatusIdle    SessionStatus = "idle"
	SessionStatusError   SessionStatus = "error"
)

// CUJStatus represents the status of a Critical User Journey
type CUJStatus string

const (
	CUJStatusDraft     CUJStatus = "draft"
	CUJStatusValidated CUJStatus = "validated"
	CUJStatusArchived  CUJStatus = "archived"
)

// CUJPriority represents the priority level of a CUJ
type CUJPriority string

const (
	CUJPriorityHigh   CUJPriority = "high"
	CUJPriorityMedium CUJPriority = "medium"
	CUJPriorityLow    CUJPriority = "low"
)

// Spec represents a product specification (PRD)
type Spec struct {
	ID        string     `json:"id" yaml:"id"`
	Project   string     `json:"project" yaml:"project"`
	Title     string     `json:"title" yaml:"title"`
	Vision    string     `json:"vision,omitempty" yaml:"vision,omitempty"`
	Users     string     `json:"users,omitempty" yaml:"users,omitempty"`
	Problem   string     `json:"problem,omitempty" yaml:"problem,omitempty"`
	Status    SpecStatus `json:"status" yaml:"status"`
	Version   int64      `json:"version,omitempty" yaml:"version,omitempty"`
	CreatedAt time.Time  `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" yaml:"updated_at"`
}

// Epic represents a large feature or initiative
type Epic struct {
	ID          string     `json:"id" yaml:"id"`
	Project     string     `json:"project" yaml:"project"`
	SpecID      string     `json:"spec_id,omitempty" yaml:"spec_id,omitempty"`
	Title       string     `json:"title" yaml:"title"`
	Description string     `json:"description,omitempty" yaml:"description,omitempty"`
	Status      EpicStatus `json:"status" yaml:"status"`
	Version     int64      `json:"version,omitempty" yaml:"version,omitempty"`
	CreatedAt   time.Time  `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" yaml:"updated_at"`
}

// Story represents a user story within an epic
type Story struct {
	ID                 string      `json:"id" yaml:"id"`
	Project            string      `json:"project" yaml:"project"`
	EpicID             string      `json:"epic_id" yaml:"epic_id"`
	Title              string      `json:"title" yaml:"title"`
	AcceptanceCriteria []string    `json:"acceptance_criteria,omitempty" yaml:"acceptance_criteria,omitempty"`
	Status             StoryStatus `json:"status" yaml:"status"`
	Version            int64       `json:"version,omitempty" yaml:"version,omitempty"`
	CreatedAt          time.Time   `json:"created_at" yaml:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at" yaml:"updated_at"`
}

// Task represents an execution unit assigned to an agent
type Task struct {
	ID        string     `json:"id" yaml:"id"`
	Project   string     `json:"project" yaml:"project"`
	StoryID   string     `json:"story_id,omitempty" yaml:"story_id,omitempty"`
	Title     string     `json:"title" yaml:"title"`
	Agent     string     `json:"agent,omitempty" yaml:"agent,omitempty"`
	SessionID string     `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	Status    TaskStatus `json:"status" yaml:"status"`
	Version   int64      `json:"version,omitempty" yaml:"version,omitempty"`
	CreatedAt time.Time  `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" yaml:"updated_at"`
}

// Insight represents a research insight from Pollard
type Insight struct {
	ID        string    `json:"id" yaml:"id"`
	Project   string    `json:"project" yaml:"project"`
	SpecID    string    `json:"spec_id,omitempty" yaml:"spec_id,omitempty"`
	Source    string    `json:"source" yaml:"source"`
	Category  string    `json:"category" yaml:"category"`
	Title     string    `json:"title" yaml:"title"`
	Body      string    `json:"body,omitempty" yaml:"body,omitempty"`
	URL       string    `json:"url,omitempty" yaml:"url,omitempty"`
	Score     float64   `json:"score" yaml:"score"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

// Session represents an agent session (tmux session)
type Session struct {
	ID        string        `json:"id" yaml:"id"`
	Project   string        `json:"project" yaml:"project"`
	Name      string        `json:"name" yaml:"name"`
	Agent     string        `json:"agent" yaml:"agent"`
	TaskID    string        `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	Status    SessionStatus `json:"status" yaml:"status"`
	StartedAt time.Time     `json:"started_at" yaml:"started_at"`
	UpdatedAt time.Time     `json:"updated_at" yaml:"updated_at"`
}

// CriticalUserJourney represents a first-class CUJ entity
type CriticalUserJourney struct {
	ID              string      `json:"id" yaml:"id"`
	SpecID          string      `json:"spec_id" yaml:"spec_id"`
	Project         string      `json:"project" yaml:"project"`
	Title           string      `json:"title" yaml:"title"`
	Persona         string      `json:"persona" yaml:"persona"`
	Priority        CUJPriority `json:"priority" yaml:"priority"`
	EntryPoint      string      `json:"entry_point" yaml:"entry_point"`
	ExitPoint       string      `json:"exit_point" yaml:"exit_point"`
	Steps           []CUJStep   `json:"steps" yaml:"steps"`
	SuccessCriteria []string    `json:"success_criteria" yaml:"success_criteria"`
	ErrorRecovery   []string    `json:"error_recovery" yaml:"error_recovery"`
	Status          CUJStatus   `json:"status" yaml:"status"`
	Version         int64       `json:"version,omitempty" yaml:"version,omitempty"`
	CreatedAt       time.Time   `json:"created_at" yaml:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at" yaml:"updated_at"`
}

// CUJStep represents a single step in a Critical User Journey
type CUJStep struct {
	Order        int      `json:"order" yaml:"order"`
	Action       string   `json:"action" yaml:"action"`
	Expected     string   `json:"expected" yaml:"expected"`
	Alternatives []string `json:"alternatives,omitempty" yaml:"alternatives,omitempty"`
}

// AcceptanceCriterion represents a Gherkin-style acceptance criterion
type AcceptanceCriterion struct {
	ID          string   `json:"id" yaml:"id"`
	StoryID     string   `json:"story_id" yaml:"story_id"`
	Given       string   `json:"given" yaml:"given"`
	When        string   `json:"when" yaml:"when"`
	Then        string   `json:"then" yaml:"then"`
	EdgeCases   []string `json:"edge_cases,omitempty" yaml:"edge_cases,omitempty"`
	TestCommand string   `json:"test_command,omitempty" yaml:"test_command,omitempty"`
}

// --- Conversion functions between Autarch types and Intermute client types ---

func toIntermuteSpec(s Spec) ic.Spec {
	return ic.Spec{
		ID:        s.ID,
		Project:   s.Project,
		Title:     s.Title,
		Vision:    s.Vision,
		Users:     s.Users,
		Problem:   s.Problem,
		Status:    ic.SpecStatus(s.Status),
		Version:   s.Version,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func fromIntermuteSpec(s ic.Spec) Spec {
	return Spec{
		ID:        s.ID,
		Project:   s.Project,
		Title:     s.Title,
		Vision:    s.Vision,
		Users:     s.Users,
		Problem:   s.Problem,
		Status:    SpecStatus(s.Status),
		Version:   s.Version,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func toIntermuteEpic(e Epic) ic.Epic {
	return ic.Epic{
		ID:          e.ID,
		Project:     e.Project,
		SpecID:      e.SpecID,
		Title:       e.Title,
		Description: e.Description,
		Status:      ic.EpicStatus(e.Status),
		Version:     e.Version,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

func fromIntermuteEpic(e ic.Epic) Epic {
	return Epic{
		ID:          e.ID,
		Project:     e.Project,
		SpecID:      e.SpecID,
		Title:       e.Title,
		Description: e.Description,
		Status:      EpicStatus(e.Status),
		Version:     e.Version,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

func toIntermuteStory(s Story) ic.Story {
	return ic.Story{
		ID:                 s.ID,
		Project:            s.Project,
		EpicID:             s.EpicID,
		Title:              s.Title,
		AcceptanceCriteria: s.AcceptanceCriteria,
		Status:             ic.StoryStatus(s.Status),
		Version:            s.Version,
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
	}
}

func fromIntermuteStory(s ic.Story) Story {
	return Story{
		ID:                 s.ID,
		Project:            s.Project,
		EpicID:             s.EpicID,
		Title:              s.Title,
		AcceptanceCriteria: s.AcceptanceCriteria,
		Status:             StoryStatus(s.Status),
		Version:            s.Version,
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
	}
}

func toIntermuteTask(t Task) ic.Task {
	return ic.Task{
		ID:        t.ID,
		Project:   t.Project,
		StoryID:   t.StoryID,
		Title:     t.Title,
		Agent:     t.Agent,
		SessionID: t.SessionID,
		Status:    ic.TaskStatus(t.Status),
		Version:   t.Version,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

func fromIntermuteTask(t ic.Task) Task {
	return Task{
		ID:        t.ID,
		Project:   t.Project,
		StoryID:   t.StoryID,
		Title:     t.Title,
		Agent:     t.Agent,
		SessionID: t.SessionID,
		Status:    TaskStatus(t.Status),
		Version:   t.Version,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

func toIntermuteInsight(i Insight) ic.Insight {
	return ic.Insight{
		ID:        i.ID,
		Project:   i.Project,
		SpecID:    i.SpecID,
		Source:    i.Source,
		Category:  i.Category,
		Title:     i.Title,
		Body:      i.Body,
		URL:       i.URL,
		Score:     i.Score,
		CreatedAt: i.CreatedAt,
	}
}

func fromIntermuteInsight(i ic.Insight) Insight {
	return Insight{
		ID:        i.ID,
		Project:   i.Project,
		SpecID:    i.SpecID,
		Source:    i.Source,
		Category:  i.Category,
		Title:     i.Title,
		Body:      i.Body,
		URL:       i.URL,
		Score:     i.Score,
		CreatedAt: i.CreatedAt,
	}
}

func toIntermuteSession(s Session) ic.Session {
	return ic.Session{
		ID:        s.ID,
		Project:   s.Project,
		Name:      s.Name,
		Agent:     s.Agent,
		TaskID:    s.TaskID,
		Status:    ic.SessionStatus(s.Status),
		StartedAt: s.StartedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func fromIntermuteSession(s ic.Session) Session {
	return Session{
		ID:        s.ID,
		Project:   s.Project,
		Name:      s.Name,
		Agent:     s.Agent,
		TaskID:    s.TaskID,
		Status:    SessionStatus(s.Status),
		StartedAt: s.StartedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func toIntermuteCUJ(c CriticalUserJourney) ic.CriticalUserJourney {
	steps := make([]ic.CUJStep, len(c.Steps))
	for i, s := range c.Steps {
		steps[i] = ic.CUJStep{
			Order:        s.Order,
			Action:       s.Action,
			Expected:     s.Expected,
			Alternatives: s.Alternatives,
		}
	}
	return ic.CriticalUserJourney{
		ID:              c.ID,
		SpecID:          c.SpecID,
		Project:         c.Project,
		Title:           c.Title,
		Persona:         c.Persona,
		Priority:        ic.CUJPriority(c.Priority),
		EntryPoint:      c.EntryPoint,
		ExitPoint:       c.ExitPoint,
		Steps:           steps,
		SuccessCriteria: c.SuccessCriteria,
		ErrorRecovery:   c.ErrorRecovery,
		Status:          ic.CUJStatus(c.Status),
		Version:         c.Version,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}

func fromIntermuteCUJ(c ic.CriticalUserJourney) CriticalUserJourney {
	steps := make([]CUJStep, len(c.Steps))
	for i, s := range c.Steps {
		steps[i] = CUJStep{
			Order:        s.Order,
			Action:       s.Action,
			Expected:     s.Expected,
			Alternatives: s.Alternatives,
		}
	}
	return CriticalUserJourney{
		ID:              c.ID,
		SpecID:          c.SpecID,
		Project:         c.Project,
		Title:           c.Title,
		Persona:         c.Persona,
		Priority:        CUJPriority(c.Priority),
		EntryPoint:      c.EntryPoint,
		ExitPoint:       c.ExitPoint,
		Steps:           steps,
		SuccessCriteria: c.SuccessCriteria,
		ErrorRecovery:   c.ErrorRecovery,
		Status:          CUJStatus(c.Status),
		Version:         c.Version,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}

// CUJFeatureLink represents a link between a CUJ and a feature
type CUJFeatureLink struct {
	CUJID     string    `json:"cuj_id" yaml:"cuj_id"`
	FeatureID string    `json:"feature_id" yaml:"feature_id"`
	Project   string    `json:"project" yaml:"project"`
	LinkedAt  time.Time `json:"linked_at" yaml:"linked_at"`
}

// --- Agent Mail Migration Types ---
// These types support the migration from MCP Agent Mail to Intermute

// Agent represents a registered agent with inbox statistics
type Agent struct {
	ID           string            `json:"agent_id"`
	SessionID    string            `json:"session_id,omitempty"`
	Name         string            `json:"name"`
	Project      string            `json:"project"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Status       string            `json:"status,omitempty"`
	LastSeen     time.Time         `json:"last_seen,omitempty"`
	CreatedAt    time.Time         `json:"created_at,omitempty"`
	// Enriched fields (not from base Intermute client)
	InboxCount  int `json:"inbox_count,omitempty"`
	UnreadCount int `json:"unread_count,omitempty"`
}

// Message represents a message with all metadata fields
type Message struct {
	ID          string    `json:"id"`
	ThreadID    string    `json:"thread_id,omitempty"`
	Project     string    `json:"project"`
	From        string    `json:"from"`
	To          []string  `json:"to"`
	CC          []string  `json:"cc,omitempty"`
	BCC         []string  `json:"bcc,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	Body        string    `json:"body"`
	Importance  string    `json:"importance,omitempty"`
	AckRequired bool      `json:"ack_required,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Cursor      uint64    `json:"cursor,omitempty"`
}

// Reservation represents a file lock held by an agent
type Reservation struct {
	ID          string     `json:"id"`
	AgentID     string     `json:"agent_id"`
	Project     string     `json:"project"`
	PathPattern string     `json:"path_pattern"`
	Exclusive   bool       `json:"exclusive"`
	Reason      string     `json:"reason,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	ReleasedAt  *time.Time `json:"released_at,omitempty"`
	// Computed/enriched fields
	AgentName string `json:"agent_name,omitempty"`
	IsActive  bool   `json:"is_active,omitempty"`
}

// InboxCounts represents inbox statistics
type InboxCounts struct {
	Total  int `json:"total"`
	Unread int `json:"unread"`
}

// --- Conversion functions for agent mail types ---

func fromIntermuteAgent(a ic.Agent) Agent {
	var lastSeen, createdAt time.Time
	if a.LastSeen != "" {
		lastSeen, _ = time.Parse(time.RFC3339, a.LastSeen)
	}
	if a.CreatedAt != "" {
		createdAt, _ = time.Parse(time.RFC3339, a.CreatedAt)
	}
	return Agent{
		ID:           a.ID,
		SessionID:    a.SessionID,
		Name:         a.Name,
		Project:      a.Project,
		Capabilities: a.Capabilities,
		Metadata:     a.Metadata,
		Status:       a.Status,
		LastSeen:     lastSeen,
		CreatedAt:    createdAt,
	}
}

func fromIntermuteMessage(m ic.Message) Message {
	var createdAt time.Time
	if m.CreatedAt != "" {
		createdAt, _ = time.Parse(time.RFC3339Nano, m.CreatedAt)
	}
	return Message{
		ID:          m.ID,
		ThreadID:    m.ThreadID,
		Project:     m.Project,
		From:        m.From,
		To:          m.To,
		CC:          m.CC,
		BCC:         m.BCC,
		Subject:     m.Subject,
		Body:        m.Body,
		Importance:  m.Importance,
		AckRequired: m.AckRequired,
		CreatedAt:   createdAt,
		Cursor:      m.Cursor,
	}
}

func fromIntermuteReservation(r ic.Reservation) Reservation {
	var createdAt, expiresAt time.Time
	var releasedAt *time.Time
	if r.CreatedAt != "" {
		createdAt, _ = time.Parse(time.RFC3339, r.CreatedAt)
	}
	if r.ExpiresAt != "" {
		expiresAt, _ = time.Parse(time.RFC3339, r.ExpiresAt)
	}
	if r.ReleasedAt != nil && *r.ReleasedAt != "" {
		t, _ := time.Parse(time.RFC3339, *r.ReleasedAt)
		releasedAt = &t
	}
	return Reservation{
		ID:          r.ID,
		AgentID:     r.AgentID,
		Project:     r.Project,
		PathPattern: r.PathPattern,
		Exclusive:   r.Exclusive,
		Reason:      r.Reason,
		CreatedAt:   createdAt,
		ExpiresAt:   expiresAt,
		ReleasedAt:  releasedAt,
		IsActive:    r.IsActive,
	}
}

func toIntermuteReservation(r Reservation, ttlMinutes int) ic.Reservation {
	return ic.Reservation{
		ID:          r.ID,
		AgentID:     r.AgentID,
		Project:     r.Project,
		PathPattern: r.PathPattern,
		Exclusive:   r.Exclusive,
		Reason:      r.Reason,
		TTLMinutes:  ttlMinutes,
	}
}
