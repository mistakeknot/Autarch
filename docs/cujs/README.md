# Autarch CUJs — the tending-cadence spine

Derived 2026-08-31 from the cujgel run (discover → teardown ×3 → vision
capture; provoke absorbed organically into the teardown discussions). The
spine is **tending cadence**, not a linear loop — mk's vision capture
(docs/research/2026-08-31-vision-capture.md) ruled that a pace-layered
estate wants per-layer visit rhythms:

| CUJ | Cadence | Layer speed |
|---|---|---|
| autarch-01-daily-walk | minutes, several times a day | fast |
| autarch-02-session-dive | an hour or an evening | fast–mid |
| autarch-03-seasonal-reshaping | weekly/monthly, at season boundaries | slow |
| autarch-04-canon-tending | event-driven, rare, permanent | slowest |

All four are `status: validated` — walked with mk one journey at a time in
conversation (2026-08-31 → 09-01) and confirmed as theirs: "it seems like
all of the above CUJs are a Komoroske/Appleton-style version of product
management + gardening/tending." That lineage (digital gardens — Appleton's
ethos, Komoroske's compendium) is recorded in every spec's
`reference_lineage` as mk-named, not torn down.

v1.1 (2026-08-31): observed-behavior evidence added from
docs/research/2026-08-31-session-archaeology.md (mk: "check my sessions
to determine how I actually work"). The cadence spine is empirically
confirmed, but the tended object is the **standing thread**, not the
garden — the garden-vs-thread axis question is now in 01/02's ledgers
awaiting mk's ruling.

v1.3 (2026-09-01): validation edit pass. "The door" retired from product
vocabulary (bare `autarch` opens the vantage, pivoted by container or by
theme); entry into a garden or a theme's salon; parking = the theme
absorbs what the session learned; two discourse routers (theme, serving);
the estate's own graph as target system of record (CanonGraph
transitional); theme lifecycle inside reshaping. The rulings and the
ontology behind them: docs/research/2026-08-31-estate-ontology.md.

v1.4 (2026-09-01, autarch-01 only): consume stage — mk ruled the briefing's
source before the build ("1 then 3": git + Claude Code transcripts now, alwe
sessions async next); the last-visit window recorded as a leaning decision;
the alone-vs-above layout question stays open, to be ruled against the real
render (both layouts are built, `b` flips). Goal da757a12.

v1.5 (2026-09-02, autarch-01 only): the thread registry slice (goal cf9e8644).
mk's session note (docs/research/2026-09-02-thread-registry-probe.md) showed
the registry already living in tmux session names; three GATE rulings landed
before the build (threads on their own screen beside the garden rows; the
marks are left / right / center three-quarters of one display; entry stays
switch-client, with "standardize to iterm2" as the next entry question). The
garden-vs-thread question closed with ruling 1; two open questions added
(window-focus entry, the seatless note topics).

Validate with the cujgel plugin:
`<plugin>/bin/cujgel-validate docs/cujs/*.json`

## Schema-demand log (per cujgel derive discipline: log first, apply on recurrence)

- **`cadence` / `pace_layer` field** — all four CUJs in this run want to
  declare their tending cadence as data (it is currently carried in
  `cuj_id` naming, the README table, and `mental_model` prose). Logged
  2026-08-31, single-session occurrence; apply only if an independent
  future run demands the same axis.
