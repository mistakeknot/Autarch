# Backlog run units

Templates for the `autarch backlog-run` user timers (bead mk-omogp.2). An installer fills in `@HOME@`, `@BIN@`,
`@CLAVAIN_ROOT@` and `@HOME_DB@`, writes them to the user systemd directory and enables the two timers.

- Weekly, Monday 14:00 UTC: triage new or changed cards, cross-check mk-gated cards against Home, message each
  coordinator, send the vizier one summary.
- Daily, Tuesday to Sunday 14:00 UTC: tell a coordinator about newly-ready cards only, when it has fewer than three
  in flight and the pool has headroom.
- `BACKLOG_RUN_FLAGS` in `~/.config/autarch/backlog-run.env` starts as `--dry-run` (digest and one vizier message, no
  coordinator is messaged). Empty it to go live.
- `touch <state-dir>/PAUSE` stops every run (default state dir `~/.local/state/autarch/backlog-run`).
