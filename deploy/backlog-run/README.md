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

## Triage seat: what it can and cannot do
The seat comes from Clavain routing (role `coordination`) and runs through the packaged dispatch wrapper in an empty
directory, with credential-looking environment variables removed. Card text reaches it as quoted data and only its
labels are used. The wrapper itself gives a Claude seat a Bash allowance and offers no tool-free profile, so a card that
carries hostile instructions could in principle make the seat run a command as the same user. A tool-free profile in the
wrapper is the real fix and is tracked as a follow-up request to the Clavain owners.
