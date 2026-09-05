---
artifact_type: plan
bead: Sylveste-fuwn
stage: implementation
---
# Project foundation onboarding

**Goal:** Make establishing a project's product foundation the next Autarch
workflow, covering mission, vision, philosophy, personas, CUJs, roadmap,
ADRs, backlog, and design systems/standards.

**Architecture:** Extend the existing project reader with bounded discovery of
conventional in-repo sources. Add a Foundation section and a portable onboarding
brief that starts from those sources and carries unresolved questions to the
user's chosen agent. Source files remain authoritative; the view is derived.

**Prior learnings:** The product-card format requires citations for drafts and
keeps drafted, declined, and confirmed distinct. File presence is inventory,
not project success. Existing onboarding uses root mission/philosophy files
and `docs/canon/personas.md`; support these alongside canon locations. Reuse
the product reader's path containment and non-regular-file checks, and the
dashboard's shared geometry. Alwe's session index was unavailable; current
repository decisions supplied the prior-art evidence.

## Observable result

- From project rows, `i` opens the HUD and `6` opens Foundation; its nine areas
  show sources found, not found in searched locations, empty, or unreadable.
- Card persona/CUJ references and existing roadmap/backlog sources are reused.
  ADR discovery works even without a card. Empty folders do not count as docs.
- `n` opens a project-specific onboarding brief; `c` copies it to the clipboard
  for the user's chosen agent. No provider is selected or agent started.
- The brief names existing evidence, asks concrete questions for the nine
  areas, carries declined-card needs, and requires proposals and explicit user
  decisions to stay separate. It links mission/persona/CUJ/roadmap/backlog work.
- Existing sources are read only. The scanner reports its search scope and
  limits; source presence never becomes an approval or completeness score.
- Refresh updates the foundation; Cozy/Compact and narrow layouts remain usable.

## Task 1: Foundation discovery and onboarding brief

Create `internal/door/foundation.go` and `foundation_test.go`; extend
`ProductBrief` and `ReadProductBrief` in `internal/door/product.go`.

1. Write failing fixtures for alternate canon/root paths, empty files/folders,
   missing/unread sources, escaping symlinks, ADRs without a card, and a live
   but empty backlog. Assert nine areas and no invented approval.
2. Implement `ReadFoundation(ProductBrief) []FoundationArea` and
   `BuildOnboardingBrief(ProductBrief) string`. Use at most 32 documents per
   area and the existing 256 KiB per-file limit; disclose partial discovery.
3. Test that the brief contains actual source paths, unresolved card needs,
   and project-specific context without writing any source.
4. Run `GOWORK=off go test -mod=readonly -race ./internal/door -run
   'TestFoundation|TestOnboarding|TestProduct' -count=1`; commit the reader.

## Task 2: Product workflow, verification, delivery

Modify `internal/door/product_view.go`, `model.go`, `dashboard.go`,
`display.go`, `cmd/autarch/project.go`, `README.md`, and product view tests.

1. Add Foundation as section 6, preserving the existing section numbers.
   Use the same product generation and refresh lifetime. Keep navigation
   reachable by keyboard when tabs exceed terminal width.
2. Add the onboarding brief view and explicit clipboard action, with a visible
   failure message if copying is unavailable. Esc returns to Foundation.
3. Verify all nine areas, brief entry/back, exact clipboard content, refresh,
   unchanged source files, and geometry at normal and narrow sizes.
4. Build and replay the real CLI in isolated tmux with fixture sources and a
   stub clipboard. Read the real Autarch foundation as a separate live check.
5. Review, run the affected race suite and full suite, commit, push, check CI,
   install the clean revision, and verify the invoked binary.

These tasks share the product model and execute sequentially. Broader rollout
and authoring each project's actual foundation remain tracked under the parent
feature; this first slice makes that work concrete and portable.
