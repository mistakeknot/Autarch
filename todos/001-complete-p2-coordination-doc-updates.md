---
status: complete
priority: p2
issue_id: "001"
tags: [docs, coordination]
dependencies: []
---

# Apply coordination doc updates from Phase 0

## Problem Statement

Phase 0 of the coordination infrastructure plan requires documentation updates (AGENTS.md, docs/FLOWS.md) that have not been applied, leaving the local-only coordination surfaces under-documented.

## Findings

- Phase 0 explicitly calls for AGENTS.md TODO updates and a new Section 17 in docs/FLOWS.md. (`docs/plans/2026-01-27-coordination-infrastructure-plan.md:11-55`)
- AGENTS.md does not list the coordination TODO items from the plan. (`AGENTS.md:55-84`)
- docs/FLOWS.md currently ends at Section 16 with no coordination infrastructure section. (`docs/FLOWS.md:686-724`)

## Proposed Solutions

### Option 1: Implement Phase 0 doc changes as written

**Approach:** Update AGENTS.md TODO + Documentation Map, and add Section 17 to docs/FLOWS.md with API surfaces, colony detection, and signal broadcast notes.

**Pros:**
- Aligns docs with plan intent
- Improves discoverability of local-only APIs

**Cons:**
- Requires careful wording updates across multiple docs

**Effort:** 1-2 hours

**Risk:** Low

---

### Option 2: Mark Phase 0 as deferred and adjust plan

**Approach:** Add a defer note in the plan and note that docs will be updated after API stabilization.

**Pros:**
- Reduces immediate doc churn

**Cons:**
- Leaves users without guidance on new APIs

**Effort:** < 1 hour

**Risk:** Medium (documentation drift persists)

## Recommended Action

Update AGENTS.md and docs/FLOWS.md to reflect coordination API surfaces, local-only policy, and colony detection.

## Technical Details

**Affected files:**
- `AGENTS.md:55-84`
- `docs/FLOWS.md:686-724`
- `docs/plans/2026-01-27-coordination-infrastructure-plan.md:11-55`

## Resources

- Coordination plan: `docs/plans/2026-01-27-coordination-infrastructure-plan.md`

## Acceptance Criteria

- [x] AGENTS.md TODO includes Pollard API, Gurgeh API, Signals WS, colony detection, Intermute request/response
- [x] AGENTS.md Documentation Map includes docs/VISION.md and docs/brainstorms/
- [x] docs/FLOWS.md includes new Section 17: Coordination Infrastructure (API Surfaces)
- [x] docs/FLOWS.md includes colony detection and signal broadcast notes in the referenced sections

## Work Log

### 2026-01-28 - Initial Discovery

**By:** Codex

**Actions:**
- Reviewed coordination plan Phase 0 doc requirements
- Verified AGENTS.md and docs/FLOWS.md do not include required updates

**Learnings:**
- Documentation drift exists between plan and current repo docs

### 2026-01-28 - Implementation

**By:** Codex

**Actions:**
- Updated AGENTS.md docs map + project status to reflect coordination APIs and local-only surfaces
- Added colony detection note to Bigend flow and signal broadcast note to signals section
- Added Section 17: Coordination Infrastructure (API Surfaces)

**Learnings:**
- Coordination docs were missing newly shipped local-only endpoints

## Notes

- Keep local-only policy language consistent with recent decisions.
