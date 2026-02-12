package signals

import (
	"regexp"
	"time"
)

// bracketRe matches --<[autarch:state:message]>-- on its own line.
// Tolerates leading bullets (⏺ • - *) and whitespace, trailing whitespace, \r.
// Groups: 1=state, 2=message.
var bracketRe = regexp.MustCompile(`(?m)^[⏺•\-\*\s]*--<\[autarch:(\w+):([^\]]*)\]>--[ \t]*\r*$`)

// ParseAgentSignals extracts all valid agent signals from terminal output data.
// It strips ANSI escape sequences before matching bracket patterns.
func ParseAgentSignals(data []byte) []AgentSignal {
	return parseAgentSignalsAt(data, time.Now())
}

// ParseAgentSignalsAt is like ParseAgentSignals but uses the given timestamp.
func ParseAgentSignalsAt(data []byte, t time.Time) []AgentSignal {
	return parseAgentSignalsAt(data, t)
}

func parseAgentSignalsAt(data []byte, t time.Time) []AgentSignal {
	clean := stripANSIBytes(data)
	matches := bracketRe.FindAllSubmatch(clean, -1)
	if len(matches) == 0 {
		return nil
	}

	var out []AgentSignal
	for _, m := range matches {
		state := AgentState(m[1])
		if !IsValidAgentState(state) {
			continue
		}
		out = append(out, AgentSignal{
			State:   state,
			Message: string(m[2]),
			Timestamp: t,
		})
	}
	return out
}
