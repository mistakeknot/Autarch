package icdata

// Run represents an Intercore run from `ic run list --json`.
type Run struct {
	ID         string   `json:"id"`
	Goal       string   `json:"goal"`
	Phase      string   `json:"phase"`
	Phases     []string `json:"phases"`
	Status     string   `json:"status"`
	ScopeID    string   `json:"scope_id"`
	Complexity int      `json:"complexity"`
	CreatedAt  int64    `json:"created_at"`
	UpdatedAt  int64    `json:"updated_at"`
	ProjectDir string   `json:"project_dir"`
}

// Dispatch represents an Intercore dispatch from `ic dispatch list --json`.
type Dispatch struct {
	ID          string  `json:"id"`
	AgentType   string  `json:"agent_type"`
	Status      string  `json:"status"`
	Name        *string `json:"name"`
	Model       *string `json:"model"`
	InTokens    int     `json:"in_tokens"`
	OutTokens   int     `json:"out_tokens"`
	CreatedAt   int64   `json:"created_at"`
	StartedAt   *int64  `json:"started_at"`
	CompletedAt *int64  `json:"completed_at"`
	ScopeID     *string `json:"scope_id"`
	ProjectDir  string  `json:"project_dir"`
}

// DisplayName returns the dispatch name, falling back to agent type.
func (d Dispatch) DisplayName() string {
	if d.Name != nil && *d.Name != "" {
		return *d.Name
	}
	return d.AgentType
}

// DisplayModel returns the model name or empty string.
func (d Dispatch) DisplayModel() string {
	if d.Model != nil {
		return *d.Model
	}
	return ""
}

// Event represents an Intercore event from `ic events tail`.
type Event struct {
	ID        int64  `json:"id"`
	RunID     string `json:"run_id"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

// Lane represents a thematic work lane from `ic lane list --json`.
type Lane struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	LaneType    string `json:"lane_type"`
	Status      string `json:"status"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// LaneVelocity represents starvation data from `ic lane velocity --json`.
type LaneVelocity struct {
	LaneID     string  `json:"lane_id"`
	Name       string  `json:"name"`
	OpenBeads  int     `json:"open_beads"`
	Closed     int     `json:"closed"`
	Throughput float64 `json:"throughput"`
	Starvation float64 `json:"starvation"`
}

// TokenSummary represents token usage from `ic run tokens --json`.
type TokenSummary struct {
	RunID        string `json:"run_id"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	CacheHits    int64  `json:"cache_hits"`
}
