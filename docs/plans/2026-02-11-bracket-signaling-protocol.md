# Plan: Bracket-Based Agent Signaling Protocol

> **Bead:** Autarch-0wp (P2)
> **Brainstorm:** docs/research/brainstorm-bracket-signaling.md
> **Source reference:** schmux `internal/signal/signal.go`

## Goal

Add a parser that extracts structured agent state signals from terminal output. Format: `--<[autarch:state:message]>--`. Pure parser — no integration wiring in this change.

## Tasks

### Task 1: Agent signal types (`pkg/signals/agent_signal.go`)

Create the `AgentState` enum and `AgentSignal` struct.

```go
package signals

import "time"

// AgentState represents an ephemeral lifecycle state reported by a dispatched agent.
type AgentState string

const (
	AgentWorking      AgentState = "working"
	AgentNeedsInput   AgentState = "needs_input"
	AgentNeedsTesting AgentState = "needs_testing"
	AgentCompleted    AgentState = "completed"
	AgentError        AgentState = "error"
)

// validAgentStates is the set of recognized agent states.
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
```

**Acceptance:** Types compile, `IsValidAgentState` works for all 5 states + rejects invalid.

---

### Task 2: ANSI stripping (`pkg/signals/ansi.go`)

Port schmux's 4-layer ANSI stripping pipeline.

```go
package signals

import "regexp"

var (
	cursorForwardRe = regexp.MustCompile(`\x1b\[\d*C`)
	cursorDownRe    = regexp.MustCompile(`\x1b\[\d*B`)
	oscSeqRe        = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	ansiCSIRe       = regexp.MustCompile(`\x1b\[\??[0-9;]*[A-Za-z]`)
)

// StripANSI removes ANSI escape sequences from s, replacing cursor
// movement with spaces/newlines to preserve word and line boundaries.
func StripANSI(s string) string {
	s = cursorForwardRe.ReplaceAllString(s, " ")
	s = cursorDownRe.ReplaceAllString(s, "\n")
	s = oscSeqRe.ReplaceAllString(s, "")
	return ansiCSIRe.ReplaceAllString(s, "")
}

// stripANSIBytes is the []byte variant for parser internals.
func stripANSIBytes(data []byte) []byte {
	data = cursorForwardRe.ReplaceAll(data, []byte(" "))
	data = cursorDownRe.ReplaceAll(data, []byte("\n"))
	data = oscSeqRe.ReplaceAll(data, nil)
	return ansiCSIRe.ReplaceAll(data, nil)
}
```

**Acceptance:** `StripANSI` handles cursor-forward→space, cursor-down→newline, OSC removal, CSI removal. Exported for reuse.

---

### Task 3: Bracket signal parser (`pkg/signals/parse.go`)

Port schmux's parser with `autarch` namespace.

```go
package signals

import (
	"regexp"
	"time"
)

// bracketRe matches --<[autarch:state:message]>-- on its own line.
// Tolerates leading bullets (⏺•-*) and whitespace, trailing whitespace, \r.
var bracketRe = regexp.MustCompile(`(?m)^[⏺•\-\*\s]*--<\[autarch:(\w+):([^\]]*)\]>--[ \t]*\r*$`)

// ParseAgentSignals extracts all valid agent signals from data.
func ParseAgentSignals(data []byte) []AgentSignal {
	clean := stripANSIBytes(data)
	matches := bracketRe.FindAllSubmatch(clean, -1)
	if len(matches) == 0 {
		return nil
	}

	now := time.Now()
	var out []AgentSignal
	for _, m := range matches {
		state := AgentState(m[1])
		if !IsValidAgentState(state) {
			continue
		}
		out = append(out, AgentSignal{
			State:     state,
			Message:   string(m[2]),
			Timestamp: now,
		})
	}
	return out
}

// ParseAgentSignalsAt is like ParseAgentSignals but uses the given timestamp.
func ParseAgentSignalsAt(data []byte, t time.Time) []AgentSignal {
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
			State:     state,
			Message:   string(m[2]),
			Timestamp: t,
		})
	}
	return out
}
```

**Acceptance:** Parses valid signals, rejects invalid states, handles ANSI-garbled input, rejects inline signals (not on own line).

---

### Task 4: Tests (`pkg/signals/parse_test.go`)

Comprehensive test suite ported from schmux + Autarch-specific cases.

Test categories:
1. **State validation** — all 5 valid states, case sensitivity, unknown states
2. **Basic parsing** — single signal, multiple signals, empty input, no signals
3. **Format edge cases** — empty message, leading whitespace, trailing whitespace, Windows \r\n, bullet prefixes (⏺, •, -, *)
4. **Rejection cases** — inline text before/after, missing delimiters, multi-line signals, invalid state
5. **ANSI stripping** — cursor forward→space, cursor down→newline, color codes, OSC sequences, DEC private modes
6. **Real-world terminal output** — Claude Code ⏺ prefix with ANSI, message with spaces preserved through cursor-forward replacement
7. **Multi-signal extraction** — multiple signals in same buffer, mix of valid and invalid

Target: ~25 test cases in table-driven format.

**Acceptance:** `go test ./pkg/signals/ -run TestParse -v` passes. `go test ./pkg/signals/ -run TestStripANSI -v` passes. `go test ./pkg/signals/ -run TestIsValidAgentState -v` passes.

---

## Implementation Order

Tasks 1-3 are independent files (no dependencies between them). Task 4 depends on all three. Execute 1-3 in parallel, then 4.

## Out of Scope

- Integration with `StreamEvent` / `StreamHandle` (future: Option C from brainstorm)
- Publishing `AgentSignal` to broker
- Instruction injection telling agents to emit bracket signals (Autarch-7lq)
- NudgeNik LLM classification (Autarch-bq9)
- Persistence of agent signals

## Verification

```bash
go test ./pkg/signals/ -run "TestParse|TestStripANSI|TestIsValidAgentState" -v
go build ./pkg/signals/
```
