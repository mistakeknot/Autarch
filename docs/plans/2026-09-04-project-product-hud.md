---
artifact_type: plan
bead: Sylveste-bfkd
stage: implementation
---
# Connect the project product context

User direction, 2026-09-04: “Autarch should be the HUD for roadmap, backlog,
CUJ, persona, and other product/design management primitives that are key to
keeping a software project consistent and on track.” This also supplies the
durable context needed when a different model continues the work.

The current product card already rules that Autarch owns no world state.
This view reads existing product files and Beads; it does not create another
backlog, ratify generated content, or infer that a project is on track from
activity. Model-transfer defaults and reassessment policy remain open.

## Observable result

- A project view connects persona/pain, primary CUJ, success criterion,
  guardrails, roadmap, current work/backlog, and decision sources.
- Declared card states and journey validation stay attributed to their source.
  Missing success criteria, stale roadmap text, unreadable sources, and absent
  work-to-spec references remain visible rather than becoming a health score.
- Live Beads reads use its read-only CLI with bounded execution. Shared tracker
  scopes state their project-label filter explicitly; zero matches is scoped.
- Entry is explicit from project rows and `autarch project [path]`; existing
  session entry and the opening catch-up remain available.
- The view has keyboard sections, scrolling, source opening, refresh and back.
  Source references stay inspectable, with no silent cap or missing/error merge.

## Sequential implementation

1. `internal/door/product.go` and `product_test.go`: bounded source readers for
   the card, CUJ files and roadmap, plus live Beads snapshot and explicit scope.
   Prove fixture fields, source errors, path containment, read-only argv and
   shared tracker filtering before adding the UI.
2. `internal/door/product_view.go`, `model.go`, and `cmd/autarch/project.go`:
   render the connected project brief and each source section; wire row entry
   and a direct project command. Test actual content, keyboard transitions,
   refresh, narrow geometry and the source-opening command.
3. Extend isolated tmux replay and use the real Autarch project sources. Verify
   persona, the daily-walk CUJ, the explicitly dated roadmap and live scoped
   backlog. Run affected race tests and the standalone build, review, push main,
   inspect CI, install and inspect the invoked build. Record actual user feedback
   separately from automated evidence.

Prior evidence: the current `docs/roadmap.md` says it was generated from Beads
on 2026-02-25; `docs/why.md` confirms persona, pain, CUJ and guardrail while
declining a project-wide success measure. These facts must survive rendering.
The February over-planning retrospective requires reproduction before adding
abstractions. Reuse the existing card format and CUJ files directly.

These tasks share model state and are executed sequentially in this session.
