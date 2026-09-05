// Package review defines the versioned local source records shared by the TUI,
// background controller, native capture companion and lattice projection.
package review

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

const Version = 1

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

type Source struct {
	ID       string `json:"id"`
	Path     string `json:"path,omitempty"`
	Revision string `json:"revision,omitempty"`
	Status   string `json:"status"` // available, unavailable, deleted
	OffsetMS int64  `json:"offset_ms,omitempty"`
	Kind     string `json:"kind,omitempty"`
}
type UIContext struct {
	At      time.Time `json:"at"`
	View    string    `json:"view"`
	Project string    `json:"project"`
	Item    string    `json:"item,omitempty"`
	Density string    `json:"density"`
	Build   string    `json:"build"`
}
type Session struct {
	ID          string    `json:"id"`
	Project     string    `json:"project"`
	Revision    int       `json:"revision"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	WindowID    uint32    `json:"window_id"`
	WindowTitle string    `json:"window_title"`
	Status      string    `json:"status"`
	Media       []Source  `json:"media"`
	Error       string    `json:"error,omitempty"`
}
type Feedback struct {
	ID                 string    `json:"id"`
	Project            string    `json:"project"`
	SuggestedProject   string    `json:"suggested_project,omitempty"`
	SessionID          string    `json:"session_id,omitempty"`
	Revision           int       `json:"revision"`
	At                 time.Time `json:"at"`
	Text               string    `json:"text"`
	OriginalText       string    `json:"original_text"`
	Evidence           []Source  `json:"evidence"`
	Context            UIContext `json:"context"`
	Analysis           string    `json:"analysis"`
	TranscriptionError string    `json:"transcription_error,omitempty"`
}
type Guidance struct {
	Path         string `json:"path"`
	Text         string `json:"text"`
	Scope        string `json:"scope"`
	Rationale    string `json:"rationale"`
	BaseRevision string `json:"base_revision"` // SHA256 of the displayed file
	Supersedes   string `json:"supersedes,omitempty"`
}
type Proposal struct {
	ID            string     `json:"id"`
	Project       string     `json:"project"`
	Revision      int        `json:"revision"`
	At            time.Time  `json:"at"`
	FeedbackIDs   []string   `json:"feedback_ids"`
	Outcome       string     `json:"outcome"`
	Change        string     `json:"change"`
	Scope         []string   `json:"scope"`
	Rationale     string     `json:"rationale"`
	Evidence      []Source   `json:"evidence"`
	Uncertainties []string   `json:"uncertainties"`
	Pushback      string     `json:"pushback,omitempty"`
	Guidance      []Guidance `json:"guidance"`
	Checklist     []string   `json:"checklist"`
	Priority      int        `json:"priority"`
	Dependencies  []string   `json:"dependencies"`
	BudgetTokens  int        `json:"budget_tokens"`
	Status        string     `json:"status"`
	AcceptedAt    *time.Time `json:"accepted_at,omitempty"`
}
type Execution struct {
	ID               string    `json:"id"`
	Project          string    `json:"project"`
	ProposalID       string    `json:"proposal_id"`
	ProposalRevision int       `json:"proposal_revision"`
	Status           string    `json:"status"`
	WorkID           string    `json:"work_id,omitempty"`
	RunID            string    `json:"run_id,omitempty"`
	DispatchID       string    `json:"dispatch_id,omitempty"`
	Build            string    `json:"build,omitempty"`
	Binary           string    `json:"binary,omitempty"`
	Model            string    `json:"model,omitempty"`
	Reason           string    `json:"reason,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type Verdict struct {
	ID          string    `json:"id"`
	Project     string    `json:"project"`
	ExecutionID string    `json:"execution_id"`
	Build       string    `json:"build"`
	Verdict     string    `json:"verdict"`
	Notes       string    `json:"notes"`
	At          time.Time `json:"at"`
}
type Turn struct {
	ID             string    `json:"id"`
	Project        string    `json:"project"`
	RuntimeSession string    `json:"runtime_session,omitempty"`
	Model          string    `json:"model,omitempty"`
	Kind           string    `json:"kind"`
	Text           string    `json:"text"`
	At             time.Time `json:"at"`
}
type Question struct {
	ID             string   `json:"id"`
	Project        string   `json:"project"`
	RuntimeSession string   `json:"runtime_session"`
	Method         string   `json:"method"`
	Title          string   `json:"title"`
	Options        []string `json:"options"`
	Status         string   `json:"status"`
	Answer         string   `json:"answer,omitempty"`
	Consequential  bool     `json:"consequential"`
	BlocksActive   bool     `json:"blocks_active"`
}
type CaptureCommand struct {
	ID      string  `json:"id"`
	Project string  `json:"project"`
	Method  string  `json:"method"`
	Target  string  `json:"target,omitempty"`
	Source  *Source `json:"source,omitempty"`
	Status  string  `json:"status"`
}
type Receipt struct {
	Hash string `json:"hash"`
	ID   string `json:"id"`
}
type State struct {
	Version    int                  `json:"version"`
	Revision   uint64               `json:"revision"`
	Sessions   map[string]Session   `json:"sessions"`
	Feedback   map[string]Feedback  `json:"feedback"`
	Proposals  map[string]Proposal  `json:"proposals"`
	Executions map[string]Execution `json:"executions"`
	Verdicts   map[string]Verdict   `json:"verdicts"`
	Turns      []Turn               `json:"turns"`
	Questions  []Question           `json:"questions"`
	Commands   []CaptureCommand     `json:"commands"`
	Context    UIContext            `json:"context"`
	Receipts   map[string]Receipt   `json:"receipts"`
}

func (s State) Clone() State {
	b, _ := json.Marshal(s)
	var c State
	_ = json.Unmarshal(b, &c)
	return c
}

// Request IDs are retry keys. A key may only be reused with identical content.
type Request struct {
	Version   int        `json:"version"`
	ID        string     `json:"id"`
	Method    string     `json:"method"`
	Project   string     `json:"project,omitempty"`
	Target    string     `json:"target,omitempty"`
	Revision  int        `json:"revision,omitempty"`
	Text      string     `json:"text,omitempty"`
	Status    string     `json:"status,omitempty"`
	Session   *Session   `json:"session,omitempty"`
	Feedback  *Feedback  `json:"feedback,omitempty"`
	Proposal  *Proposal  `json:"proposal,omitempty"`
	Execution *Execution `json:"execution,omitempty"`
	Verdict   *Verdict   `json:"verdict,omitempty"`
	Context   *UIContext `json:"context,omitempty"`
	Source    *Source    `json:"source,omitempty"`
	Turn      *Turn      `json:"turn,omitempty"`
	Question  *Question  `json:"question,omitempty"`
}
type Response struct {
	Version      int    `json:"version"`
	ID           string `json:"id,omitempty"`
	Error        string `json:"error,omitempty"`
	State        *State `json:"state,omitempty"`
	StorageBytes int64  `json:"storage_bytes,omitempty"`
	DataDir      string `json:"data_dir,omitempty"`
}
