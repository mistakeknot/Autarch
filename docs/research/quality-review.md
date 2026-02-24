# Quality Review — Thematic Work Lanes

**Date:** 2026-02-21
**Scope:** lane store/velocity (Go), lane CLI (Go), lane TUI pane (Go), lib-discovery.sh / lib-sprint.sh (Shell)
**Verdict:** needs-changes

## Summary

The thematic work lanes implementation is structurally solid. The schema design, transaction safety in `SnapshotMembers`, event-sourced close lifecycle, and shell lane-filter integration are all correct and consistent with project conventions. Two correctness issues must be fixed before production use: silent acceptance of invalid `--days` values feeds garbage to the velocity calculator, and `sql.ErrNoRows` equality comparisons will break under wrapped error chains. The TUI `truncate` function uses byte slicing that corrupts multi-byte UTF-8 names. Nine findings are documented in the primary output file.

## Primary Output

Full findings written to: `/root/projects/Interverse/.clavain/quality-gates/fd-quality.md`

## Files Reviewed

- `/root/projects/Interverse/infra/intercore/internal/lane/store.go`
- `/root/projects/Interverse/infra/intercore/internal/lane/velocity.go`
- `/root/projects/Interverse/infra/intercore/internal/lane/store_test.go`
- `/root/projects/Interverse/infra/intercore/internal/lane/velocity_test.go`
- `/root/projects/Interverse/infra/intercore/cmd/ic/lane.go`
- `/root/projects/Interverse/hub/autarch/pkg/tui/lane_pane.go`
- `/root/projects/Interverse/plugins/interphase/hooks/lib-discovery.sh`
- `/root/projects/Interverse/os/clavain/hooks/lib-sprint.sh`
- `/root/projects/Interverse/infra/intercore/internal/db/db.go` (migration context)
- `/root/projects/Interverse/infra/intercore/internal/db/schema.sql` (schema context)
- `/tmp/qg-diff-1771665590.txt` (diff)

## Key Findings (abbreviated)

**Q1 (HIGH)** — `store.go:106,123`: `err == sql.ErrNoRows` should be `errors.Is(err, sql.ErrNoRows)`. Project's own `state/state.go:100` uses `errors.Is`. Direct equality fails for wrapped errors.

**Q2 (HIGH)** — `lane.go:415-419`: `--days` flag uses `fmt.Sscanf` with all return values discarded. Zero or negative values silently propagate to `ComputeStarvationFromDB`, making `windowStart` land in the future and returning zero throughput for all lanes.

**Q3 (MEDIUM)** — `lane.go` (8 call sites): `json.NewEncoder(os.Stdout).Encode(...)` errors are all discarded, unlike sibling `discovery.go` which checks them. Produces silent truncated output on encoding failure.

**Q4 (MEDIUM)** — `lane_pane.go:38`: `viewport.New(width-2, height-1)` lacks lower-bound guard. `strings.Repeat("─", min(p.width-4, 60))` at line 75 panics when `width < 4`.

**Q5 (MEDIUM)** — `lane_pane.go:136-140`: `truncate` uses byte slicing (`s[:max-1]`), not rune-aware slicing. Multi-byte UTF-8 lane names are silently corrupted.

**Q9 (LOW)** — `sprint.md:234` calls `sprint_create` with one argument; the new `$3` lane parameter in `lib-sprint.sh` is unreachable from the primary caller. The sprint.md workaround duplicates logic. Interface needs reconciliation.

## Migration Note

The v12→v13 migration requires no explicit `ALTER TABLE` block because the new `lanes`, `lane_events`, and `lane_members` tables are entirely new (no columns added to existing tables). They are created by the `schemaDDL` apply step (`CREATE TABLE IF NOT EXISTS`) at line 236 of `db.go`. This is correct by the project's own migration pattern.
