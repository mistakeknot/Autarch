title: "refactor: extract shared async jobs package"
type: refactor
date: 2026-01-28

# refactor: extract shared async jobs package

## Enhancement Summary
**Deepened on:** 2026-01-28  
**Sections enhanced:** Overview, Proposed Solution, Technical Considerations, SpecFlow Analysis, Implementation Plan, Test Plan  
**Research inputs:** repo scan, Pollard job store review, institutional learnings scan (no matches)

### Key Improvements
1. Preserve Pollard job status semantics exactly (including unused states) to avoid API drift.
2. Make retention and concurrency behavior explicit (TTL + max eviction + clone safety).
3. Expand test coverage to include cancel races, TTL expiry, and max eviction.

### New Considerations Discovered
- Status enum includes `stalled/retrying/paused` without behavior; keep for compatibility but document non-use.
- Cancellation after completion should be deterministic (return “already complete”).
- Shared package should avoid HTTP concerns and leave defaults to callers.

## Overview
Extract Pollard’s in-memory async job model into a shared Autarch package so multiple tools can reuse a common job store and status model without committing to shared HTTP handlers yet. This keeps Pollard’s existing API behavior intact while enabling other tools to adopt async jobs when needed.

## Problem Statement / Motivation
Pollard already ships a robust async job model (status lifecycle, cancellation, TTL, max retention). As coordination infrastructure expands, other tools will likely need similar async job primitives. Duplicating the Pollard model per tool risks drift and inconsistent semantics. A minimal shared package provides reuse without forcing a shared HTTP surface or Intermute schema changes before a second concrete consumer exists.

## Proposed Solution
Create a new shared package **`pkg/jobs`** (confirmed) that contains:
- Job status enum
- Job struct
- In-memory JobStore with Start/Cancel/Get and retention (TTL/max)

Update Pollard’s server to use the shared package, preserving existing HTTP endpoints and response shapes. Defer shared HTTP handlers and Intermute schema changes until another tool adopts the job model.

### Research Insights
**Best Practices:**
- Keep the status enum identical to Pollard (`queued/running/succeeded/failed/canceled/expired/stalled/retrying/paused`) to avoid subtle API drift.
- Ensure the shared store returns **cloned** job copies to prevent external mutation of in-memory state.

**Performance Considerations:**
- Preserve Pollard’s current pruning behavior (TTL and max eviction) to avoid memory growth.
- Keep pruning on Create/Get/finish to avoid a background goroutine unless needed.

**Edge Cases:**
- `Cancel` called after completion should return “already complete” consistently.
- `Start` called on non-queued jobs should return a stable error (no side effects).

## Technical Considerations
- **API stability:** Keep Pollard’s `/api/jobs` responses unchanged.
- **Isolation:** Avoid introducing HTTP concerns into the shared package.
- **Naming:** Prefer `pkg/jobs` to align with existing shared packages (`pkg/events`, `pkg/signals`).
- **Retention defaults:** Keep defaults in Pollard server; shared package remains configurable.
- **Contract alignment:** Do not merge with `pkg/contract` Run/Task states yet.
- **Typed errors:** **Defer** shared error types in v1; keep existing error strings.

### Research Insights
**Implementation Details:**
- Pollard’s `JobStore.Start` uses `context.WithCancel` and runs the job in a goroutine; preserve this behavior when extracting.
- The shared package should not introduce new dependencies or HTTP types.

**Edge Cases:**
- Retention only expires **queued/paused** jobs; terminal jobs are evicted by TTL/max.
- Max eviction should only remove **terminal** jobs to avoid losing in-flight work.

## Acceptance Criteria
- [ ] New shared package compiles and has unit tests for JobStore behavior.
- [ ] Pollard server uses the shared JobStore without changing external API behavior.
- [ ] Job statuses and retention semantics remain identical to current Pollard behavior.
- [ ] No Intermute schema or client changes.

## Success Metrics
- Pollard API endpoints continue to return expected job states and results.
- A second tool can adopt the shared JobStore without refactoring Pollard-specific code.

## Dependencies & Risks
- **Risk:** Premature API generalization.  
  **Mitigation:** Keep shared package minimal and free of HTTP types.
- **Risk:** Subtle behavior drift in Pollard job handling.  
  **Mitigation:** Preserve existing tests and add regression coverage.

## References & Research
### Internal References
- Pollard jobs store: `internal/pollard/server/jobs.go`
- Pollard server endpoints: `internal/pollard/server/server.go`
- Coordination plan: `docs/plans/2026-01-27-coordination-infrastructure-plan.md`
- Coordination API foundation plan: `docs/plans/2026-01-28-feat-coordination-api-foundation-plan.md`

### Institutional Learnings
- No relevant solutions found in `docs/solutions/` for async jobs or coordination packages.
- Note: `docs/solutions/patterns/critical-patterns.md` not present; none reviewed.

## SpecFlow Analysis (User/Flow Gaps)
### Flow Overview
1. **Create job** → Job ID returned (queued).
2. **Start job** → transitions to running, executes async function.
3. **Cancel job** → transitions to canceled, invokes cancellation.
4. **Finish job** → transitions to succeeded/failed, captures result/error.
5. **Retention** → queued/paused jobs expire after TTL; terminal jobs evicted by TTL/max.

### Research Insights
**Best Practices:**
- Document the **allowed transitions** (queued → running → terminal) so other tools don’t assume unsupported states.
- Clarify behavior for canceled jobs in result endpoints (Pollard returns 409 with error).

### Missing Elements & Gaps
- **Error semantics:** No shared error types for “not found” or “already complete.”
- **Stall/retry states:** Status enum includes stalled/retrying/paused but no behavior defined for them.
- **Cancellation race:** Behavior when cancel hits after completion is not explicitly documented.

### Critical Questions
1. Should the shared package expose typed errors (e.g., ErrNotFound, ErrNotQueued)? **Decision:** not in v1.
2. Are `JobStalled/JobRetrying/JobPaused` meant to be used now or removed from the shared enum?
3. Should JobStore expose metrics (counts, terminal jobs) or keep purely internal?

## Implementation Plan

### Phase 1: Extract shared package
- [x] Create `pkg/jobs` with:
  - `JobStatus` enum
  - `Job` struct
  - `JobStore` with `Create`, `Start`, `Cancel`, `Get`
  - retention logic (TTL, max) identical to Pollard
- [x] Add unit tests in `pkg/jobs` (port tests from Pollard if present or add new).

### Phase 2: Wire Pollard to shared package
- [x] Update `internal/pollard/server/jobs.go` to use `pkg/jobs` types.
- [x] Ensure `internal/pollard/server/server.go` compiles with no API changes.
- [x] Run Pollard server tests (if present) or add regression tests for job transitions.

### Phase 3: Documentation
- [ ] Add a brief note to coordination docs if needed (optional): shared async jobs live in `pkg/jobs`.

### Research Insights
**Test Additions Suggested:**
- Start → finish success (result set, status succeeded).
- Start → finish error (status failed, error set, result nil).
- Cancel queued vs running (status canceled, error “job canceled”).
- TTL expiry for queued/paused jobs (status expired).
- Max eviction removes **oldest terminal** jobs only.

## Test Plan
- `go test ./pkg/jobs`
- `go test ./internal/pollard/server`
- `go test ./...` (optional sweep)

### Research Insights
**Regression Focus:**
- Confirm `/api/jobs/{id}` responses unchanged post-extraction.
- Confirm `/api/jobs/{id}/result` retains 409 semantics for failed/pending.
