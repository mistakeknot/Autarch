# Repo patterns analysis

**Source:** Agent result from session b817bfb0
**Task ID:** af17f3b
**Date:** 2026-02-05

---

Now I have a comprehensive picture of the codebase. Let me compile the research findings.

## Repository Research Summary

### 1. Test Patterns

**Testing Framework and Style**

The codebase uses exclusively the Go standard library `testing` package -- zero use of `testify`, `require`, or `assert`. All 293 test files containing 1,077 test functions use raw `t.Fatalf`, `t.Errorf`, and explicit conditional checks. The acceptance criteria plan should be aware that any new tests must follow this convention.

**Table-Driven Tests**: Found in several places. Examples:
- `/root/projects/Autarch/internal/gurgeh/arbiter/types_test.go` (lines 26-42) -- `TestPhaseString` with `[]struct{phase, expected}`
- `/root/projects/Autarch/pkg/mcp/server_test.go` (lines 198-248) -- `TestServer_SuggestHunters` with table of queries and expected hunters
- `/root/projects/Autarch/pkg/events/intermute_bridge_test.go` (lines 93-131) -- `TestIntermuteBridge_HighImportanceForErrorEvents` with event type to importance mapping

**No `t.Parallel()` usage**: Zero occurrences of `t.Parallel()` across the entire codebase. Tests run sequentially within each package. The plan's mandate for `-race` flag is correct but the codebase does not currently use `t.Parallel()`, which means race conditions would only surface under `-race` with concurrent goroutines, not concurrent test functions.

**Integration Test Pattern**: `/root/projects/Autarch/internal/gurgeh/arbiter/integration_test.go` demonstrates the established pattern:
- Uses `_test` package suffix (external test package `arbiter_test`)
- Creates test helper structs like `testResearchProvider` in the test file
- Tests full sprint lifecycle through multiple phases
- Uses `AdvanceSync` specifically for synchronous test behavior (a conscious API design to avoid async in tests)

**TUI Testing Pattern**: `/root/projects/Autarch/internal/coldwine/tui/model_test.go` shows TUI components are tested via:
- Creating `NewModel()` directly
- Setting dependency-injected loaders (`m.TaskLoader = func()...`)
- Calling `m.Update(msg)` and checking returned commands
- Style rendering verification with `stripANSI()` helper

**Mock Patterns**: The codebase uses manual mocks (no mockgen or similar). Examples:
- `mockClient` struct in `/root/projects/Autarch/internal/gurgeh/arbiter/intermute_test.go`
- `mockIntermuteMessenger` in `/root/projects/Autarch/pkg/events/intermute_bridge_test.go`
- `testResearchProvider` in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator_test.go`
- Package-level function variables for mocking in `/root/projects/Autarch/pkg/intermute/register.go` (e.g., `var newClient = ic.New` then overriding in tests)

**Test Helper Package**: There is no dedicated `testutil` or `testhelpers` package. Each package contains its own test helpers inline. The `internal/file/` package provides `AtomicWriteFile` which is used in production but could serve tests. Tests use `t.TempDir()` or `os.MkdirTemp()` for temporary directories.

**Security-Aware Tests**: `/root/projects/Autarch/internal/gurgeh/arbiter/persistence_test.go` includes path traversal rejection tests (`TestSaveSprintStateRejectsPathTraversal`, `TestLoadSprintStateRejectsPathTraversal`, `TestSaveSprintStateRejectsEmptyID`).

**Gap for the plan**: The plan mentions "All arbiter packages: `go test -race ./internal/gurgeh/arbiter/...`" as mandatory, but there is no CI configuration or Makefile target visible that enforces this. This is a convention that must be manually followed.

### 2. Error Handling Patterns

**Sentinel Errors**: Used sparingly but consistently:
- `ErrBlocker` in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` (line 22) with `IsBlockerError()` helper using `errors.Is`
- `ErrOffline` in `/root/projects/Autarch/pkg/intermute/client.go` (line 21) for graceful degradation when Intermute is unavailable

**Error Wrapping**: Consistent use of `fmt.Errorf("context: %w", err)` throughout. Examples from orchestrator.go: `fmt.Errorf("generating vision draft: %w", err)`, `fmt.Errorf("register agent %q: %w", name, err)`.

**Graceful Degradation Pattern**: The codebase has a strong pattern of non-fatal failures:
- Orchestrator.Start (line 168): Intermute spec creation failure is logged as a warning to stderr, sprint continues
- All Intermute client methods check `c.offline` first and return `ErrOffline` -- callers decide whether to treat this as fatal
- `TaskBroadcaster.BroadcastCreated` returns nil immediately when `b.sender == nil`
- `Register()` in `pkg/intermute/register.go` returns a no-op cleanup function when `INTERMUTE_URL` is empty

**Stderr Warning Pattern**: Errors that are non-fatal are written directly to stderr via `fmt.Fprintf(os.Stderr, "warning: ...")`. This is used throughout the orchestrator for scanner failures, research failures, and persistence failures. The plan's observation about "swallowed errors" in the institutional learnings is directly relevant -- the `GenerationErrorMsg` routing issue was a real bug.

**Gap for the plan**: There are no custom error types with structured fields (e.g., no `ReservationConflictError{BlockerID: "..."}` that AC-4.2 would need). The plan assumes structured error responses for reservation conflicts, but the current Intermute client's `Reserve()` just passes through whatever error the HTTP client returns.

### 3. Configuration Patterns

**Three Config Formats in Use**:
- **TOML**: Gurgeh (`/root/projects/Autarch/internal/gurgeh/config/config.go`) and Coldwine (`/root/projects/Autarch/internal/coldwine/config/config.go`) use `BurntSushi/toml`
- **YAML**: Pollard (`/root/projects/Autarch/internal/pollard/config/config.go`) uses `gopkg.in/yaml.v3`
- **Environment variables**: Intermute client uses `INTERMUTE_URL`, `INTERMUTE_API_KEY`, `INTERMUTE_PROJECT`, `INTERMUTE_HEARTBEAT_INTERVAL`

**Legacy Path Support**: Both Gurgeh and Coldwine config loaders check for legacy directory names first (`.praude` for Gurgeh, `.tandemonium` for Coldwine), then fall back to current names. This pattern is consistent.

**Default Config Pattern**: Pollard has `DefaultConfig()` returning a full struct with sensible defaults. Coldwine has `defaultConfig()`. Gurgeh has `DefaultConfigToml` as a string constant.

**No Global Configuration**: There is no `~/.autarch/pollard-preferences.yaml` or `~/.autarch/config.yaml` in the current code, despite the plan referencing it. The only global path used is `~/.autarch/events.db` for the event store. This is a gap -- AC-3.7 (global preferences merge) has no implementation foundation.

### 4. Module Boundaries

**Clean Boundaries** (minimal cross-tool imports):
- `pkg/` packages are truly shared: `pkg/intermute/`, `pkg/signals/`, `pkg/db/`, `pkg/tui/`, `pkg/events/`, `pkg/httpapi/`, `pkg/discovery/`
- `internal/gurgeh/` does NOT import from `internal/coldwine/` or `internal/pollard/` or `internal/bigend/`
- `internal/coldwine/` does NOT import from `internal/gurgeh/` or `internal/pollard/`
- `internal/pollard/` does NOT import from `internal/gurgeh/` or `internal/coldwine/`

**Coupling Points**:
- `internal/bigend/aggregator/aggregator.go` imports from both `internal/bigend/coldwine` and `internal/gurgeh/specs` -- Bigend is the aggregation point that reads from other tools' data
- `pkg/intermute/` is the shared coordination layer used by all tools
- `pkg/signals/` defines signal types used across tools
- `pkg/events/` provides the cross-tool event store with Intermute bridge

**Interface Boundaries**: The `ResearchProvider` interface in the arbiter package is well-designed for testing (line 45 of orchestrator.go references it; `/root/projects/Autarch/internal/gurgeh/arbiter/intermute_test.go` verifies compliance with `var _ ResearchProvider = (*mockResearchBridge)(nil)`). Similarly, `MessageSender` interface in `/root/projects/Autarch/internal/coldwine/intermute/broadcaster.go` and `tmuxAPI` interface in `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go`.

**Gap for the plan**: The plan proposes an `AgentTeamsClient` interface in `pkg/` shared between Coldwine and Bigend. No such interface exists today. The closest pattern is `ResearchProvider` in the arbiter package (internal, not pkg). Moving to pkg/ would be a new pattern but consistent with how `pkg/intermute/` works.

### 5. Intermute Integration Patterns

**Two Layers of Integration**:

1. **Registration Layer** (`pkg/intermute/register.go`): Simple agent registration + heartbeat. Used at tool startup. Returns cleanup function. Uses env vars for config.

2. **Full Client** (`pkg/intermute/client.go`): Complete CRUD client wrapping the base Intermute client (`github.com/mistakeknot/intermute/client`). Supports:
   - REST operations: Spec, Epic, Story, Task, Insight, Session, CUJ, Reservation CRUD
   - WebSocket: Real-time event subscriptions via `Connect()` and `On()` handlers
   - Messaging: `SendMessage()`, `InboxSince()`
   - Reservations: `Reserve()`, `ReleaseReservation()`, `ActiveReservations()`, `AgentReservations()`
   - Offline mode: All methods return `ErrOffline` when no URL configured

**Type Conversion Layer**: `/root/projects/Autarch/pkg/intermute/types.go` contains Autarch-local type definitions that mirror Intermute types, with bidirectional conversion functions (`toIntermuteSpec`/`fromIntermuteSpec`, etc.). This is a deliberate decoupling -- Autarch types have their own status enums and enriched fields.

**Manager** (`/root/projects/Autarch/internal/intermute/manager.go`): Auto-spawns Intermute server if not already running. Health check on `/api/sessions`. Binary lookup in PATH, `~/.autarch/bin/`, `/usr/local/bin/`.

**Event Broadcasting**: `/root/projects/Autarch/internal/coldwine/intermute/broadcaster.go` wraps `MessageSender` interface to broadcast task lifecycle events as Intermute messages with structured JSON payloads and importance levels.

**Reservation API**: The `Reserve()` method delegates to `c.base.Reserve()` which calls the Intermute REST API. The plan correctly identifies that glob overlap detection is missing in Intermute itself -- the Autarch client just passes through the path pattern string.

### 6. MCP Server Patterns

**Implementation** (`/root/projects/Autarch/pkg/mcp/server.go`):
- JSON-RPC 2.0 over stdin/stdout
- Protocol version: `2024-11-05`
- Server name: `autarch`, version `0.1.0`
- Line-delimited JSON-RPC messages via `bufio.Scanner`

**Tool Registration Pattern**:
- `Tool` struct with `Name`, `Description`, `InputSchema` (raw map), and `Handler` function
- `ToolHandler` signature: `func(ctx context.Context, params map[string]interface{}) (interface{}, error)`
- `RegisterTool()` adds tools to a map; `registerDefaultTools()` registers 8 built-in tools

**Currently Registered Tools** (8 total):
1. `autarch_list_prds` -- List PRD specifications
2. `autarch_get_prd` -- Get specific PRD by ID
3. `autarch_list_tasks` -- List Coldwine tasks
4. `autarch_update_task` -- Update task status
5. `autarch_research` -- Run Pollard research
6. `autarch_suggest_hunters` -- Get recommended hunters
7. `autarch_project_status` -- Bigend project status
8. `autarch_send_message` -- Send via Intermute

**Gap for the plan**: The plan's AC-1.17 requires `autarch_list_findings` and `autarch_triage_finding` MCP tools. AC-4.7 requires `autarch_reserve_paths` and `autarch_release_paths`. None of these exist. The existing MCP server has no findings, triage, or reservation tools. The plan also recommends full CRUD for 7 entities -- currently only partial CRUD exists (list/get for PRDs, list/update for tasks, no findings CRUD at all).

**Test Pattern for MCP**: Tests send raw JSON-RPC strings to the server via `WithIO()` and decode responses. Uses `time.Sleep(50ms)` to wait for async processing -- fragile but established pattern.

### 7. Signal/Event Patterns

**Two Separate Systems** (the plan correctly identifies this):

1. **Signals System** (`/root/projects/Autarch/pkg/signals/`):
   - `Signal` struct with `Type`, `Source`, `SpecID`, `AffectedField`, `Severity`
   - `Broker` with in-memory fan-out (`map[*subscriber]struct{}`)
   - HTTP server at `/api/signals` (POST to publish, GET `/ws` for WebSocket)
   - **No persistence, no deduplication** -- pure in-memory broadcast
   - Signal types: `competitor_shipped`, `research_invalidation`, `assumption_decayed`, `hypothesis_stale`, `spec_health_low`, `execution_drift`, `vision_drift`, `task_blocked`
   - Silently drops signals when subscriber buffer (64) is full (lines 51-54 of broker.go)
   - Uses `netguard.EnsureLocalOnly()` to enforce loopback binding

2. **Events System** (`/root/projects/Autarch/pkg/events/`):
   - SQLite-backed event store at `~/.autarch/events.db`
   - `Event` with `EventType`, `EntityType`, `EntityID`, `SourceTool`, `Payload` (JSON), `ProjectPath`
   - Append-only with `Replay()` for catch-up
   - `IntermuteBridge` forwards events as Intermute messages
   - Reconciliation support with `reconcile_cursors` and `reconcile_conflicts` tables

**No Bridge Between Systems**: The Signals broker and the Events store are completely independent. Signals are published through the HTTP API to the in-memory broker. Events are written to SQLite and optionally forwarded to Intermute. There is no mechanism for a Signal to become an Event or vice versa. The plan's Research Insights Gap 4 is accurate.

**Intermute WebSocket vs Signals WebSocket**: The Intermute client's `Connect()` method subscribes to Intermute's domain events (spec/task/session changes). The Signals server has its own `/ws` endpoint for real-time signal streaming. Bigend's aggregator imports `pkg/intermute` but there is no import of `pkg/signals` in the aggregator -- confirming the plan's finding that Bigend only connects to Intermute WebSocket.

### Key Contradictions and Gaps vs. the Acceptance Criteria Plan

1. **AC-3.4a (Signal deduplication)**: The plan correctly identifies this is missing. The `(spec_id, type, affected_field)` constraint described in the PRD requirements exists nowhere in code. The Signal struct has the `AffectedField` field, but the Broker does no dedup. The Events store has no unique constraint on these fields either.

2. **AC-1.1 (Codebase scan <10s)**: The plan's own research correctly notes this is unrealistic for LLM-powered exploration. The `exploration.Explore()` function has a 10-minute timeout. The split into structural scan (<5s) and LLM exploration (<90s) is the right recommendation.

3. **AC-3.7 (Global preferences)**: No `~/.autarch/pollard-preferences.yaml` exists. The Pollard config loads from `.pollard/config.yaml` per-project only. The plan mentions this feature but there is zero implementation foundation for global preference files.

4. **AC-3.9 (Feedback rolling window)**: No `.pollard/feedback.yaml` exists in the codebase. The entire feedback/triage mechanism is described in the plan but not implemented at all. The plan treats it as something to test, but it would need to be built first.

5. **AC-4.2 (Glob overlap detection)**: Correctly identified as blocking. The `Reserve()` call in `pkg/intermute/client.go` passes the path pattern as a string to Intermute, which does a simple INSERT. No overlap checking exists anywhere in the stack.

6. **AC-2.7 (SQLite task state)**: Coldwine's `internal/coldwine/storage/` has SQLite-backed storage with the `pkg/db` helper (WAL mode, `MaxOpenConns(1)`). The plan's concern about `SQLITE_BUSY` under 3+ concurrent agent sessions is valid given the single-connection setting.

7. **MCP tool surface**: Plan assumes `autarch_list_findings` and `autarch_triage_finding` exist (AC-1.17) and `autarch_reserve_paths`/`autarch_release_paths` exist (AC-4.7). Neither exists. Only 8 tools are registered, covering PRDs, tasks, research, hunters, project status, and messaging.

8. **Config format inconsistency**: Gurgeh and Coldwine use TOML; Pollard uses YAML. The plan does not address this, but it means acceptance tests need to handle both formats.

9. **Atomic file writes**: The `internal/file/atomic.go` provides `AtomicWriteFile` with proper temp-file-then-rename semantics and `fsync`. However, the plan's concern about `SaveRevision` non-atomic two-file writes is valid -- the orchestrator's `saveLocked()` calls `SaveSprintState()` which does a single file write, but `SaveRevision` (mentioned in the plan) likely writes to both a snapshot file and a metadata file without wrapping them in a transaction.

10. **No `AgentTeamsClient` interface**: The plan proposes this as a dependency (defined in `pkg/`), but nothing like it exists. The closest analog is the `ResearchProvider` interface pattern in `internal/gurgeh/arbiter/`, which is internal rather than shared.