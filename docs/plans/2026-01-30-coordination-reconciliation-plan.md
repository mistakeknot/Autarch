# Coordination Reconciliation + Awareness Plan

**Bead:** [none]

**Goal:** Deliver a full hybrid reconciliation layer (file-first + event-first), add a unified signals/events panel, and establish the golden path + supporting docs.

**Architecture:** Hybrid reconciliation in shared packages, LWW conflict log, unified UI panel fed by Intermute WS with event-spine fallback. Run artifacts recorded via shared contract and stored locally with metadata in the event spine.

**Tech Stack:** Go 1.24, SQLite (event spine), Bubble Tea, Intermute (optional)

---

## SpecFlow Notes (Gaps + Decisions Needed)

- **Reconcile cadence:** manual only (`autarch reconcile`) or auto on tool startup?
- **Idempotency:** event IDs or checkpoints to prevent duplicate backfills.
- **Conflict log storage:** event spine table vs separate file in project.
- **Precedence rules:** LWW by timestamp only, or include tool priority + version/mtime tie‑breakers?
- **Signals panel UX:** ack/dismiss behavior and persistence.
- **Artifact retention:** size limits, cleanup policy, and privacy guidance.
- **Golden path script:** which external APIs/hunters are mocked or skipped.

---

## Progress

- [x] Task 1: Reconciliation MVP (specs + tasks + events)
- [x] Task 2: Unified Signals/Events Panel (global TUI)
- [x] Task 3: Run Artifact Schema + Capture Hooks
- [ ] Task 4: Event Spine Query Utilities
- [ ] Task 5: Schema Versioning Docs (soft versioning)
- [ ] Task 6: Local-Only Policy Docs
- [ ] Task 7: Golden Path (doc + smoke test)
- [ ] Task 8: Full Test Pass
- [ ] Task 9: Commit + Push

---

### Task 1: Reconciliation MVP (specs + tasks + events)

**Files:**
- Modify: `pkg/events/`
- Modify: `internal/gurgeh/specs/` (projection hooks)
- Modify: `internal/coldwine/specs/` or `internal/coldwine/storage/`
- Modify: `cmd/autarch/` (add `reconcile` subcommand)

**Steps:**
1) Define reconciliation policy types (entity precedence, LWW rules, conflict log schema).
   - Keep reconcile types isolated to avoid import cycles (adapter pattern as needed).
2) Implement file-first projection emitters for specs + tasks (derive events from file diffs).
3) Write reconciliation runner that:
   - Reads file state
   - Emits derived events to `pkg/events`
   - Logs conflicts (LWW) to a new conflict table or log
4) Add CLI command `autarch reconcile --project <path>` to run reconciliation.
5) Add tests for spec/task diff -> event emission and conflict logging.
6) Add explicit idempotency tests (no duplicate events on repeated runs).

**Acceptance:**
- Spec/task changes always emit events (including backfilled history).
- Conflicts are logged and surfaced (no silent drops).

---

### Task 2: Unified Signals/Events Panel (global TUI)

**Files:**
- Add: `internal/tui/views/signals.go`
- Modify: `internal/tui/unified_app.go`
- Modify: `internal/tui/messages.go`

**Steps:**
1) Add a new global panel/view that lists signals + recent events (minimal view, no extra dashboard logic).
2) Wire Intermute WebSocket feed when available; fallback to event spine queries.
3) Add filtering by source/tool/type and severity.
4) Add conflict entries from reconciliation log to the panel.
5) Add help/hints in the unified footer.

**Acceptance:**
- Panel works offline (event spine only).
- Panel updates live when Intermute WS is available.

---

### Task 3: Run Artifact Schema + Capture Hooks

**Files:**
- Modify: `pkg/contract/` (add `RunArtifact`)
- Modify: `internal/coldwine/agents/` or runner
- Modify: `pkg/events/` (artifact event)

**Steps:**
1) Define `RunArtifact` schema (type, label, path, mime, created_at, run_id).
2) Store artifact metadata in event spine (`run.artifact_added`).
3) Write artifacts to project-local `.autarch/artifacts/<run-id>/` and add `.gitignore` rule.
4) Add minimal capture hooks (log, diff, notes). Expand later.

**Acceptance:**
- Artifact metadata is queryable in events.
- Files live in project-local storage and are non-git.

---

### Task 4: Event Spine Query Utilities

**Files:**
- Add: `cmd/events/` or extend `cmd/autarch/`
- Modify: `pkg/events/`

**Steps:**
1) Add `events query` CLI (time range, type filter, entity filter).
2) Add `events since <timestamp>` helper for golden path validation.
3) Add basic indices if missing.

**Acceptance:**
- Queries are fast enough for local usage.

---

### Task 5: Schema Versioning Docs (soft versioning)

**Files:**
- Add: `docs/SCHEMA_VERSIONING.md`
- Modify: `docs/INTEGRATION.md`

**Steps:**
1) Define compatibility rules for contract + events.
2) Document how version bumps are handled.

---

### Task 6: Local-Only Policy Docs

**Files:**
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/FLOWS.md`

**Steps:**
1) Document local-only default (outbound only for hunters/research).
2) Specify Intermute + signals server as optional local services.

---

### Task 7: Golden Path (doc + smoke test)

**Files:**
- Add: `docs/WORKFLOWS_GOLDEN_PATH.md`
- Add: `scripts/golden-path-smoke.sh`

**Steps:**
1) Define the canonical flow (new project -> PRD -> tasks -> runs -> outcomes).
2) Create a smoke script that exercises the flow and checks event spine for expected events.
3) Skip external hunters by default unless `AUTARCH_GOLDEN_PATH_LIVE=1` is set.

**Acceptance:**
- Script runs locally without Intermute.
- Failures are readable and actionable.

---

### Task 8: Full Test Pass

**Steps:**
1) `GOCACHE=/tmp/go-build go test ./...`

---

### Task 9: Commit + Push

**Steps:**
1) Commit each task incrementally.
2) `git push origin main`

---

### References

- `docs/ARCHITECTURE.md`
- `docs/INTEGRATION.md`
- `docs/FLOWS.md`
