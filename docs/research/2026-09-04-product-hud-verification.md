# Project product HUD verification

Implementation: `eb5f4cb` and its four preceding commits after `53bf171`.
Work item: `Sylveste-bfkd`. Product direction and scope are recorded in
`docs/plans/2026-09-04-project-product-hud.md`.

The project HUD reads existing product files and Beads. It presents current
work alongside persona, primary journey and success condition, project success
measure, problem and guardrail. Roadmap, backlog, journeys and decision sources
have their own scrollable sections. It is entered with `i` from project rows
or `autarch project [path]`. Existing session entry remains available.

## Automated evidence

- Full offline `GOWORK=off go test -mod=readonly -race ./...` passed: 157 package
  results, including packages without tests. Agent CLIs were replaced with
  local failure stubs for the ordinary suite.
- `GOWORK=off go build -mod=readonly ./cmd/...` passed.
- Focused regressions cover source declarations, missing/unread distinctions,
  bounded reads, escaping references, read-only Beads arguments and shared
  label scope, refresh isolation, Unicode geometry, source-opening argv,
  uppercase Markdown journeys and row navigation.
- Independent review found a FIFO open that could block before validation.
  Pre-open file-type validation and a named-pipe regression fixed it. The
  subsequent reader/UI review found no important outstanding defects.
- The first Linux CI run at `9591e30` built successfully but exposed a test
  assumption about temporary-path lengths: the visible backlog label wrapped
  across two lines. The content assertion now normalizes visual whitespace
  and includes that Linux path as a regression. Application behavior was unchanged.

## Terminal evidence

An isolated tmux replay exercised all five sections, the dated roadmap,
long-document scrolling, explicit spec references, journey steps, decision
text, exact source opening through a stub editor, refresh replacing prior
work, resizing to 38 by 14, and row entry/back. All checks passed. These are
interaction checks, not a measurement of a human's comprehension time.

The live Autarch product card was read from the existing `cuj-lineage`
worktree. It showed the confirmed persona and daily-walk CUJ, the journey's
about-one-minute recognition condition, and the explicitly declined
project-wide success measure. Its roadmap visibly says it was generated
2026-02-25; this was not relabeled current from its filesystem timestamp.

The same view read the live shared Sylveste tracker with label `autarch`:
three non-closed matches at verification, two in progress. The filter and its
exclusion of unlabeled work were visible. No spec link was inferred from prose.

The original Autarch checkout still lacks the new card/CUJs: earlier
fast-forward was refused because local `go.mod` and `go.sum` edits overlap.
Opening that checkout correctly showed missing sources while retaining its
live backlog. Those unrelated edits were not changed. Source provenance is
part of the product view; opening a different worktree can show different files.

## Product boundaries

Declared validation is not measured success. File dates are not freshness
checks. The view reads intent and work without reprioritizing, ratifying or
storing a second product database. The user has not yet chosen the default
for transfer to a preferred model or how far that model should reassess an
earlier plan. This delivery makes product context visible independently of
those choices; it does not implement model transfer.
