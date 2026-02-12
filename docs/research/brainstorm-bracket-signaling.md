# Brainstorm: Bracket-Based Agent Signaling Protocol

> **Bead:** Autarch-0wp (P2)
> **Source:** schmux `internal/signal/signal.go` (145 lines)
> **Target:** Autarch `pkg/signals/` + `pkg/agenttargets/`

## Problem

Autarch dispatches AI agents (Claude, Codex) via `pkg/agenttargets/` and streams their output as `StreamEvent`s. The signals infrastructure (`pkg/signals/`) has a rich broker/pubsub system, per-tool emitters, WebSocket streaming, and TUI overlay. **But there's no bridge between them.** Agent output is consumed as raw text — there's no structured way for agents to communicate state changes back to the orchestrator.

When a dispatched agent is blocked, needs testing, hits an error, or completes — the orchestrator has no way to know except by reading the final result text and guessing.

## What schmux does

Agents emit `--<[schmux:state:message]>--` on their own line. Five states: `working`, `needs_input`, `needs_testing`, `completed`, `error`. A regex parser strips ANSI escape sequences (4-layer pipeline: cursor-forward→space, cursor-down→newline, OSC removal, remaining CSI removal) before matching the bracket pattern.

Key design decisions:
- **Own-line anchoring** (`^...$` with `(?m)`) prevents false positives in code blocks
- **Bullet prefix tolerance** (`[⏺•\-\*\s]*`) handles Claude Code's `⏺` line prefix
- **Message field** allows freeform text (everything up to `]`)
- **Case-sensitive** state validation
- **145 lines** total including ANSI stripping

## What Autarch already has

### Signal infrastructure (rich, not wired to agents)
- `pkg/signals/signal.go` — 8 signal types, 3 severities, `Signal` struct
- `pkg/signals/broker.go` — pubsub with buffered channels, newest-wins drop
- `pkg/signals/server.go` — HTTP + WebSocket server on :8092
- `pkg/signals/client.go` — HTTP POST client for publishing
- Per-tool emitters in `internal/{gurgeh,pollard,coldwine}/signals/`
- TUI overlay (`/sig` command) reading from event store

### Agent streaming (rich, no signal parsing)
- `pkg/agenttargets/stream.go` — `StreamEvent` types (Text, Thinking, ToolUse, Result, Error)
- `pkg/agenttargets/backend_claude.go` — JSONL stream parser
- `pkg/agenttargets/backend_codex.go` — stderr progress + file output
- `pkg/agenttargets/runner_stream.go` — `CollectResult()` convenience

### The gap
No component that: (1) reads `StreamEvent.Text` content, (2) parses bracket signals from it, (3) translates agent states to Autarch signals or actions.

## Design Options

### Option A: Minimal parser only (`pkg/signals/parse.go`)

Just the parser. No integration with StreamEvent or broker. Callers strip ANSI + match brackets themselves.

```go
// pkg/signals/parse.go
func ParseAgentSignals(data []byte) []AgentSignal
func StripANSI(s string) string
```

**Pros:** Smallest change, no coupling, any consumer can use it
**Cons:** Every consumer needs to wire up their own event→parse→action pipeline

### Option B: Parser + StreamEvent scanner (`pkg/agenttargets/signal_scanner.go`)

Parser in `pkg/signals/`, scanner in `pkg/agenttargets/` that wraps a `StreamHandle` and emits parsed signals alongside regular events.

```go
// pkg/agenttargets/signal_scanner.go
type SignalScanner struct { ... }
func NewSignalScanner(handle *StreamHandle) *SignalScanner
func (s *SignalScanner) Events() <-chan StreamEvent       // passthrough
func (s *SignalScanner) Signals() <-chan signals.AgentSignal // parsed signals
```

**Pros:** Integration point exists, consumers get both streams
**Cons:** More coupling, consumers need to select on two channels

### Option C: Parser + new StreamEventType

Add `StreamAgentSignal` to `StreamEventType`. The backend parsers (claude, codex) detect bracket signals in text content and emit them as first-class events.

```go
StreamAgentSignal  // Agent emitted a bracket signal
```

`StreamEvent` gains an `AgentSignal *AgentSignal` field.

**Pros:** Signals are first-class in the event stream, single channel to consume
**Cons:** Backends need to buffer text to detect own-line signals, couples parsing into backends

### Recommended: Option A first, then Option C

Start with the pure parser (Option A). It's the schmux port — clean, testable, no dependencies. Then wire it into backends as Option C once we see how consumers use it. Option B is the middle ground but creates an extra abstraction layer that may not earn its keep.

## Agent State vs Autarch Signal: Type Mapping

Schmux has 5 agent states. Autarch has 8 signal types for spec-level alerts. These are **different concepts** — agent states are ephemeral lifecycle events, signals are persistent alerts about real-world changes.

**Don't merge them.** Keep them as separate types:

```go
// New: Agent lifecycle states (ephemeral, from bracket parsing)
type AgentState string
const (
    AgentWorking      AgentState = "working"
    AgentNeedsInput   AgentState = "needs_input"
    AgentNeedsTesting AgentState = "needs_testing"
    AgentCompleted    AgentState = "completed"
    AgentError        AgentState = "error"
)

// New: Parsed bracket signal
type AgentSignal struct {
    State     AgentState
    Message   string
    Timestamp time.Time
}
```

Autarch signals (`SignalCompetitorShipped`, etc.) remain unchanged. An `AgentSignal` can *trigger* an Autarch signal (e.g., `AgentError` on a Coldwine task → `SignalExecutionDrift`), but they're not the same thing.

## ANSI Stripping

Schmux's 4-layer pipeline is battle-tested against real terminal output. Copy it wholesale:

1. Cursor forward (`\x1b[\d*C`) → space (preserves word boundaries)
2. Cursor down (`\x1b[\d*B`) → newline (preserves line boundaries)
3. OSC sequences (`\x1b]...\x07`) → remove
4. Remaining CSI sequences (`\x1b[?...`) → remove

Order matters: cursor-forward must become spaces before the generic CSI strip would delete them.

## Namespace

Change `schmux` to `autarch` in the bracket format: `--<[autarch:state:message]>--`

## Extensibility

The bracket format supports future states without code changes — just add to the `ValidStates` map. Potential future states:
- `needs_review` — agent wants human code review
- `blocked` — agent waiting on external dependency
- `paused` — agent voluntarily yielded

## Integration Points (future, not in this bead)

1. **Coldwine agent dispatch** — scan StreamEvents for AgentSignals, update task status
2. **Bigend dashboard** — show agent states in real-time
3. **NudgeNik fallback** (Autarch-bq9) — when no bracket signals detected after N seconds, use LLM classification
4. **Gurgeh validation agents** — emit `completed` or `error` with spec validation results
5. **Log pane** — show agent state changes as log entries

## Test Plan

Port schmux's test cases (18 parse cases, 7 ANSI cases, 5 real-world terminal garble cases) and adapt:
- Change `schmux` → `autarch` in all test data
- Add Autarch-specific cases (Codex stderr format, Claude JSONL text blocks)
- Test empty message, multi-signal extraction, invalid states, case sensitivity

## File Plan

| File | Purpose | LOC estimate |
|------|---------|-------------|
| `pkg/signals/agent_signal.go` | `AgentState`, `AgentSignal` types | ~30 |
| `pkg/signals/parse.go` | `ParseAgentSignals()`, `StripANSI()` | ~100 |
| `pkg/signals/parse_test.go` | Ported + new test cases | ~200 |
| `pkg/signals/ansi.go` | ANSI stripping pipeline (reusable) | ~40 |

**Total: ~370 LOC** (higher than original estimate because tests are comprehensive)

## Open Questions

1. **Should `AgentSignal` publish to the broker automatically?** Leaning no — let consumers decide when/whether to broadcast. Keeps the parser pure.
2. **Should agents emit signals in their instructions?** Yes, but that's Autarch-7lq (instruction injection), not this bead. This bead is parse-side only.
3. **Do we need `AgentSignal` in Intermute's domain types?** Not yet — agent states are ephemeral. Only persist if we add session tracking.
