# Architecture Review — Thematic Work Lanes

**Reviewer:** fd-architecture (Flux-drive Architecture & Design Reviewer)
**Date:** 2026-02-21
**Primary output:** `/root/projects/Interverse/.clavain/quality-gates/fd-architecture.md`

This file is the research archive copy. See the primary output file for the authoritative findings index and verdict.

## Key Findings Summary

**Verdict: needs-changes**

1. **A1 (MEDIUM)** — `ComputeStarvation` in `velocity.go` has no production caller. Only `ComputeStarvationFromDB` is wired to the CLI. The `BeadStatus` type and external-bead-data API surface are premature extensibility with no concrete consumer.

2. **A2 (MEDIUM)** — Lane membership is maintained in two independent systems: `lane_members` (intercore SQLite) and `lane:<name>` bd labels. The discovery filter reads labels; velocity reads `lane_members`. No reconciliation mechanism exists. Divergence is silent.

3. **A3 (LOW)** — `sprint_create`'s `$3` lane parameter is dead code. The primary call site in `sprint.md` passes one argument; lane tagging happens separately via direct `bd label add`.

4. **A4 (LOW)** — `LanePane`, `FetchLanes`, and `FetchLaneVelocity` have no callers in any autarch TUI view — the integration seam is open.

5. **A5 (LOW)** — `HunterConfig.LaneScope` is set from `--lane` flag but consumed by zero hunter implementations, making `pollard scan --lane` a silent no-op.

6. **A6 (INFO)** — Starvation bar normalization comment claims 0–50 range but scores are unbounded.

7. **A7 (INFO)** — `TestMigrate_V12ToV13_LaneTables` tests fresh install, not the v12→v13 upgrade path.

## Architecture Observations (Safe)

- Migration strategy is correct: new tables use `CREATE TABLE IF NOT EXISTS` in `schemaDDL`; only column additions need `ALTER TABLE` blocks. Consistent with v8/v9 precedent.
- JSON field-name contract between `ic lane velocity` output and `icdata.LaneVelocity` is correctly aligned (`closed`, `open_beads`, `throughput`, `starvation`).
- `SqlDB()` accessor pattern for passing `*sql.DB` to the `lane` package is consistent with `discovery`, `portfolio`, and other store packages.
- Schema `UNIQUE` constraint on `lanes.name` correctly enforced; composite `PRIMARY KEY (lane_id, bead_id)` on `lane_members` prevents duplicate membership.
- `generateID()` uses `crypto/rand` — correct for an ID generation context.
- `SnapshotMembers` is transactional with correct diff-and-patch logic.
