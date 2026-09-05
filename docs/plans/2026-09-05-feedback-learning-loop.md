# Feedback and learning loop implementation

User-authorized plan, tracked by `Sylveste-fuwn`. The
[PRD](../prds/2026-09-05-feedback-learning-loop.md),
[CUJ](../cujs/autarch-05-feedback-learning-loop.md) and
[ruling record](../brainstorms/2026-09-05-feedback-learning-loop.md) preserve the
submitted plan's scope. Existing local changes in the original checkout are
outside this work; implementation uses an isolated checkout of `main`.

Delivery order:

1. Durable shared records, local controller IPC, selected-window native capture,
   voice originals/correctable transcript, TUI workbench, evidence playback and
   reconnect. Verify saved feedback and recording lifetime independent of TUI
   and agents, including write/interruption failures.
2. Foundation discovery with source/history coverage; Flere background
   investigation, one project conversation, questions, explicit handoffs,
   proposal review and revision-bound acceptance. Verify the read-only boundary,
   question round trips, immediate progression and stale/cancelled requests.
3. Project-bound idempotent Clavain submission and governed worker routing;
   Interweave lattice feedback connector and Beads dependency ingestion. Verify
   deferred priorities/dependencies, execution states and rebuildable trace.
4. Build and invoke the pilot, complete the real human feedback → acceptance →
   execution → build-bound retest journey, then demonstrate guidance applied to
   later related work. Record actual binary identity and all remaining gates.

Implementation choices within this approved architecture may proceed without
reopening settled product decisions. Captures/private session material never
enter Git. Acceptance does not authorize unseen scope or later proposal edits.

Alignment: preserves source authority while connecting review to execution.
Conflict/Risk: a passing implementation without the human/device acceptance
journey remains incomplete.
