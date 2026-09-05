---
artifact_type: plan
bead: Sylveste-pilf
stage: implementation
---
# Comfortable catch-up, without arguments

User feedback: the TUI is stark and requiring `--since 24h` feels unfriendly.
User chose both Cozy and Compact options for comparison, like Gmail density.

Bare Autarch already uses the last visit, or 24 hours on a first visit. Make
that entry path obvious. Use the existing theme with clear navigation, a
framed reading area, restrained accents, and human-readable time labels.
Cozy gives project changes breathing room and wraps their summaries; Compact
keeps more projects visible. Both retain question evidence and source limits.

## Observable requirements

- Open bare `autarch`; use visible controls to choose a time range or density.
- Cozy/Compact can be compared immediately; density survives reopening.
- Time choices include the opening window, 24 hours, 3 days, a week and a month.
  Changing a range during a read queues it instead of mixing asynchronous data.
- Keyboard and clickable controls work; help explains navigation and handoff.
- The product HUD shares the density setting. Source truth, quiet orientation,
  explicit session entry and the last-visit rule remain unchanged.
- Both modes fit narrow and normal terminals without clipping chrome or
  silently dropping the tail of a scrollable list.

## Sequential implementation

1. `internal/door/display.go`, `display_test.go`, `model.go`: local density
   preference, controls, range selection/queue and keyboard navigation.
2. `internal/door/dashboard.go`, `catchup.go`, `product_view.go`: shared visual
   frame, readable hierarchy and two meaningful density treatments.
3. `cmd/autarch/door.go`, `project.go`, `README.md`, terminal tests: wire defaults
   and mouse controls; prove bare-command entry, switching, persistence,
   range changes and original question/session flow. Review, test, push, install.

Prior learning: `tui-breadcrumb-hidden-by-oversized-child-view-20260127.md`
requires one shared geometry calculation for body and chrome. ANSI-aware
wrapping/truncation is mandatory. Existing preference tests must never write
the user's config; display preferences use an explicit test path.

These steps share model/layout state and execute sequentially in this session.
