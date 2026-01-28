---
status: pending
priority: p2
issue_id: "003"
tags: [pollard, intermute, coordination]
dependencies: []
---

# Implement Pollard Intermute request/response inbox

## Problem Statement

Phase 5 of the coordination plan (async agent-to-Pollard requests via Intermute) is not implemented, so agent colonies cannot request research via Intermute messaging.

## Findings

- Plan calls for inbox handler/protocol and wiring into `pollard serve`. (`docs/plans/2026-01-27-coordination-infrastructure-plan.md:137-147`)
- `pollard serve` only starts the HTTP server; no inbox handler or Intermute loop is present. (`internal/pollard/cli/serve.go:1-34`)
- No `internal/pollard/inbox/` package exists.

## Proposed Solutions

### Option 1: Implement inbox handler as planned

**Approach:**
- Add `internal/pollard/inbox/{handler.go,protocol.go}`
- Start inbox handler alongside HTTP server when Intermute is available

**Pros:**
- Matches plan and enables agent colonies to request research

**Cons:**
- Requires Intermute message contract design

**Effort:** 4-6 hours

**Risk:** Medium

---

### Option 2: Defer and document as out-of-scope

**Approach:**
- Mark Phase 5 deferred in plan and related docs

**Pros:**
- Avoids half-built protocols

**Cons:**
- Leaves no Intermute request path

**Effort:** < 1 hour

**Risk:** Medium (feature gap)

## Recommended Action

**To be filled during triage.**

## Technical Details

**Affected files:**
- `internal/pollard/cli/serve.go:1-34`
- `docs/plans/2026-01-27-coordination-infrastructure-plan.md:137-147`

## Resources

- Coordination plan Phase 5: `docs/plans/2026-01-27-coordination-infrastructure-plan.md:137-147`

## Acceptance Criteria

- [ ] Inbox handler can parse research requests and reply with results
- [ ] `pollard serve` starts inbox handler when Intermute is configured
- [ ] Minimal protocol types defined for request/response payloads

## Work Log

### 2026-01-28 - Initial Discovery

**By:** Codex

**Actions:**
- Verified plan’s Phase 5 steps are absent from repo
- Confirmed `pollard serve` only starts HTTP server

**Learnings:**
- Intermute request/response is still unimplemented in Autarch
