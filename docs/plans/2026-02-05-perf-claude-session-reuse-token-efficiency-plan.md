---
title: "perf: Claude Code Session Reuse for Token Efficiency"
type: perf
date: 2026-02-05
status: reviewed
reviewers: DHH (APPROVE), Kieran (REVISE→fixes incorporated), Simplicity (APPROVE)
---

# perf: Claude Code Session Reuse for Token Efficiency

## Overview

Reduce token consumption during Gurgeh sprint by reusing the Claude Code session from initial exploration. Add an optional `sessionID` parameter to existing functions instead of creating new ones.

**Validated savings**: 71% cost reduction, 64% fewer tokens, 55% faster for resumed calls.

## Validation Results

Tested on 2026-02-05 with Claude Code CLI:

| Metric | First Call (exploration) | Resumed Call | Savings |
|--------|-------------------------|--------------|---------|
| Duration | 10.4s | 4.7s | **55% faster** |
| Cache read tokens | 85,241 | 30,903 | **64% fewer** |
| Cost | $0.32 | $0.09 | **71% cheaper** |
| Context preserved? | - | ✅ Yes | - |

The resumed session correctly recalled file contents without re-reading them.

## Simplified Implementation (~60 LOC)

Based on reviewer feedback (DHH, Kieran, Simplicity), this plan follows the simplest approach:

1. **No new structs** — return tuple `(map[string]any, string, error)` from `Explore()`
2. **No new functions** — add optional `sessionID` param to existing `GeneratePhase()`
3. **No new tiers** — session reuse is just part of the existing flow, falls back naturally
4. **Fix deprecations** — replace `strings.Title` with `cases.Title` in all touched files

### Step 1: Update `streamMessage` and `Explore()` to Capture Session ID

**File**: `internal/gurgeh/exploration/explore.go`

Add `SessionID` field to `streamMessage`:

```go
type streamMessage struct {
    Type      string         `json:"type"`
    SessionID string         `json:"session_id"` // NEW
    Result    string         `json:"result"`
    IsError   bool           `json:"is_error"`
    Message   *streamContent `json:"message"`
}
```

Update `Explore()` to return session ID as second value:

```go
// Explore runs Claude Code and returns parsed output plus session ID.
// Session ID can be used with GeneratePhase() to avoid re-exploration.
func Explore(ctx context.Context, cwd string) (map[string]any, string, error) {
    // ... existing setup ...

    var sessionID string
    var finalResult string
    var isError bool

    for scanner.Scan() {
        line := scanner.Text()
        var msg streamMessage
        if err := json.Unmarshal([]byte(line), &msg); err != nil {
            continue
        }

        // Capture session ID (first non-empty one wins)
        if msg.SessionID != "" && sessionID == "" {
            sessionID = msg.SessionID
            slog.Info("exploration session", "id", sessionID)
        }

        // ... existing tool logging and result capture ...
    }

    // ... existing result parsing ...

    return result, sessionID, nil
}
```

### Step 2: Add Session ID to SprintState

**File**: `internal/gurgeh/arbiter/types.go`

```go
type SprintState struct {
    // ... existing fields ...
    ExplorationSessionID string // Claude Code session for reuse
}
```

### Step 3: Update `GeneratePhase()` to Accept Optional Session ID

**File**: `internal/gurgeh/exploration/explore.go`

Add `sessionID` parameter. When non-empty, use `--resume` and a simpler prompt.
On `--resume` failure, retry once without session (graceful fallback).

```go
// GeneratePhase asks Claude Code to generate content for a specific phase.
// If sessionID is non-empty, resumes that session to avoid re-exploring.
// Falls back to fresh exploration if resumed session fails.
func GeneratePhase(ctx context.Context, cwd string, phase string,
    priorContext map[string]string, sessionID string) (string, error) {

    ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
    defer cancel()

    // Build context from prior phases
    var contextParts []string
    phaseOrder := []string{"vision", "problem", "users", "features", "cujs",
                           "requirements", "scope", "acceptance"}
    titler := cases.Title(language.English)
    for _, p := range phaseOrder {
        if content, ok := priorContext[p]; ok && content != "" {
            contextParts = append(contextParts, fmt.Sprintf("## %s\n%s",
                titler.String(p), content))
        }
    }
    priorContextStr := strings.Join(contextParts, "\n\n")

    // Choose prompt based on whether we have a session to resume
    var phasePrompt string
    if sessionID != "" {
        phasePrompt = fmt.Sprintf(`Generate the %s section for this PRD.

You already explored this codebase. Use that knowledge.

PRIOR SECTIONS:
%s

Be specific to THIS project. 2-4 paragraphs max. No placeholders.
Return ONLY the section content.`, phase, priorContextStr)
    } else {
        phasePrompt = fmt.Sprintf(`Generate content for the %s section of a PRD.

Explore this codebase to understand what it does, then write the %s section.

PRIOR CONTEXT (approved phases):
%s

Be concise and specific to THIS project. Extract evidence from the codebase.
2-4 paragraphs max. Return ONLY the section content.`, phase, phase, priorContextStr)
    }

    slog.Info("generating phase", "phase", phase, "resumed", sessionID != "")

    // Build command args
    args := []string{"-p", phasePrompt, "--output-format", "stream-json",
                     "--verbose", "--print"}
    if sessionID != "" {
        args = append(args, "--resume", sessionID)
    }

    result, err := runClaude(ctx, cwd, args)

    // If resumed session failed, retry without session (Issue D fix)
    if err != nil && sessionID != "" {
        slog.Warn("session resume failed, retrying fresh", "phase", phase, "err", err)
        args = []string{"-p", phasePrompt, "--output-format", "stream-json",
                        "--verbose", "--print"}
        result, err = runClaude(ctx, cwd, args)
    }

    return result, err
}
```

> **Note**: Extract shared output-parsing into a `runClaude(ctx, cwd, args)` helper to avoid duplicating the scanner loop. This is existing code refactored, not new logic.

### Step 4: Fix `strings.Title` Deprecation (while in file)

**File**: `internal/gurgeh/exploration/explore.go` — 4 call sites

| Line | Current | Fix |
|------|---------|-----|
| 117 | `strings.Title(p)` | `titler.String(p)` |
| 217 | `strings.Title(key)` | `titler.String(key)` |
| 227 | `strings.Title(p)` | `titler.String(p)` |
| 421 | `strings.Title(p)` | `titler.String(p)` |

Add import: `"golang.org/x/text/cases"` and `"golang.org/x/text/language"` (already in go.mod).
Create shared titler at package level or in each function: `titler := cases.Title(language.English)`.

### Step 5: Update ALL Callers

**`Explore()` callers** — signature changes from `(map[string]any, error)` to `(map[string]any, string, error)`:

| File | Line | Change |
|------|------|--------|
| `internal/tui/unified_app.go` | ~874 | `result, sessionID, err := exploration.Explore(...)` — pass sessionID through ScanProgress |
| `internal/gurgeh/cli/commands/explore.go` | ~62 | `result, _, err := exploration.Explore(...)` — ignore sessionID |

**`GeneratePhase()` callers** — signature adds `sessionID string` parameter:

| File | Line | Change |
|------|------|--------|
| `internal/gurgeh/arbiter/orchestrator.go` | 356 | Add `state.ExplorationSessionID` as 5th arg |
| `internal/gurgeh/arbiter/orchestrator.go` | 383 | Add `state.ExplorationSessionID` as 5th arg |

**`SetExplorationResult()` or equivalent** — store session ID:

```go
// Where exploration result is stored on state:
o.state.ExplorationSessionID = sessionID
```

### Step 6 (Optional Follow-up): Remove `GeneratePhaseFromContext()`

Per Simplicity reviewer: once `GeneratePhase()` supports session reuse, `GeneratePhaseFromContext()` (lines 202-313, ~110 LOC) becomes redundant. The resumed session natively carries the codebase context that `GeneratePhaseFromContext` manually assembles.

**Not blocking for v1.** Can be done as a follow-up PR that also simplifies the 66-line three-tier fallback in `advanceInternal()` (lines 340-406) down to a flat call.

## Acceptance Criteria

### Must Have
- [x] `Explore()` returns `(map[string]any, string, error)` with session ID
- [x] `streamMessage` has `SessionID` field
- [x] `SprintState.ExplorationSessionID` persisted in sprint YAML
- [x] `GeneratePhase()` accepts `sessionID string` parameter
- [x] Resumed sessions use simpler prompt (no "Explore this codebase")
- [x] Failed `--resume` retries once without session ID (graceful fallback)
- [x] All callers updated: `Explore()` (2 sites), `GeneratePhase()` (2 sites)
- [x] All `strings.Title` calls in `explore.go` replaced with `cases.Title`

### Verification
- [x] `go build ./cmd/...` compiles cleanly
- [x] `go test ./internal/gurgeh/...` passes (unit tests; integration tests require live Claude)
- [ ] Manual test: run a sprint, observe `resumed=true` in logs for later phases
- [ ] Manual test: kill Claude session mid-sprint, observe fallback to fresh generation

## What Was Cut (per reviewer feedback)

| Original Plan | Removed | Reason |
|---------------|---------|--------|
| `ExploreResult` struct | ✂️ | Tuple return is simpler |
| `GeneratePhaseWithSession()` | ✂️ | Add param to existing function |
| Four-tier fallback | ✂️ | Session reuse is just part of existing flow |
| Token metrics | ✂️ | Check Claude dashboard instead |
| TUI indicator | ✂️ | User doesn't need to know |
| `truncateID()` helper | ✂️ | Log full session ID — logs should be grep-able |

## Risks

| Risk | Mitigation |
|------|------------|
| Session expired | Retry once without `--resume` (Issue D fix) |
| Different cwd | Claude Code validates; error triggers retry |
| Breaking signature change | All 4 call sites enumerated and updated in same PR |

## Files Changed

| File | Change |
|------|--------|
| `internal/gurgeh/exploration/explore.go` | SessionID field, return tuple, update GeneratePhase, fix strings.Title |
| `internal/gurgeh/arbiter/types.go` | Add ExplorationSessionID to SprintState |
| `internal/gurgeh/arbiter/orchestrator.go` | Store/pass session ID (4 call sites) |
| `internal/tui/unified_app.go` | Handle new Explore() return |
| `internal/gurgeh/cli/commands/explore.go` | Handle new Explore() return |

## Estimated Effort

~60 lines of code changes across 5 files. Half-day implementation.
