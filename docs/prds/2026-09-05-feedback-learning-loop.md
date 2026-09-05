---
artifact_type: prd
status: accepted-for-implementation
bead: Sylveste-fuwn
---

# Autarch feedback and learning pilot

Deliver [the review journey](../cujs/autarch-05-feedback-learning-loop.md) inside
Autarch's TUI with a macOS 26 SwiftUI companion. The
[accepted rulings](../brainstorms/2026-09-05-feedback-learning-loop.md) govern
experience, authority, continuity, retention and Interweave reuse.

The background controller owns versioned local review/agent records; the
companion owns selected-window ScreenCaptureKit capture and SpeechAnalyzer
transcription. Neither depends on TUI lifetime. Save snapshots for analysis
while recording continues. Retain original media and unapproved notes locally;
write accepted guidance into existing canonical project documents. Stable IDs,
project identity, references, timestamps and revisions cross every boundary.

Use Flere JSONL RPC with streaming, structured questions, cancellation,
reconnection and explicit context handoff. Investigation is restricted to
reading/research/proposals. Clavain submission must be project/tracker bound,
idempotent and respect scope, priority, dependencies, budgets and eligible
models through the actual worker launch. Expose selection and fallback reasons.

Project the full trace in a separate Interweave lattice SQLite cache, rebuilt
from source records, without changing other consumers' caches. Relationship
direction, source revisions, approval state and missing/deleted evidence must
survive rebuilding. Identity confidence never implies human acceptance.

Acceptance requires real capture and voice correction/playback; TUI reconnect;
failure retention; authority and duplicate prevention; real Flere question
round trips and stale/cancel handling; complete rebuilt trace; a human-accepted
proposal, actual changed/invoked build and guided human retest; and subsequent
application of accepted guidance without a reminder. Run relevant Go race,
Swift, lattice, Flere and Clavain checks plus repository gates. Component checks
are necessary evidence but never substitute for the real journey.

Alignment: human rulings become attributable constraints on later work.
Conflict/Risk: runtime availability, device permissions and human retest remain
live acceptance dependencies; report their state explicitly.
