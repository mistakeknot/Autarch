---
status: pending
priority: p2
issue_id: "006"
tags: [reliability, timeouts]
dependencies: []
---

# Finish timeout/cancellation guardrails

## Problem Statement

The guardrails plan calls for eliminating `context.Background()` usage outside entry points and enforcing a consistent timeout policy, but multiple call sites still create background contexts directly.

## Findings

- Plan requires replacing bare `context.Background()` with caller-provided contexts and documented timeout policy. (`docs/plans/2026-01-27-task-performance-reliability-guardrails-plan.md:35-49`)
- Event bridge forwarding still uses `context.Background()` internally. (`pkg/events/writer.go:51-56`)
- Many internal call sites (TUI, Bigend aggregator, Coldwine coordination) still use `context.Background()` without timeouts (see `rg -n "context.Background" internal/ pkg/`).

## Proposed Solutions

### Option 1: Thread context through call graph

**Approach:**
- Add ctx parameters to writers/managers and pass contexts from UI/CLI
- Replace internal Background calls with derived contexts + timeouts from `pkg/timeout`

**Pros:**
- Aligns with plan and improves cancellation

**Cons:**
- Broad refactor across packages

**Effort:** 6-10 hours

**Risk:** Medium

---

### Option 2: Target critical paths first

**Approach:**
- Fix Intermute bridge, Pollard API, and Bigend refresh paths
- Leave lower-risk background uses for later

**Pros:**
- Smaller, incremental scope

**Cons:**
- Partial compliance with plan

**Effort:** 3-5 hours

**Risk:** Medium

## Recommended Action

**To be filled during triage.**

## Technical Details

**Affected files:**
- `docs/plans/2026-01-27-task-performance-reliability-guardrails-plan.md:35-49`
- `pkg/events/writer.go:51-56`

## Acceptance Criteria

- [ ] No `context.Background()` usage outside entry points unless justified
- [ ] Timeouts applied via `pkg/timeout` defaults
- [ ] Updated components accept caller contexts

## Work Log

### 2026-01-28 - Initial Discovery

**By:** Codex

**Actions:**
- Verified plan requirement for timeout policy
- Found internal Background usage in events writer and other call sites

**Learnings:**
- Guardrails plan only partially implemented; needs a follow-up refactor
