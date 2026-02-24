package tui

// Phase represents the current palette interaction phase.
type Phase int

const (
	PhaseCommand Phase = iota // Fuzzy command search (existing behavior)
	PhaseTarget               // Select broadcast target (All/Claude/Codex/Gemini)
	PhaseConfirm              // Confirm before executing
)

func (p Phase) String() string {
	switch p {
	case PhaseCommand:
		return "command"
	case PhaseTarget:
		return "target"
	case PhaseConfirm:
		return "confirm"
	default:
		return "unknown"
	}
}

// Target represents the broadcast target group.
type Target int

const (
	TargetAll    Target = iota // All agent panes
	TargetClaude               // Claude panes only
	TargetCodex                // Codex panes only
	TargetGemini               // Gemini panes only
)

func (t Target) Label() string {
	switch t {
	case TargetAll:
		return "All agents"
	case TargetClaude:
		return "Claude"
	case TargetCodex:
		return "Codex"
	case TargetGemini:
		return "Gemini"
	default:
		return "Unknown"
	}
}

// PaneCounts holds live counts of agent panes by type.
type PaneCounts struct {
	Claude int
	Codex  int
	Gemini int
}

// Total returns the total number of agent panes.
func (pc PaneCounts) Total() int {
	return pc.Claude + pc.Codex + pc.Gemini
}

// ForTarget returns the pane count for a specific target.
func (pc PaneCounts) ForTarget(t Target) int {
	switch t {
	case TargetAll:
		return pc.Total()
	case TargetClaude:
		return pc.Claude
	case TargetCodex:
		return pc.Codex
	case TargetGemini:
		return pc.Gemini
	default:
		return 0
	}
}

// BroadcastAction holds the resolved broadcast context passed to the action.
type BroadcastAction struct {
	Target     Target
	PaneCounts PaneCounts
}
