# Project foundation onboarding verification

Work item: `Sylveste-fuwn`. User priority: onboard each project to mission,
vision, roadmap, philosophy, ADRs, backlog, CUJs, personas, and design
systems/standards. The [implementation plan](../plans/2026-09-04-project-foundation.md)
records the first delivery's scope.

## Delivered behavior

Foundation is section 6 of the existing project HUD. It discovers nine areas
from bounded, contained in-repo reads and the existing live Beads scope.
Readable, absent, empty, and unreadable sources remain distinguishable.
Physical-file aliases count once. Custom/inherited sources and scan limits
are disclosed; this is not a completeness or approval score.

`n` opens an onboarding brief. `c` explicitly copies it for the chosen agent.
The brief preserves project identity, source paths, search scope, product-card
declarations and unresolved needs, and questions for each area. It asks for
cited provisional drafts, structured human questions, and explicit connections
among intent, journeys, roadmap, decisions, standards, and backlog.

## Evidence

- Full `GOWORK=off go test -mod=readonly -race ./...` passed: 121 packages with
  tests, plus packages without tests. External agent CLIs used offline stubs.
- After the live alias-count correction, both affected packages passed again
  with `-race -count=1`: `./internal/door ./cmd/autarch`.
- Unit fixtures cover conventional and alternate paths, empty files/folders,
  live empty backlogs, escaping symlinks, bounded ADR discovery without a card,
  physical-file aliases, exact clipboard payloads, visible clipboard failure,
  source preservation, and onboarding entry/back.
- Real isolated tmux replay passed: bare entry, source question evidence and
  session handoff, density persistence, Foundation inventory, onboarding brief,
  copying via a stub clipboard, unchanged mission source, a new vision found
  after refresh, and the full brief tail at 40x16.
- An initial tail assertion failed because a wrapped sentence was separated
  by the frame borders in capture text. The actual final page and scroll count
  proved the sentence visible; the assertion now strips borders before matching.
- Independent review found no important outstanding issues in the initial
  reader/UI scope. Live validation then found duplicate conventional aliases;
  a failing physical-file fixture reproduced it before correction.

## Autarch pilot

The live Autarch card worktree showed a vision source, confirmed card persona,
four journey sources, roadmap, decision references, and the shared label-scoped
backlog. Mission, philosophy, and design standards were not found at scanned
locations. That does not mean no guidance exists: Autarch's `agents/` and
`docs/tui/` contain relevant guidance outside those conventions.

The [pilot foundation draft](../brainstorms/2026-09-04-autarch-foundation-draft.md)
therefore includes actual provisional mission/vision wording, reused card
context, an onboarding journey proposal, a rollout sequence, existing decision
and backlog links, and design-scope reconciliation. It does not ratify itself.

The workflow prepares onboarding; it does not automatically author or approve
each project's canonical foundation or launch agents. Human rulings on the
pilot and applying the workflow across projects remain the next work. Other
projects' canonical documents were not changed.
