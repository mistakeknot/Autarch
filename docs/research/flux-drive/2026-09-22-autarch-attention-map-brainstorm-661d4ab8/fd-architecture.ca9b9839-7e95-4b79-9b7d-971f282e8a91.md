### Findings Index

- P0 | F1 | "The service plus a BB plugin" | "Autarch holds no state" is already false in the existing code, and the brainstorm adds more state without reconciling the rule
- P0 | F2 | "What We're Building" | Mycroft's real dispatch substrate is tmux sessions, not BB threads — the brainstorm assumes a rewrite it never names as one
- P1 | F3 | "position means fixed regions" | The map's placement interface (`GraphSource`) doesn't exist in code — it's an unimplemented plan, not a dependency the map can read through today
- P1 | F4 | "The rail" | Two escalation queues: `escalate.DecisionQueue` (in-process, Go/TUI) vs. the rail's escalation queue (BB plugin) — no stated relationship
- P1 | F5 | "The rail doubles as the Mycroft composer" | Composer duplication against BB's native composer is unaddressed — what does Mycroft's composer do that BB's spawn/tell doesn't?
- P1 | F6 | "The service plus a BB plugin" | No data contract between the Go service and the TypeScript plugin — the Go structs that would feed it aren't exposed over any wire protocol yet
- P2 | F7 | "Ecosystem bands" / "Dependency edges" | CanonGraph's coverage gap (51/98 projects, 5 edges) is a known-open question carried forward from the layer-view plan without new mitigation at 2x the scale
- P2 | F8 | "Autarch door" | The door's existing threads screen (`internal/door/threads*.go`) duplicates ground the BB-plugin rail is about to cover, with no stated fate for the TUI

Verdict: needs-changes

## Summary

The boundary rule — BB owns threads, tracker owns tasks, Mycroft owns assignment, Autarch holds no state — is clean on paper but contradicted by the codebase that exists today, and the brainstorm's own feature list (pins, last-visit stamps, seen-glow, weighed alternatives, composer history) adds more of exactly the state the rule forbids, without saying which of these violate the rule and which don't. Separately, the brainstorm's central mechanism — "Mycroft coordinates agents as BB threads" — is not an extension of the current `internal/mycroft` package, which spawns and tracks agents purely through tmux sessions (`spawn/spawner.go`, `patrol/source.go`) with no BB integration anywhere in the tree. That's a rewrite of Mycroft's dispatch substrate, and it isn't named as scope. The map's placement source, `GraphSource`, is designed in `docs/plans/2026-09-03-layer-view-plan.md` but not yet built (`internal/door/graph.go` doesn't exist) — the brainstorm treats it as available. None of these findings say the shape is wrong; the map+rail-plus-lenses idea is coherent and well grounded in prior art. What's missing is the connective tissue: which state lives where, which existing package the new work replaces vs. extends, and what crosses the Go/TypeScript boundary.

## Issues Found

### 1. [P0] "Autarch holds no state" is already false, and the brainstorm doesn't reconcile it — "The service plus a BB plugin"

Evidence: `internal/door/preferences.go:16-31` defines `WorkbenchPreference` persisted to `~/.autarch/preferences.json` (`DefaultPreferencePath`), keyed by project. `internal/door/display.go:47-80` reads/writes a separate YAML preferences file. `internal/mycroft/escalate/escalate.go:62-68` holds an in-process `DecisionQueue` of `PendingDecision` — assignment-adjacent state that lives in Autarch's own process, not BB or the tracker.

Failure scenario: the brainstorm states the rule as a refusal ("No state owned by Autarch") and then lists five new state items the design needs (pin, last-visit stamp, seen-glow, Mycroft's weighed alternatives, composer history) with no per-item ruling on which are "preferences" (apparently allowed, per the `door.yaml`-pin analogy already in the doc) and which are "state" (apparently forbidden). Without that line, whoever writes the plan will either (a) put everything in a new Autarch-owned store, silently breaking the refusal the same way `preferences.json` already does, or (b) try to push things that don't fit anywhere else (Mycroft's weighed alternatives, seen-glow) into BB plugin storage, the tracker, or CanonGraph, forcing those systems to model UI-only concepts they have no business owning. Recommend the plan enumerate all five items with an explicit owner (BB plugin storage / Mycroft service / none-it's-derived) before write-plan, and either narrow "Autarch holds no state" to "no task/assignment/thread state" (excluding view preferences, which already have precedent) or admit the precedent is being extended.

### 2. [P0] Mycroft's dispatch substrate is tmux, not BB threads — the brainstorm assumes a rewrite it never scopes — "What We're Building"

Evidence: `internal/mycroft/spawn/spawner.go:15-53` (`ClaudeCodeSpawner.Spawn`) creates a `tmux new-session`; there is no `AgentSpawner` implementation anywhere that spawns or addresses a BB thread. `internal/mycroft/patrol/source.go:114-146` (`detectAgentsFromTmux`) detects agents by parsing `tmux list-sessions` names. `internal/tui/views/mycroft.go` wires `mycroft.DataSource` from `patrol` and dispatches through `scheduler.Dispatcher`, all tmux-session-shaped. A repo-wide grep for `"bb thread"`/`ThreadSpawn`/`bb-bridge` inside `internal/mycroft/` returns nothing.

Failure scenario: "Mycroft coordinates agents as BB threads" (line 27) reads as if it describes current behavior extended to a new UI, but it actually requires a new `AgentSpawner`/`DataSource` implementation that talks to BB's thread spawn/tell API instead of tmux, a new identity mapping (tmux session name -> BB thread ID), and a decision about what happens to the existing tmux-based tier system (T0-T3, `scheduler/`, `patrol/`) — keep both, retire tmux, or run them in parallel per-project. None of the Key Decisions or Open Questions name this. If write-plan proceeds without surfacing it, the estimate for "read-only attention and outcomes view" (item 8, first slice) will look small because it only needs read access to BB's `thread list --json`, but the next slice (approval-driven dispatch at T1) silently requires building a second `AgentSpawner` backend and reconciling it with the tmux one — a rewrite disguised as a UI feature.

### 3. [P1] `GraphSource` is a plan, not a dependency, and the map assumes it exists — "Position means fixed regions that you can nudge"

Evidence: `docs/plans/2026-09-03-layer-view-plan.md` WI-1/WI-2 specify `internal/door/graph.go` (`GraphSource`, `HTTPGraph`) and `internal/door/layers.go` (`ReadLayers`, `LayerRow`) in detail, but neither file exists in the tree (`ls internal/door/graph.go` -> no such file; repo-wide grep for `GraphSource` in `.go` files returns nothing). The plan itself is unimplemented.

Failure scenario: "the map's placement should read through" `GraphSource` (prior-art section) presumes a working, tested read path with SSE decoding, session handling, and a 2s timeout. None of that exists yet. If the attention-map plan is written assuming this interface is a stable foundation to build on, the actual dependency order is: land the layer-view plan's WI-1..5 first (including its own open question about layer speed semantics), then build the map on top. That ordering isn't stated anywhere in the brainstorm — Open Questions 1-2 discuss CanonGraph's data gaps but not that the reader itself is unbuilt.

### 4. [P1] Two escalation queues with no stated relationship — "The rail"

Evidence: `internal/mycroft/escalate/escalate.go:62-90` implements `DecisionQueue`, an in-process queue of `PendingDecision{Agent, BeadID, BeadTitle, Priority, Reasoning}` that the Bubble Tea `MycroftsView` (`internal/tui/views/mycroft.go:37`, field `decisions *escalate.DecisionQueue`) already renders and lets the operator approve/reject through the TUI.

Failure scenario: the brainstorm's rail is "the escalation queue (ordered by who is stalled) plus a Mycroft composer," rendered in the BB plugin. If the BB-plugin service builds its own escalation queue independent of `escalate.DecisionQueue`, mk gets two places dispatch decisions can be pending — the TUI's queue and the plugin's rail — with no reconciliation, and an approval in one won't clear the other. If instead the plugin's rail is meant to read `DecisionQueue` through the new Go service, that needs to be the stated design, and the TUI's independent approve/reject path needs a decision (retire it, or accept two front-ends over one state — which then also bears on Finding 1, since `DecisionQueue` is Autarch-owned in-process state).

### 5. [P1] Composer duplication against BB's own composer — "The rail doubles as the Mycroft composer"

Evidence: BB "provides plugin pages, thread panels, thread spawn and tell" per the project context; the brainstorm's rail composer is "styled after Cursor and BB" and used to bootstrap changes and new projects via `clavain:project-onboard`.

Failure scenario: it's unclear whether "the Mycroft composer" is (a) a thin wrapper that pre-fills BB's native compose box with a scope-derived prompt and hands off, or (b) a second text-composition surface Autarch renders itself inside the plugin page, parallel to BB's own thread-spawn UI. (b) means Autarch (or its plugin) is now maintaining input state (composer history, per line 25 in the doc list) that overlaps BB's own thread history — the plugin holding "composer history" is arguably plugin-local UI state, which is a smaller violation than Autarch's Go service holding it, but the brainstorm doesn't draw that line. Recommend stating explicitly that the composer is a scoped-prompt generator that calls BB's spawn/tell API and that any "history" is BB's thread history, read back, not a new store.

### 6. [P1] No data contract between the Go service and the TypeScript plugin — "The service plus a BB plugin"

Evidence: today, `pkg/fleet` (`FleetView`, `AgentView`, `BeadView`, etc., re-exported via `internal/mycroft/types.go:9-19`) is consumed only in-process by the Bubble Tea TUI (`internal/tui/views/mycroft.go`). There is no HTTP/JSON/gRPC surface in the tree that serves these types externally, and the brainstorm doesn't mention one.

Failure scenario: "an independent Autarch/Mycroft service" implies the Go side exposes an API a TypeScript BB plugin calls, but nothing in the design pins down the wire format, versioning, or which of `pkg/fleet`'s types are exposed as-is vs. reshaped for the map (region coordinates, lens data, escalation cards are all new shapes not present in `FleetView` today). Without this, "service + plugin" is two words, not an architecture — the plan needs at minimum a stated transport (REST? the existing `internal/tui` message-passing suggests nothing about network APIs) and a schema owner, or the two sides will drift the way `escalate.DecisionQueue` and any new rail state would (Finding 4).

## Improvements

- Add a state inventory table to the plan: for each of {pin, last-visit stamp, seen-glow, Mycroft's weighed alternatives, composer history, escalation queue}, name the owner (BB plugin storage / Mycroft Go service / derived-not-stored) and cite the precedent (`preferences.json`, `DecisionQueue`) it follows or diverges from.
- Before scoping the map, land (or explicitly re-plan) `docs/plans/2026-09-03-layer-view-plan.md` WI-1/WI-2 so `GraphSource`/`ReadLayers` exist and are tested; treat the map's read-only slice as strictly downstream of that plan rather than assuming the interface.
- Name the Mycroft-as-BB-threads work as its own slice with its own plan: a new `AgentSpawner`/`DataSource` pair alongside (or replacing) `spawn.ClaudeCodeSpawner`/`patrol.PatrolSource`, with a stated fate for the tmux-based tier system.
- Decide and state whether the door's TUI (`threads.go`, `threads_view.go`, the eventual layers screen) is retired, kept as a secondary front-end reading the same service, or left to diverge — don't let this fall out by omission once the BB plugin exists.
- Pin down the service/plugin transport and schema ownership in the same document that names "independent Autarch/Mycroft service," even at a one-paragraph level, before write-plan.

```
--- VERDICT ---
STATUS: warn
FILES: 0 changed
FINDINGS: 8 (P0: 2, P1: 4, P2: 2)
SUMMARY: The map+rail shape is coherent, but the "Autarch holds no state" boundary is already violated by existing code and left unreconciled, and the plan treats an unbuilt GraphSource and a from-scratch BB-thread dispatch substrate as if they already exist inside the current tmux-based Mycroft.
---
```
<!-- flux-drive:complete -->
