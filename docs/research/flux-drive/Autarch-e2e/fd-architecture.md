# Architecture Review: Autarch End-to-End Workflow Gaps

**Reviewer:** Architecture (fd-architecture)
**Date:** 2026-02-07
**Scope:** End-to-end data flow across Gurgeh, Coldwine, Pollard, Bigend, Signals, Events, Intermute

---

## 1. Executive Summary

- **The Gurgeh-to-Coldwine handoff is broken at the critical persistence step.** The `coldwine import --from-briefs` command parses briefs correctly but never writes to the database (explicit TODO at `/root/projects/Autarch/internal/coldwine/cli/commands/import.go:84`). The entire PRD-to-task pipeline produces console output but no persistent state.

- **Signal emitters exist for all three tools but Coldwine and Pollard emitters are never called.** Gurgeh's emitter is wired into the Gurgeh HTTP server (`/root/projects/Autarch/internal/gurgeh/server/server.go:139`). Coldwine's `TaskDurationDrift` and `AgentFailureDrift` methods and Pollard's `CompetitorShipped` and `ResearchInvalidation` methods are defined but have zero call sites anywhere in the codebase.

- **Dashboard views depend on an HTTP client (`pkg/autarch/Client`) that requires a running server, but no server serves all the needed endpoints.** The `ColdwineView` calls `client.ListEpics()`, `client.ListStories()`, `client.ListTasks()` -- these hit HTTP endpoints that only exist on the Bigend daemon or Intermute, neither of which is started by `autarch tui`. The views will show errors or empty state in normal usage.

- **The Bigend daemon still references the legacy `tandemonium` directory** instead of `.coldwine/` at `/root/projects/Autarch/internal/bigend/daemon/projects.go:73,117,133`. Task stats always return zeros.

- **The arbiter handoff options ("Deep Research" and "Generate Tasks") are UI-only labels** -- selecting "research" or "tasks" in the handoff menu calls `v.onComplete(v.state)`, which propagates the sprint state upward but does not invoke Pollard's Scanner or Coldwine's importer. The actual cross-tool invocation is left to the caller, and the current callers (both `ArbiterView` and `SprintView`) do not implement it.

---

## 2. Integration Gap Analysis

### 2.1 Gurgeh -> Coldwine: Broken Persistence Pipeline

**Files:**
- `/root/projects/Autarch/internal/coldwine/prd/import.go` -- PRD-to-epic conversion (works)
- `/root/projects/Autarch/internal/coldwine/prd/brief_import.go` -- brief-to-task conversion (works)
- `/root/projects/Autarch/internal/coldwine/cli/commands/import.go:84` -- **TODO: Actually persist to database**

**Problem:** The `importFromBriefs()` function at line 54 successfully parses brief markdown files into `storage.WorkTask` structs, but the import command prints "Note: Database persistence coming soon. Tasks parsed but not yet saved." The import result is only printed to stdout. There is no call to any storage layer to persist the tasks.

**Impact:** The documented workflow `Gurgeh Spec -> Decompose -> Briefs (.md) -> Import -> Coldwine Tasks (SQLite)` from AGENTS.md line 468 terminates at the "Import" step. Users who run `coldwine import --from-briefs PRD-001` see output but get no actual tasks in Coldwine.

**Evidence:** The `--dry-run` flag and the non-dry-run path produce nearly identical output -- both just print task titles. The non-dry-run path adds a note about persistence "coming soon."

### 2.2 Bigend Daemon: Legacy Directory References

**Files:**
- `/root/projects/Autarch/internal/bigend/daemon/projects.go:73` -- `TODO: Load tasks from .tandemonium/state.db`
- `/root/projects/Autarch/internal/bigend/daemon/projects.go:117` -- checks `os.Stat(filepath.Join(path, ".tandemonium"))`
- `/root/projects/Autarch/internal/bigend/daemon/projects.go:133` -- `TODO: Query .tandemonium/state.db for actual stats`

**Problem:** The `ProjectManager.scanProject()` method checks for `.tandemonium/` (the old Coldwine directory name), not `.coldwine/`. Even if it found the directory, `loadTaskStats()` returns hardcoded zeros. The aggregator at `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` does correctly use a `coldwine.NewReader()` to load stats, but the daemon's project manager -- used by the web server -- does not.

**Impact:** The Bigend web dashboard (`/root/projects/Autarch/internal/bigend/web/server.go`) shows zero task stats for all projects. The TUI aggregator path works but the daemon path does not.

### 2.3 Dashboard Views: Missing Backend

**Files:**
- `/root/projects/Autarch/internal/tui/views/coldwine.go:78-93` -- calls `v.client.ListEpics("")`, `v.client.ListStories("")`, `v.client.ListTasks("", "")`
- `/root/projects/Autarch/internal/tui/views/pollard.go:74-78` -- calls `v.client.ListInsights("", "")`
- `/root/projects/Autarch/internal/tui/views/bigend.go:118-122` -- calls `v.client.ListSessions("")`
- `/root/projects/Autarch/pkg/autarch/client.go` -- HTTP client that needs a running server

**Problem:** The `autarch.Client` is an HTTP client that makes REST calls to a base URL. For the dashboard views to show data, a server must be running that serves `/specs`, `/epics`, `/stories`, `/tasks`, `/insights`, and `/sessions` endpoints. The `autarch tui` entry point does not start any such server. The Gurgeh read-only server (`cmd/gurgeh serve`) serves specs but not epics/stories/tasks. The Bigend daemon serves sessions but not specs/insights.

**Impact:** All four dashboard views (Bigend, Gurgeh, Coldwine, Pollard) will show either "Loading..." indefinitely or errors when `autarch tui` is launched without a separate server running. The views have no local fallback -- they do not read from `.gurgeh/specs/` or `.coldwine/` directly.

### 2.4 Arbiter Handoff: Research and Tasks Are Stubs

**Files:**
- `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go:670-698` -- `GetHandoffOptions()` returns 4 options
- `/root/projects/Autarch/internal/gurgeh/arbiter/tui/arbiter_view.go:238-253` -- `handleHandoffKey()` only acts on `opt.ID == "spec"`

**Problem:** The handoff menu shows "Deep Research", "Generate Tasks", "Export Spec", and "Export PRD". When the user selects "Export Spec" (`opt.ID == "spec"`), it calls `ExportSpec()` and dispatches the result via `ArbiterCompleteMsg`. For all other options (including "research" and "tasks"), it falls through to the generic `v.onComplete(v.state)` callback, which merely signals completion to the parent view. No code exists to:
- Invoke Pollard's `Scanner.ResearchForPRD()` when "Deep Research" is selected
- Invoke Coldwine's `ImportFromPRD()` or `ImportFromBriefs()` when "Generate Tasks" is selected

**Impact:** The documented sprint handoff flow (`Press R: Run full research`, `Press T: Generate tasks`) from AGENTS.md line 522-524 are display-only labels. Selecting them ends the sprint without triggering the advertised actions.

### 2.5 Signal Emitters: Coldwine and Pollard Never Fire

**Files:**
- `/root/projects/Autarch/internal/coldwine/signals/emitter.go` -- `TaskDurationDrift()`, `AgentFailureDrift()`
- `/root/projects/Autarch/internal/pollard/signals/emitter.go` -- `CompetitorShipped()`, `ResearchInvalidation()`
- `/root/projects/Autarch/internal/gurgeh/signals/emitter.go` -- `CheckSpec()` (CALLED)
- `/root/projects/Autarch/internal/gurgeh/server/server.go:139-151` -- actual invocation site

**Problem:** A grep for `csignals`, `coldwinesignals`, `coldwine/signals` usage outside of the package definition returns zero results. Same for Pollard's signals package -- the only external reference is `psignals.NewClient()` in the Gurgeh server for publishing already-generated Gurgeh signals to the Signals WebSocket server. The Coldwine and Pollard emitter methods are never instantiated or called.

**Impact:** The Signals overlay (`/sig`) and SignalsView will never show `execution_drift`, `competitor_shipped`, or `research_invalidation` signals. Only Gurgeh's `assumption_decayed`, `hypothesis_stale`, and `spec_health_low` signals are ever generated, and only when specs are fetched through the Gurgeh HTTP server.

### 2.6 Event Spine: Only Coldwine Writes Events

**Files:**
- `/root/projects/Autarch/internal/coldwine/cli/commands/scan.go:64` -- `events.NewWriter(store, events.SourceColdwine)` for `UntrackedItemDetected`
- `/root/projects/Autarch/internal/coldwine/tui/artifacts.go:31` -- `events.NewWriter(store, events.SourceColdwine)` for `RunArtifactAdded`

**Problem:** The event spine (`pkg/events/`) defines 20+ event types spanning all tools (spec.revised, task.created, task.started, task.blocked, task.completed, run.started, run.completed, etc.). But only two event types are ever written:
1. `EventUntrackedItemDetected` -- by `coldwine scan` CLI
2. `EventRunArtifactAdded` -- by Coldwine TUI

Gurgeh never emits `EventSpecRevised`. Pollard never emits `EventInsightLinked`. Coldwine never emits `EventTaskCreated`, `EventTaskStarted`, `EventTaskCompleted`, etc.

**Impact:** The events database (`~/.autarch/events.db`) contains only untracked-item and artifact events. The SignalsView's event tab, which filters by event type including task lifecycle events, will be essentially empty.

### 2.7 Command Palette Actions: Stub Implementations

**Files:**
- `/root/projects/Autarch/internal/tui/views/bigend.go:440-442` -- "New Session" Action returns nil
- `/root/projects/Autarch/internal/tui/views/gurgeh.go:362-364` -- "New Spec" Action returns nil
- `/root/projects/Autarch/internal/tui/views/coldwine.go:314-332` -- "New Epic", "New Story", "New Task" all return nil
- `/root/projects/Autarch/internal/tui/views/pollard.go:318-330` -- "Run Research", "Link Insight" both return nil

**Problem:** Every command palette action across all four dashboard views is a stub that returns `nil`. The palette shows these commands and they can be selected, but they do nothing.

**Impact:** The command palette appears functional but is purely decorative for creation/action commands. Only "Refresh" commands work.

---

## 3. Missing Feedback Loops

### 3.1 Coldwine -> Gurgeh: No Execution Feedback

**Current state:** Gurgeh produces specs, Coldwine reads them (one-way). There is no mechanism for Coldwine to:
- Report back that a spec produced 0 implementable tasks (indicating spec quality issues)
- Notify Gurgeh when tasks consistently fail (suggesting requirements are unclear)
- Update spec status when all derived tasks are completed

The `Coldwine -> Gurgeh` arrow shown in AGENTS.md line 443 does not exist in code.

### 3.2 Pollard -> Gurgeh: Research Results Do Not Auto-Update Specs

**Current state:** Pollard produces insights in `.pollard/insights/`. Gurgeh's `exploration` package reads the filesystem during onboarding scans. But:
- When Pollard discovers a competitor shipped a feature, no signal reaches Gurgeh's spec
- The `ResearchInvalidation` signal is defined but never emitted (Section 2.5)
- The arbiter's `ResearchProvider` interface exists (`/root/projects/Autarch/internal/gurgeh/arbiter/intermute.go`) but the `ResearchBridge` requires a running Intermute server

### 3.3 Bigend: Observation Without Action

**Current state:** Bigend is defined as "READ ONLY aggregation" (AGENTS.md line 423). This is architecturally clean, but it means:
- When Bigend detects a stalled agent, there is no automated intervention
- When Bigend sees all tasks for an epic are done, it cannot trigger spec status update
- The "Agent Intelligence" feature (AGENTS.md line 105) is listed as TODO

---

## 4. Orphaned Infrastructure

### 4.1 Intermute Bridges: Fully Built, Never Connected

**Files:**
- `/root/projects/Autarch/internal/gurgeh/intermute/sync.go` -- PRD sync to Intermute
- `/root/projects/Autarch/internal/coldwine/intermute/broadcaster.go` -- task event broadcasting
- `/root/projects/Autarch/internal/coldwine/intermute/sessions.go` -- session management
- `/root/projects/Autarch/internal/pollard/intermute/publisher.go` -- insight publishing

All four Intermute bridges are fully implemented with tests. They have graceful degradation (nil client = no-op). But they are never instantiated in the actual application flow:
- The Gurgeh arbiter TUI does not create a `PRDSyncer`
- The Coldwine TUI does not create a `TaskBroadcaster`
- The Pollard CLI does not create a `Publisher` after scan completion

The only Intermute integration that runs is in the Pollard Scanner's `processIntermuteInbox()`, which is a polling inbox -- it does not proactively send results anywhere.

### 4.2 pkg/contract: Types Without Consumers

The `pkg/contract/` package defines `Initiative`, `Epic`, `Story`, `Task`, `Run`, `Outcome`, `RunArtifact`, `InsightLink` types. These are the intended "unified data contract" per ARCHITECTURE.md line 192-213.

However, the actual data types used in the codebase are:
- Coldwine: `internal/coldwine/epics.Epic`, `internal/coldwine/storage.WorkTask` (different structs)
- Gurgeh: `internal/gurgeh/specs.Spec`, `internal/gurgeh/specs.PRD` (different structs)
- Bigend views: `pkg/autarch.Epic`, `pkg/autarch.Task` (HTTP DTO types, also different)

The contract types are used only by the event spine (`events.NewWriter` writes contract-typed payloads) and have no mapping layer to/from the actual tool-specific types.

### 4.3 Pollard API: File-Based Inbox Unused

The `Scanner.ProcessInbox()` method at `/root/projects/Autarch/internal/pollard/api/scanner.go:660` implements a YAML-file-based inbox (`/.pollard/inbox/`) for receiving research requests. The complementary `SendResearchRequest()` at line 817 creates inbox messages. But:
- No code ever calls `SendResearchRequest()`
- No code ever calls `ProcessInbox()`
- The inbox directory is never created by any init command

### 4.4 pkg/events IntermuteBridge: Built, Never Attached

The event spine's `IntermuteBridge` (`events.NewIntermuteBridge()`) is documented in ARCHITECTURE.md as the way events auto-forward to Intermute. The `Writer.AttachBridge()` method exists. But the two actual `events.NewWriter()` call sites in Coldwine (`scan.go:64`, `artifacts.go:31`) never call `AttachBridge()`.

---

## 5. Workflow Automation Opportunities

### 5.1 Sprint Completion -> Task Generation (High Priority)

The most impactful missing automation. When a user completes an arbiter sprint and selects "Generate Tasks":
1. `ExportSpec()` should be called to produce a spec
2. The spec should be saved to `.gurgeh/specs/`
3. `prd.ImportFromPRD()` should be called with the spec ID
4. The result should be persisted to Coldwine's database
5. The user should be switched to the Coldwine tab showing the new tasks

Currently steps 1-5 are all manual and step 4 is impossible (persistence not implemented).

### 5.2 Sprint Completion -> Research (Medium Priority)

When a user completes a sprint and selects "Deep Research":
1. The spec content should be extracted (vision, problem, requirements)
2. `Scanner.ResearchForPRD()` should be called
3. Results should be linked to the spec ID
4. A signal should be emitted if research invalidates assumptions

Steps 1-4 could be automated from the handoff menu.

### 5.3 Signal-Driven Spec Review (Medium Priority)

When signals fire (assumption decayed, competitor shipped), the system should:
1. Record signals to the event spine (currently only written to signal store)
2. Surface them in the Signals overlay (works if signals are in events.db)
3. Offer a "Review affected specs" action that launches `StartReview()` with the signal IDs

The `StartReview()` method at `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go:723` already supports signal-aware review (pre-accepting sections without signals, flagging sections with signals). But there is no UI flow that triggers it.

### 5.4 Scan -> Event Spine -> Dashboard Refresh (Low Priority)

When any tool produces artifacts (scan completes, task changes status, spec is revised):
1. An event should be written to the event spine
2. If Intermute is available, the event should be forwarded
3. Dashboard views should auto-refresh when events arrive

The infrastructure exists (event types defined, WebSocket support in SignalsView) but no tool writes lifecycle events except Coldwine's untracked-item scanner.

---

## 6. Recommendations (Prioritized)

### P0: Wire Coldwine Brief Import Persistence

**File:** `/root/projects/Autarch/internal/coldwine/cli/commands/import.go:84`

Complete the database persistence for `importFromBriefs()`. This is the single most impactful change because it unblocks the entire Gurgeh->Coldwine pipeline. The `storage.WorkTask` struct is already correct; it just needs a `Store.InsertTask()` call.

### P1: Wire Arbiter Handoff Actions

**File:** `/root/projects/Autarch/internal/gurgeh/arbiter/tui/arbiter_view.go:238-253`

Make the "Generate Tasks" and "Deep Research" handoff options actually invoke the corresponding tool APIs:
- "Generate Tasks": call `ExportSpec()` then `prd.ImportFromPRD()` or emit a message that the parent view handles
- "Deep Research": call `Scanner.ResearchForPRD()` or invoke the `deepscan.go` flow

This does not require Intermute -- both can be direct API calls within the same process.

### P2: Fix Bigend Legacy Directory References

**File:** `/root/projects/Autarch/internal/bigend/daemon/projects.go:73,117,133`

Replace `.tandemonium` with `.coldwine` in `scanProject()` and implement `loadTaskStats()` using the same `coldwine.NewReader()` pattern the aggregator already uses. This is a small change that fixes the web dashboard.

### P3: Wire Coldwine and Pollard Signal Emitters

**Files:**
- `/root/projects/Autarch/internal/coldwine/signals/emitter.go`
- `/root/projects/Autarch/internal/pollard/signals/emitter.go`

Add call sites:
- Coldwine: After task completion, call `emitter.TaskDurationDrift()` if actual > 3x estimate
- Pollard: After competitor-tracker scan, call `emitter.CompetitorShipped()` for new findings

Write the resulting signals to the event spine so SignalsView can display them.

### P4: Add Local Data Reading Fallback to Dashboard Views

**Files:** `/root/projects/Autarch/internal/tui/views/{bigend,gurgeh,coldwine,pollard}.go`

The dashboard views currently require a running HTTP server. Add a fallback mode that reads directly from `.gurgeh/specs/`, `.coldwine/`, `.pollard/` when the HTTP client is unavailable or not configured. This makes `autarch tui` work standalone without requiring 3 servers to be running.

### P5: Emit Lifecycle Events from All Tools

**Files:**
- Gurgeh: Emit `EventSpecRevised` when specs are saved
- Coldwine: Emit `EventTaskCreated/Started/Completed/Blocked` during task lifecycle
- Pollard: Emit `EventInsightLinked` when insights are linked to specs

This populates the event spine and makes the Signals overlay useful.

---

## 7. Module Boundary Assessment

### Boundaries That Are Respected

1. **Gurgeh does not write to `.coldwine/`** -- it exports briefs to `.gurgeh/briefs/` and Coldwine reads them. Clean separation.
2. **Bigend does not write to any tool's state** -- it is genuinely read-only. The aggregator only reads.
3. **pkg/signals/ is tool-agnostic** -- signal types are shared, emitters are tool-specific in `internal/*/signals/`.
4. **Intermute bridges use interfaces** -- `PRDSyncer`, `TaskBroadcaster`, `Publisher` all accept nil clients for graceful degradation.

### Boundary Violations

1. **Coldwine imports Gurgeh internals directly:** `/root/projects/Autarch/internal/coldwine/prd/import.go:10` imports `praudeSpecs "github.com/mistakeknot/autarch/internal/gurgeh/specs"`. This is `internal/` reaching into another tool's `internal/`. The proper boundary is through `pkg/contract/` types or file-based reading.

2. **Bigend views import Coldwine internals:** `/root/projects/Autarch/internal/tui/views/bigend.go:11` imports `"github.com/mistakeknot/autarch/internal/coldwine/tasks"`. The BigendView directly uses Coldwine's internal `TaskProposal` type.

3. **Gurgeh server imports Gurgeh signals AND pkg/signals client:** `/root/projects/Autarch/internal/gurgeh/server/server.go` uses both `gsignals` (internal) and `psignals` (pkg/signals server client) -- mixing signal generation with signal publishing in a server handler.

### Overall Architecture Fit

**Acceptable with significant gaps.** The architecture is well-designed on paper -- clean tool boundaries, optional Intermute coordination, graceful degradation, shared contract types, typed signals. The problem is execution: roughly 60% of the integration infrastructure is built but unwired. The most critical gap is the Gurgeh->Coldwine import persistence, which blocks the primary user workflow. The second most critical gap is the dashboard views requiring HTTP servers that are not part of the normal launch flow.

The project has built the right abstractions (signal emitters, event writers, Intermute bridges, contract types) but has not yet connected them at the application level. The fix is wiring, not redesign.
