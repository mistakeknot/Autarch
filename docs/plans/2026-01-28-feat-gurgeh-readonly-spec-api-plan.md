title: "feat: gurgeh read-only spec api for agents"
type: feat
date: 2026-01-28

# feat: gurgeh read-only spec api for agents

## Enhancement Summary
**Deepened on:** 2026-01-28  
**Sections enhanced:** Proposed Solution, Technical Considerations, SpecFlow Analysis, Implementation Plan, Test Plan  
**Research inputs:** repo scan (`internal/gurgeh/server`, `internal/gurgeh/specs`, `internal/gurgeh/project`), local-only bind guard, existing httpapi envelope

### Key Improvements
1. Align list endpoint with `offset/limit` while keeping `cursor` as a backward-compatible alias.
2. Use `project.SpecsDir`/`project.ArchivedSpecsDir` to support `.gurgeh` and legacy `.praude` layouts.
3. Add concrete test cases via `httptest` to lock behavior (pagination, archived, error codes).

### New Considerations Discovered
- Existing server already implements full endpoints; work is additive (pagination + archived + tests).
- `specs.LoadSummariesWithArchived` is available and should be used for include_archived.
- Local-only guard is centralized in `pkg/netguard.EnsureLocalOnly`.

## Overview
Ship a local-only, read-only Gurgeh Spec API that external agents can use to query spec summaries, full specs, requirements, CUJs, hypotheses, and history. The server already exists; this plan aligns it with the decisions from the brainstorm (offset/limit pagination, include archived option, shared response envelope, strict local-only).

## Current State
- `internal/gurgeh/server/server.go` already provides:
  - `/health`
  - `/api/specs` list
  - `/api/specs/{id}` detail
  - `/api/specs/{id}/requirements`, `/cujs`, `/hypotheses`, `/history`
- Uses `pkg/httpapi` envelope and `pkg/netguard.EnsureLocalOnly`.
- Pagination is currently `cursor` + `limit` (cursor is numeric offset).
- List only reads active specs from `.gurgeh/specs` (no archived option).

## Gaps to Close
1. Align pagination with **offset/limit** terminology (agents expect offset/limit).
2. Add `include_archived` option for list endpoint using `.gurgeh/archived/specs`.
3. Use project path helpers (`internal/gurgeh/project`) for root/archived compatibility.

## Proposed Solution
- Keep the existing server and routes, but:
  - Accept `offset` and `limit` query params (fallback to existing `cursor` for compatibility).
  - Add `include_archived=true` to list archived specs.
  - Use `project.SpecsDir(root)` and `project.ArchivedSpecsDir(root)` for path resolution.
- Maintain `pkg/httpapi` response envelope for consistency with Pollard.
- Keep strict local-only binding via `netguard.EnsureLocalOnly`.

### Research Insights
**Best Practices:**
- Preserve stable ordering by spec ID before paginating to keep `offset` deterministic.
- Return an empty list with `meta.cursor=""` when offset exceeds length.
- Treat `include_archived` as false by default; accept `true/1/yes` as truthy.

**Edge Cases:**
- `offset` should take precedence over `cursor` when both provided.
- `include_archived` should default to false and only include archived when explicitly true.

## Technical Considerations
- **Response envelope:** continue `httpapi.WriteOK` / `httpapi.WriteError`.
- **Ordering:** sort by spec ID for stable pagination.
- **Meta:** keep `meta.cursor` as the **next offset** (even if input is `offset`).
- **Archived behavior:** only include archived when `include_archived=true`.
- **Backward compatibility:** support both `offset` and `cursor` parameters (offset wins).

### Research Insights
**Implementation Details:**
- Use `project.SpecsDir(root)` / `project.ArchivedSpecsDir(root)` rather than manual `.gurgeh` paths.
- Parse `include_archived` as a boolean (`true`, `1`, `yes`) to avoid surprises.
- Keep default limit at 50; ignore invalid limit values and fall back to default.

## Acceptance Criteria
- [ ] `GET /api/specs` supports `offset`, `limit`, and `include_archived`.
- [ ] Responses remain wrapped in `pkg/httpapi` envelope.
- [ ] Non-loopback bind addresses are rejected.
- [ ] Endpoints return 404 for missing specs and 405 for invalid methods.
- [ ] No Intermute changes required.

## SpecFlow Analysis (User/Flow Gaps)
### Flow Overview
1. **List specs** → agent requests `offset/limit`, optionally `include_archived`.
2. **Spec detail** → agent fetches `/api/specs/{id}`.
3. **Spec sub-resources** → agent fetches requirements/CUJs/hypotheses/history.
4. **Errors** → missing spec (404), invalid method (405), invalid pagination (defaults applied).

### Missing Elements & Gaps
- **Pagination defaults**: Decide default `limit` (recommend 50).
- **Archived semantics**: Confirm archived specs are excluded by default.
- **Error shape**: Keep existing `httpapi.ErrInvalidRequest` and `ErrNotFound`.

### Research Insights
**Best Practices:**
- Use `http.StatusMethodNotAllowed` with `ErrInvalidRequest` for non-GET routes, consistent with Pollard.
- Treat malformed pagination params as defaults rather than returning 400 to keep agent clients simple.

### Critical Questions
1. Should we expose `offset` only, or support `cursor` for backward compatibility? **Decision:** support both.
2. Should default `limit` remain 50? **Decision:** yes.

## Implementation Plan

### Phase 1: Align list endpoint
- [x] Update pagination parsing to accept `offset` and `limit` (fallback to `cursor`).
- [x] Add `include_archived` query param.
- [x] Use `project.SpecsDir` / `project.ArchivedSpecsDir` and `specs.LoadSummariesWithArchived`.

### Phase 2: Tests
- [x] Add `internal/gurgeh/server/server_test.go` to cover:
  - offset/limit pagination
  - include_archived behavior
  - 404 for unknown spec id
  - 405 for invalid methods

### Research Insights
**Test Additions Suggested:**
- Verify `offset` beats `cursor` when both set.
- Verify `include_archived` default is false and truthy parsing works (`true/1/yes`).
- Verify response envelope meta cursor increments by limit (or empty if end).
- Verify legacy `.praude` roots still resolve via `project.SpecsDir`.

### Phase 3: Documentation
- [x] Add a short note to `docs/FLOWS.md` or `docs/INTEGRATION.md` with endpoint list + pagination params.

## Test Plan
- `go test ./internal/gurgeh/server`
- `go test ./internal/gurgeh/specs`
- `go test ./...` (optional sweep)

### Research Insights
**Regression Focus:**
- Ensure `/api/specs/{id}/history` still works after path helper switch.
- Ensure archived specs resolve correctly via legacy `.praude` roots.
