# ADR: specs.Spec is the Canonical PRD Type

**Date:** 2026-02-01
**Status:** Decided
**Bead:** Autarch-cxz

## Decision

`specs.Spec` is the canonical type for all PRD/vision content. `specs.PRD` becomes a read-only view/adapter layer for version grouping and Intermute sync.

## Context

Two competing PRD representations exist:
- `specs.Spec` — 30 fields, used by 48 files, all creation flows produce this type
- `specs.PRD` — 8 fields, used by 16 files, always derived from Spec via `MigrateSpecToPRD()`

## Evidence

| Criterion | Spec | PRD |
|-----------|------|-----|
| User-facing creation | All flows | None |
| File adoption | 48 files | 16 files |
| Analysis modules | 50+ | 2-3 |
| Schema richness | Hypotheses, assumptions, market research | Flat feature container |
| Vision support | Yes (Type field) | No |
| Cross-tool sync | Via PRD adapter | Direct to Intermute |

## Migration Path

1. **Keep PRD as derived view** — `MigrateSpecToPRD()` stays for backward compat
2. **Add direct Spec→Intermute sync** — currently only PRD syncs to Intermute
3. **Deprecate PRD creation APIs** — remove `NewPRD()`, `AddFeature()` as public APIs
4. **Legacy support** — `LoadAllPRDs()` remains for `.praude/` projects

## Consequences

- `Autarch-5ie` (expand interview fields) should enrich `specs.Spec`, not `specs.PRD`
- Status vocabulary should standardize on Spec's: `draft → research → validated → archived`
- PRD's `approved → in_progress → done` status values become aliases mapped at the Intermute boundary
