# Oracle Architecture Review: Autarch & Gurgeh
**Date:** 2026-02-01
**Model:** GPT-5.2 Pro via Oracle browser automation
**Context:** 173 files (~258K tokens)

---

## 1) Architecture

### 1.1 Monorepo shape and boundaries

Standard Go monorepo layout:
- `cmd/…` = executable entry points (e.g., `cmd/autarch`, `cmd/gurgeh`, `cmd/bigend`, `cmd/coldwine`, `cmd/pollard`).
- `internal/…` = tool-specific implementations and host TUIs
- `pkg/…` = shared libraries, especially **shared TUI primitives** (`pkg/tui`) and cross-tool signaling (`pkg/signals`)

### 1.2 Where Gurgeh lives

Gurgeh spans several layers:

**Core PRD schema + lifecycle**
- `internal/gurgeh/specs/…` — spec model + persistence + validation
- `internal/gurgeh/validation/validation.go` — perspective-based validators

**Arbiter subsystem (the sprint orchestrator)**
- `internal/gurgeh/arbiter/…` — 8-phase PRD sprint state machine, consistency engine, confidence scoring, scanning adapters, export/handoff

**TUI layers — two distinct stacks:**
1. **Suite-level TUI** (`internal/tui/…`) — `UnifiedApp` drives onboarding + sprint + dashboards; `internal/tui/views/sprint_view.go` is the chat-driven 3-pane SprintView
2. **Gurgeh-specific TUI** (`internal/gurgeh/tui/…`) — includes its own router/screens plus an older `SprintView` (`internal/gurgeh/tui/sprint.go`)

### 1.3 Cross-tool coupling

- **Gurgeh → Coldwine**: PRD → tasks/epics handoff
- **Gurgeh → Pollard**: phase-specific research + scanning
- **Bigend**: mission control / aggregation layer

Code coupling points:
- Arbiter `scanner` interface for quick scans (`orchestrator.go:64–72`, `:635–677`)
- Arbiter `ResearchProvider` for targeted scans (`orchestrator.go:590–634`)
- Handoff options include "Generate Tasks" (Coldwine) and "Deep Research" (Pollard) (`orchestrator.go:236–248`)

---

## 2) Gurgeh Arbiter: 8-Phase PRD Sprint Orchestrator

### 2.1 Phase model

8 phases in `types.go:14–61`: Vision → Problem → Users → Features&Goals → CUJs → Requirements → Scope&Assumptions → AcceptanceCriteria

### 2.2 Orchestrator lifecycle

`Start()` → initializes state with PhaseVision, generates first draft
`AcceptDraft()` → marks current phase accepted
`ReviseDraft()` → updates draft content
`Advance()` → the control loop:
1. Run consistency checks
2. Block on blocker conflicts
3. Update confidence
4. Increment phase
5. Run quick scan (conditionally)
6. Run phase research
7. Generate next draft

### 2.3 Consistency checking

- `consistency.Engine.Check()` — checks vision alignment, user-feature alignment
- **Concern:** Uses hard-coded phase indices (1 and 3) — brittle if ordering changes
- **Duplication:** Second older consistency system at `internal/gurgeh/consistency/…`

### 2.4 Confidence scoring

Two layers:
1. `confidence.Calculate()` — base = accepted/total, penalties for conflicts, research/shape bonuses
2. `Orchestrator.updateConfidence()` — blends in scan-derived phase quality

---

## 3) TUI Layer: Bubble Tea Architecture

### 3.1 Model layering

- `UnifiedApp` = root `tea.Model` (owns current view, factories, global routing)
- Each screen implements `pkgtui.View` (Init/Update/View/Focus/Blur)
- Layout via `pkg/tui/ShellLayout` (3-pane: sidebar/document/chat)

### 3.2 Message flow: SprintView ↔ UnifiedApp

1. Kickoff → `ProjectCreatedMsg`
2. UnifiedApp creates SprintView, calls `StartSprint…` → `SprintDraftUpdatedMsg`
3. User chats → `SprintStreamLineMsg*` → `SprintStreamDoneMsg`
4. User accepts → `SprintPhaseAdvancedMsg` → next `SprintDraftUpdatedMsg`
5. Final phase → `SprintCompleteMsg` → `OnboardingCompleteMsg` → `enterDashboard()`

---

## 4) Code Quality Issues

### 4.1 Concurrency hazards
- **`State()` returns internal pointer** — callers can mutate without sync (`orchestrator.go:74–82`)
- **Manual unlock/relock in `ChatAcceptDraft`** — fragile pattern (`orchestrator.go:770–807`)
- **Background deep scan mutates shared state** without locking (`deepscan.go:29–86`)

### 4.2 Research wiring incomplete
- Coordinator unused in ArbiterView (`tui/arbiter_view.go:52–61`)
- `RunTargetedScan` is a no-op (`intermute.go:154–158`)

### 4.3 Scan timing drift
- Quick scan triggers at Features&Goals but docs say after Problem — Users phase can't benefit

### 4.4 SprintView focus handling
- Doesn't consult `shell.Focus()` — Tab may visually move focus but keystrokes still go to chat

### 4.5 Review mapping inconsistencies
- Problem and Users both map to `spec.UserStory.Text` in `StartReviewSprint`

### 4.6 Silent error handling
- `Start()` discards Intermute spec-creation errors
- `runQuickScan` and `runPhaseResearch` swallow errors without logging

---

## 5) Design Strengths

1. **Clean control-loop decomposition** — `Advance()` centralizes progression rules
2. **Strong UI/domain separation** — `pkg/tui` primitives reused across all views
3. **Message-based navigation** — explicit message types avoid tight coupling
4. **Ports/adapters pattern** — scan artifacts avoid import cycles
5. **Review mode with signals** — coherent direction for spec review workflow

---

## 6) Top 5 Recommendations (by impact)

### 1. Fix Orchestrator state ownership + concurrency
- `State()` should return deep copy
- All mutations through mutex-protected methods
- Consider actor pattern (channel-based)
- Files: `orchestrator.go:74–82`, `:770–807`, `deepscan.go:29–86`

### 2. Unify & complete research integration
- Implement real `RunTargetedScan`
- Pass real provider into orchestrator
- Surface failures in UI
- Files: `intermute.go:154–158`, `arbiter_view.go:52–61`, `orchestrator.go:590–634`

### 3. Resolve duplicate implementations
- Two SprintViews, two confidence systems, two consistency systems
- Pick one source of truth per concern, delete the other
- Files: `views/sprint_view.go` vs `gurgeh/tui/sprint.go`, `arbiter/confidence` vs `gurgeh/confidence`, `arbiter/consistency` vs `gurgeh/consistency`

### 4. Fix phase semantics drift
- Move quick scan to after Problem acceptance
- Fix review extractor mappings to match ExportToSpec
- Replace hard-coded indices with Phase constants
- Files: `orchestrator.go:205–214`, `:320–415`, `consistency/engine.go:51–109`

### 5. Make SprintView fully ShellLayout-focus aware
- Route key handling by `shell.Focus()`
- Follow `signals.go:152–176` pattern
- Files: `sprint_view.go:191–217`, `shelllayout.go:84–162`

### Bonus fixes
- Surface Intermute spec creation failures (`orchestrator.go:111–118`)
- Ensure export helpers referenced by `export.go` exist
- Add tests for phase transitions, blocker gating, confidence scoring
