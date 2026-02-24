# Security Review: Acceptance Criteria Plan

**Reviewer:** Security Sentinel (Claude Opus 4.6)
**Date:** 2026-02-05
**Plan under review:** `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md`
**Supporting research:** `/root/projects/Autarch/docs/research/acceptance-criteria-2026-02-05/deep-dive-security-remediation.md`

---

## Threat Model Context

### What this project actually is

Autarch is a **local-only developer tool suite** for AI-assisted PRD creation, task orchestration, and research intelligence. It runs on a single developer's machine with:

- All servers bound to `127.0.0.1` by default (confirmed in CLAUDE.md: "non-loopback requires explicit opt-in + auth")
- SQLite for local state (WAL mode, single-connection by design)
- YAML-based persistence in project directories (`.gurgeh/`, `.coldwine/`, `.pollard/`)
- WebSocket for real-time updates between local components
- Intermute (at `../Intermute`) for file reservations and cross-tool coordination
- MCP server communicating via stdin/stdout (local process boundary)
- Planned Agent Teams integration (Claude Code experimental feature) for parallel agent development

### What the plan changes about the attack surface

The acceptance criteria plan introduces several features that meaningfully expand the attack surface compared to the current baseline:

1. **Agent Teams integration (CUJ-4):** Multiple AI agent processes running concurrently, each with full MCP tool access, coordinating via file reservations. This creates a new **intra-machine trust boundary** where agents must be isolated from each other.

2. **Feedback YAML influencing agent behavior (CUJ-3):** `.pollard/feedback.yaml` becomes a behavioral control surface for agents -- preferences, exclusions, and relevance tuning all come from this file. Any local process with write access can steer agent decisions.

3. **Bigend web dashboard (CUJ-5):** A web server on `127.0.0.1:8099` with WebSocket terminal streaming, discoverable by any browser tab open on the local machine. This is the most exposed component.

4. **MCP tool surface expansion:** The plan adds `autarch_list_findings`, `autarch_triage_finding`, `autarch_reserve_paths`, `autarch_release_paths` and envisions full CRUD for 7 entities. Each tool is accessible to any process on the MCP pipe.

### Primary threat actors for this project

1. **Malicious web content (realistic):** A developer using Autarch has a browser open. Any website they visit can attempt to connect to `localhost:8099` WebSocket endpoints. This is the most realistic attack vector.

2. **Rogue local process (low probability):** Another application or script on the developer's machine could interact with Autarch's local servers. In practice, if a rogue process is running with the same user privileges, the developer has larger problems. However, the Agent Teams feature creates a legitimate multi-process scenario where agents *should* be isolated from each other.

3. **Corrupted project files (medium probability):** A developer clones a malicious repository containing crafted `.gurgeh/`, `.pollard/`, or `.coldwine/` files. This is a realistic supply-chain-adjacent scenario.

---

## Assessment of Plan's F1-F6 Findings

### Are the six identified findings correctly categorized?

The plan identifies six security findings (F1-F6). I reviewed the supporting deep-dive research at `/root/projects/Autarch/docs/research/acceptance-criteria-2026-02-05/deep-dive-security-remediation.md` and verified the actual source code for each.

| Finding | Plan Rating | My Assessment | Agree? | Notes |
|---------|-------------|---------------|--------|-------|
| F1: No glob overlap in Intermute Reserve() | CRITICAL | CRITICAL | Yes | Confirmed in code. The `Reserve()` function at `Intermute/internal/storage/sqlite/sqlite.go` does a blind INSERT. This is the architectural foundation of CUJ-4 isolation. Without it, the entire parallel agent safety model is fiction. |
| F2: No agent identity verification for reservations | HIGH | HIGH | Yes | The reservation handler accepts `agent_id` from request body without verification. `releaseReservation` performs no ownership check. Confirmed in the deep-dive's code excerpts. |
| F3: MCP tools have no access control | HIGH | HIGH (but nuanced) | Partially | Path traversal in `handleGetPRD` is confirmed -- `filename` at line 75 of `/root/projects/Autarch/pkg/mcp/handlers.go` is user-controlled with only `.yaml` suffix gating. However, the MCP scope concern (no RBAC for teammates) is forward-looking -- Agent Teams integration code does not yet exist in Go. The path traversal is concrete now. |
| F4: WebSocket wildcard origins | HIGH | HIGH | Yes | Confirmed at two locations: `/root/projects/Autarch/internal/bigend/web/server.go:439` and `/root/projects/Autarch/internal/bigend/daemon/server.go:339`, both with `OriginPatterns: []string{"*"}`. This streams tmux terminal output, which could contain API keys, tokens, or other sensitive data visible in the terminal. |
| F5: Feedback YAML poisoning | MEDIUM | MEDIUM | Yes | The feedback YAML mechanism is not yet implemented (no `feedback.yaml` parser exists), but the broader YAML handling pattern using `os.ReadFile` + `yaml.Unmarshal` into `map[string]interface{}` is used throughout, confirmed at `handlers.go:84-87` and `handlers.go:182-185`. The file size and depth concerns are real. |
| F6: X-Forwarded-For spoofing | MEDIUM | LOW-MEDIUM (for local-only) | Mostly | The vulnerability is real (confirmed in the deep-dive's code excerpt of `isLocalRequest()`), but exploitability requires Intermute to be bound to a non-loopback address, which the project explicitly forbids by default. This becomes HIGH if/when non-loopback is enabled. The phasing to "before non-loopback" is correct. |

### Missing findings (F7-F10)

The plan missed four security-relevant issues that I identified from code review:

**F7 (MEDIUM): Signal broker drops signals silently on subscriber backpressure**

File: `/root/projects/Autarch/pkg/signals/broker.go`, lines 51-54

```go
select {
case sub.ch <- sig:
default:
    // Drop if subscriber is slow.
}
```

The plan mentions this in the Research Insights section ("broker.go line 51-54 silently drops signals when subscriber buffers fill") but does NOT create a security finding for it. In the context of CUJ-3 (continuous research validation), a `SignalResearchInvalidation` that gets dropped means a developer proceeds with a PRD based on invalidated assumptions, unaware that contradicting research was found. This is a data integrity issue with security implications when the PRD guides agent behavior.

**Mitigation:** The plan's existing AC-3.4a covers deduplication but not delivery guarantees. Add an AC requiring that high-severity signals (warning, critical) use a persistent delivery mechanism -- not just in-memory fan-out. At minimum, log dropped signals to the event spine so they can be recovered.

**F8 (MEDIUM): `handleUpdateTask` path traversal identical to `handleGetPRD`**

File: `/root/projects/Autarch/pkg/mcp/handlers.go`, lines 168-172

```go
filename := id
if !strings.HasSuffix(filename, ".yaml") {
    filename = filename + ".yaml"
}
taskPath := filepath.Join(s.projectPath, ".coldwine", "tasks", filename)
```

The deep-dive research (F3) notes this but the plan's acceptance criteria (AC-F3b) only specifies a test for `handleGetPRD`. The `handleUpdateTask` path is worse because it **writes** to the traversed path via `os.WriteFile` at line 208. An MCP caller passing `id: "../../.gurgeh/specs/PRD-001"` could overwrite arbitrary YAML files in the project.

**Mitigation:** AC-F3b should be expanded to cover all handlers that construct file paths from user input, not just `handleGetPRD`. Both `handleGetPRD` and `handleUpdateTask` need `filepath.Base()` + resolved-path-under-target-directory checks.

**F9 (LOW): `SaveRevision` no longer has the non-atomic write issue described in the plan**

File: `/root/projects/Autarch/internal/gurgeh/specs/evolution.go`, lines 42-105

The plan's "Data Integrity Risks" section states: "Two files written sequentially (snapshot YAML + revision metadata) with no write-to-temp-then-rename. The function mutates spec.Version as a side effect even if file writes fail."

I reviewed the actual code and found:
1. Both files ARE written via `fileutil.AtomicWriteFile` (lines 88 and 98) -- this IS write-to-temp-then-rename.
2. The function creates a `snapshot := *spec` copy at line 79, then sets `snapshot.Version = version` at line 80 -- it does NOT mutate the input `spec.Version` (the original `spec` pointer's Version field is untouched).
3. There IS a file-level lock at line 53-62 (`fileutil.LockFile`) that serializes concurrent writers.
4. If the revision metadata write fails, there IS a best-effort rollback (line 100: `os.Remove(snapPath)`).

The plan's description of this vulnerability appears to be based on an older version of the code. The current implementation is significantly more robust. The remaining risk is that the rollback at line 100 is best-effort (`_ = os.Remove`), but this is acceptable -- a dangling snapshot without metadata is harmless (it will not appear in `LoadHistory` because that function only reads `_rev.yaml` files).

This is **not a security finding** anymore. Remove or downgrade in the plan.

**F10 (LOW): Bigend daemon server has same WebSocket wildcard as web server**

File: `/root/projects/Autarch/internal/bigend/daemon/server.go`, line 339-340

```go
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    OriginPatterns: []string{"*"}, // Allow all origins for local development
})
```

The plan's F4 finding only references the web server. The daemon server has the identical vulnerability. The fix should cover both locations.

---

## Phased Security Recommendations Assessment

The plan proposes three phases:

- **Phase 0 (blocking):** Glob overlap detection + reservation ownership verification
- **Phase 1 (before production):** Restrict WebSocket origins, add CSRF, validate feedback YAML schema
- **Phase 2 (before non-loopback):** Disable X-Forwarded-For trust, add authentication middleware

### Phase prioritization assessment

The phasing is **mostly correct** with one exception:

**F4 (WebSocket wildcard origins) should be Phase 0, not Phase 1.** The reason: F4 is exploitable *today* by any website a developer visits. It requires zero special conditions beyond "developer has a browser open while using Bigend." The `OriginPatterns: []string{"*"}` at two locations (web server and daemon server) allows `evil.com` to open `ws://127.0.0.1:8099/ws/terminal/{session}` and stream the developer's terminal output. This is the most realistic attack scenario in the entire threat model.

The fix is trivial (change `"*"` to localhost patterns), making it even more unreasonable to defer.

**Adjusted recommended phasing:**

| Phase | Findings | Rationale |
|-------|----------|-----------|
| Phase 0 (blocking CUJ-4) | F1, F2, F4 (+ F10) | F1/F2 are required for reservation integrity. F4/F10 are trivial fixes for the most realistic attack. |
| Phase 1 (before v1 release) | F3, F5, F8, F7 | MCP access control, YAML hardening, path traversal in write handler, signal delivery guarantees |
| Phase 2 (before non-loopback) | F6 | XFF trust is only exploitable if Intermute is network-exposed |

The deep-dive research at `/root/projects/Autarch/docs/research/acceptance-criteria-2026-02-05/deep-dive-security-remediation.md` actually recommends "F6 first -- lowest effort, immediate auth bypass risk if server is network-reachable." I disagree with that ordering *for this project's threat model*. The project explicitly binds to loopback. F6 is not exploitable until that changes. F4 is exploitable right now.

---

## Specific Questions from the Review Request

### 1. Are any findings missing from F1-F6?

Yes. Three additional findings identified:

- **F7:** Signal broker silent drops (MEDIUM) -- data integrity risk for research invalidation signals
- **F8:** `handleUpdateTask` path traversal with WRITE capability (MEDIUM) -- worse than F3's read-only path traversal in `handleGetPRD`
- **F10:** Daemon server has same WebSocket wildcard as web server (duplicate of F4 but in a different file)

Additionally, F9 (SaveRevision non-atomic writes) was **incorrectly identified** in the plan -- the code uses `AtomicWriteFile` and file locking. This should be struck from the data integrity risks section.

### 2. Are the phased security recommendations (Phase 0/1/2) prioritized correctly?

Mostly. F4 should be elevated to Phase 0. See the "Adjusted recommended phasing" table above.

### 3. Is the YAML feedback poisoning risk (F5) adequately mitigated by the proposed ACs?

**Partially.** The plan describes the risk but has no acceptance criteria in the AC tables for F5. The deep-dive research proposes `safeReadYAML()` with file size limits, `KnownFields(true)` for schema enforcement, and file permission checks. These are sound.

However, the **preference poisoning** aspect of F5 is not mitigated by schema validation alone. If `.pollard/feedback.yaml` says "exclude all security findings" and the agent obeys, schema validation will not catch this -- the content is valid YAML with a valid schema. The real mitigation is:

1. **Immutable preference categories:** Define which preference categories the feedback file can influence (e.g., domain focus, source exclusions) and which it cannot (e.g., security-related content cannot be excluded via feedback).
2. **Feedback audit trail in git:** Since `.pollard/` is git-committed, any feedback modification is visible in `git diff`. Add an AC requiring that unexpected feedback changes trigger a warning on session start.
3. **Rolling window integrity (AC-3.9):** The plan's existing AC-3.9 covers the rolling window but not crash recovery. The deep-dive correctly identifies that archive-then-truncate is not atomic.

**Recommended additional ACs:**

- AC-F5a: YAML file size limit (1 MB) enforced before parsing
- AC-F5b: YAML bomb protection -- parsing times out or fails gracefully within 5 seconds
- AC-F5c: Feedback schema validation -- only recognized preference keys accepted
- AC-F5d: World-writable feedback files are rejected with a warning

### 4. Does the reservation system's lack of agent identity verification (F2) have acceptance criteria?

**No.** This is a gap. The plan describes F2 in the Security Findings section but there are no corresponding ACs in the numbered AC tables. The deep-dive research proposes AC-F2 ("Given a reservation R owned by agent-A in project P, when agent-B sends DELETE, then 403 Forbidden") with five test cases, but these are in the research document, not in the plan's AC tables.

**The following ACs from the deep-dive should be incorporated into the plan:**

- AC-F2: Reservation release requires ownership verification (agent_id match)
- AC-F2 Test 1-5 from deep-dive (owner can release, non-owner blocked, wrong project blocked, agent_id must belong to project, localhost still checks ownership)

### 5. Are there trust boundary violations in the Agent Teams integration?

**Yes. This is the plan's most significant security gap.**

The plan describes Agent Teams integration at a feature level (AC-2.8, AC-2.9, AC-4.5, AC-4.8, AC-5.7) but does not define the trust boundary between:

1. **Lead agent vs. teammate agents.** The plan states teammates "self-claim unblocked work from the shared task list" (CUJ-4) but has no AC ensuring a teammate cannot claim tasks outside its assignment scope, release another teammate's reservations, or modify specs belonging to other tasks.

2. **Agent Teams vs. Intermute.** Coldwine "translates Agent Teams task claims into Intermute file reservations" but there is no bridge mechanism defined (this is Gap 2 in the plan). Without this bridge, there is no enforcement point. A teammate can claim a task in Agent Teams and start modifying files before Coldwine has time to acquire reservations.

3. **MCP tool access scope.** F3 identifies that teammates inherit full MCP access. The plan notes this but proposes no AC. A teammate assigned to `internal/auth/**/*.go` can invoke `autarch_update_task` on any task, `autarch_reserve_paths` on any path, or `autarch_triage_finding` on any finding. This violates the principle of least privilege.

**Missing ACs for trust boundary enforcement:**

- AC-TB1: When Agent Teams is active, MCP tools enforce scope based on the caller's assigned task. A teammate assigned to task T1 (with file pattern `internal/auth/**/*.go`) cannot invoke `autarch_reserve_paths` for `pkg/events/*.go`.
- AC-TB2: Coldwine acquires Intermute reservations BEFORE notifying the teammate that the task is available. The teammate cannot begin file modifications until the reservation is confirmed. (This is the timing-critical version of Gap 2.)
- AC-TB3: A teammate cannot release reservations it does not own.
- AC-TB4: Agent Teams' plan approval gating (AC-2.9) is enforced in Coldwine, not just in Agent Teams. If a teammate bypasses plan mode (e.g., by not using the plan tool), Coldwine rejects file modifications.

---

## Summary

### Real risk level: MEDIUM

This is a local-only developer tool where the primary realistic attack is cross-origin WebSocket hijacking from malicious web content (F4). The reservation system issues (F1, F2) are correctness problems that become security problems in the Agent Teams scenario. The YAML and MCP issues (F3, F5, F8) are defense-in-depth concerns.

### Must-fix items (Phase 0)

| ID | Issue | File(s) | Fix Effort |
|----|-------|---------|------------|
| F1 | Glob overlap detection in Intermute Reserve() | `Intermute/internal/storage/sqlite/sqlite.go` | Medium |
| F2 | Reservation ownership verification | `Intermute/internal/http/handlers_reservations.go` | Low |
| F4 + F10 | WebSocket origin restriction | `Autarch/internal/bigend/web/server.go:439`, `Autarch/internal/bigend/daemon/server.go:339` | Trivial |

### Nice-to-have hardening (Phase 1)

| ID | Issue | Fix Effort |
|----|-------|------------|
| F3 | MCP scope-based gating | Medium |
| F5 | YAML size limits + schema validation | Low-Medium |
| F7 | Persistent signal delivery for high-severity | Medium |
| F8 | Path traversal in handleUpdateTask WRITE path | Low |

### Corrections to the plan

1. **Strike "SaveRevision non-atomic writes" from Data Integrity Risks.** The code uses `AtomicWriteFile` and file locking. This was already fixed.
2. **Add F2 acceptance criteria to the AC tables.** Currently only described in prose, not in testable AC format.
3. **Add F4/F10 fix to Phase 0.** The WebSocket wildcard is the most realistic exploitable vulnerability.
4. **Add trust boundary ACs (AC-TB1 through AC-TB4)** for Agent Teams integration.
5. **Expand AC-F3b** to cover `handleUpdateTask` path traversal (F8), not just `handleGetPRD`.
6. **Add F5 acceptance criteria** to the AC tables (currently only in the deep-dive research).
