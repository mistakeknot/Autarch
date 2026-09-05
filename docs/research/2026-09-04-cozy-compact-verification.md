# Cozy and Compact comparison

User feedback: the daily catch-up felt stark, and the example requiring
`--since 24h` made entry feel unfriendly. The user selected both Cozy and
Compact as options to compare. Work item: `Sylveste-pilf`.

Bare `autarch` now has a framed reading area, visible navigation, a View
selector, and a Time range selector. Cozy wraps longer summaries and adds
space between projects. Compact fits more projects into the same viewport.
The choice persists locally and applies to the product HUD. The time selector
offers the opening window, 24 hours, 3 days, 7 days, and 30 days.

## Verification

- `GOWORK=off go test -mod=readonly -race ./...` passed: 121 packages with
  tests, plus packages with no test files. External agent CLIs were guarded
  by offline stubs. The affected door and CLI packages ran afresh.
- The real-tmux test launches the CLI with no arguments in an isolated home.
  It selects Compact, clicks Time range with SGR mouse input, applies three
  days, restores the opening window, quits, reopens, and verifies the saved
  density and last-visit window. It switches back to Cozy with `d`.
- The same terminal test reads question context before explicit resume,
  verifies the original conversation ID and no injected prompt, switches the
  attached client to the source seat, opens the card through a stub editor,
  and removes a structured answered question after refresh.
- Display regressions cover preference preservation, queued ranges, stale
  read metadata, repeated active-tab selection, and 120x38, 80x26, and 40x16
  geometry. The selected final range remains visible at 40x16; footer clicks
  cannot choose offscreen options.
- Independent review reproduced two defects: late start metadata could
  reopen a completed read, and selecting Threads twice could lose the Back
  destination. Synchronous counters with per-read generations/channels and
  preserving the previous screen fixed both. Re-review found no remaining
  important findings.

## Live comparison

The preview was opened with no arguments against the real local projects and
sessions. At 120x38, Cozy displayed five project summaries and Compact eight.
Both retained commit subjects, report/source limitations, and coverage.
The density preference was restored to Cozy after comparison.

These checks establish functioning controls and a usable comparison build.
They do not establish which density the user prefers or measure the original
one-minute human recognition criterion. Preferred-model continuation remains
a separate product decision.
