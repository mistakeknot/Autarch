package signals

import "time"

// AgentState represents an ephemeral lifecycle state reported by a dispatched agent
// via bracket signals in its output: --<[autarch:state:message]>--
type AgentState string

const (
	AgentWorking      AgentState = "working"
	AgentNeedsInput   AgentState = "needs_input"
	AgentNeedsTesting AgentState = "needs_testing"
	AgentCompleted    AgentState = "completed"
	AgentError        AgentState = "error"
)

var validAgentStates = map[AgentState]bool{
	AgentWorking:      true,
	AgentNeedsInput:   true,
	AgentNeedsTesting: true,
	AgentCompleted:    true,
	AgentError:        true,
}

// IsValidAgentState reports whether s is a recognized agent state.
func IsValidAgentState(s AgentState) bool {
	return validAgentStates[s]
}

// AgentSignal is a parsed bracket signal from agent output.
type AgentSignal struct {
	State     AgentState `json:"state"`
	Message   string     `json:"message"`
	Timestamp time.Time  `json:"timestamp"`
}
