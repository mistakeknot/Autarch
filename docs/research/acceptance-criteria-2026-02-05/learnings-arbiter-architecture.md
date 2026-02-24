# Learnings: arbiter architecture

**Source:** Agent result from session b817bfb0
**Task ID:** a929278
**Date:** 2026-02-05

---

## Analysis: Arbiter Architecture Patterns vs. Acceptance Criteria Plan

The arbiter architecture patterns learning document is **highly relevant** to the acceptance criteria plan. The plan already incorporates several insights from this learning, but there are specific areas where the relationship is direct and some residual gaps. Here is the detailed analysis across the four dimensions asked:

---

### 1. Does the plan account for import cycle constraints?

**Partially, but indirectly.** The plan does not explicitly mention the import cycle pattern (local adapter sub-packages under `arbiter/`), but it does reference the internal architecture components correctly. For example:

- AC-1.8 references "four components (Completeness, Consistency, Specificity, Research)" which maps to the `confidence/calculator.go` and `consistency/engine.go` sub-packages documented in the architecture learning.
- AC-1.14 references "GoalFeature consistency check" which is one specific check type from the consistency engine.

**Gap:** The plan's Dependencies section at the end lists `go test -race` as mandatory but does not mention that the arbiter's orchestrator, consistency, and confidence sub-packages have a specific dependency graph constraint. If someone were to implement new acceptance criteria (e.g., adding more consistency check types beyond GoalFeature), they would need to follow the adapter pattern described in the learning:

> "Created lightweight sub-packages under `arbiter/` that define local types... The orchestrator converts between adapter types and arbiter types via helper methods (`checkConsistency`, `updateConfidence`)."

**Recommendation:** In the Dependencies section, add a note:

> New arbiter sub-packages (e.g., additional consistency check types beyond GoalFeature) must follow the local adapter sub-package pattern documented in `docs/solutions/patterns/arbiter-spec-sprint-architecture.md` to avoid Go import cycles between the orchestrator and its engines.

---

### 2. Do the hunter API assumptions match the actual API?

**Yes, the plan correctly accounts for this.** The learning documents a critical divergence:

> "Plan assumed `HuntResult.Items []Item` field. Actual API: `HuntResult.OutputFiles []string` (YAML file paths). Hunters write results to YAML files and return paths."

The acceptance criteria plan avoids making direct assumptions about the `HuntResult` struct. AC-1.12 says:

> "Phase-appropriate hunters trigger automatically... verify hunter selection in log pane"

This is observation-based (watching log output), not API-structure-dependent. AC-1.16 about rate-limited hunters also doesn't assume a specific data structure. The plan's Research Insights section further states:

> "Only 4 of 8 phases have research configs in `research_phases.go`"

This shows the plan authors explored the actual code rather than relying on assumptions. The Research Coverage metric was revised downward to ">60% (4 of 8 phases have configs)" in the Timing Thresholds Summary, which is consistent with the learning's warning about plan-to-implementation drift.

**No gap here.** The plan learned from this pattern.

---

### 3. Does the plan account for the arbiter's internal architecture (orchestrator, confidence calculator, consistency engine)?

**Yes, and in detail.** The plan maps directly to the three sub-packages documented in the architecture learning:

| Architecture Component | Acceptance Criteria |
|---|---|
| `confidence/calculator.go` | AC-1.8 (four-component score), AC-1.9 (research coverage impact on score) |
| `consistency/engine.go` | AC-1.14 (GoalFeature consistency check) |
| `quick/scanner.go` | Not directly tested, but AC-1.1 references "Kickoff completes codebase scan" which is the quick scan functionality |

The plan also correctly notes in its Out for v1 section:

> "only GoalFeature ships" (referring to consistency check types)

This aligns with the architecture learning's mention that multiple consistency check types exist but the orchestrator routes to them selectively.

The plan's "Institutional Learnings Applied" section explicitly references related arbiter patterns:

> "Race condition testing mandatory (from `arbiter-state-pointer-escape`): All arbiter tests must run with `go test -race`. The `State()` method returns deep-copied snapshots to avoid data races with TUI refresh (~60 FPS)."

> "Error message routing (from `swallowed-generation-error-msg`): Parent views must pass error messages to children. `GenerationErrorMsg` from arbiter must reach SprintView's chat panel."

> "Phase propagation (from `spec-propagation-consistency-pattern`): When user edits a phase via chat, dependent phases should update automatically. This behavior exists but has no AC."

**Minor gap:** The third point -- phase propagation has no acceptance criterion. The learning about the consistency engine implies that when spec content changes, consistency checks should re-fire. The plan should add an AC like:

> AC-1.14a: When a user edits Vision or Problem content via the chat pane, GoalFeature consistency re-evaluates within 2 seconds and surfaces any new conflicts without requiring manual re-trigger.

---

### 4. Are there plan assumptions that contradict the documented architecture?

**One subtle contradiction, one alignment risk:**

**Contradiction -- Color API:** The plan's AC-1.5 says:

> "Pollard 3-pane layout renders: sidebar (Inbox/Accepted/Rejected/Deferred + hunter status), doc pane (finding detail), agent pane (triage conversation)"

This is a visual/layout criterion, but the learning documents:

> "Plan referenced `tui.TokyoNight.Cyan`, `tui.TokyoNight.Blue`, etc. Actual: `tui.ColorPrimary`, `tui.ColorSecondary`, `tui.ColorSuccess`, etc."

The acceptance criteria plan does not reference specific color constants (correctly avoiding this trap), but implementers working from the CUJ descriptions might. The CUJ-1 description mentions "⚔ competitive, research_trend, user icons" and "warning color" -- these are conceptual, not API-level, so they're safe. No direct contradiction in the plan itself.

**Alignment risk -- Quick scan phase timing:** The architecture learning documents that the quick scanner is one of three arbiter sub-packages. The plan's Research Insights section correctly identified:

> "Codebase scan (AC-1.1) target of <10s is unrealistic. The actual `exploration.Explore()` function shells out to Claude Code with a comprehensive prompt. Current timeout is set to 10 minutes."

The plan then recommends splitting into "structural scan (<5s, local file walk) and LLM exploration (<90s with streaming partial results)." This aligns with the architecture learning's `quick/scanner.go` which parses hunter output files (local, fast) versus the full LLM exploration. The Timing Thresholds Summary table reflects this split. No contradiction.

---

## Summary

The arbiter architecture patterns learning is **directly applicable** to this acceptance criteria plan. The key insight to quote:

> "Before implementing cross-package orchestrators: Map the dependency graph first. If package A's types are used by B, C, D, and A needs to call B, C, D -- use adapter patterns or interfaces."

The plan already incorporates most lessons from this learning (hunter API reality, color API reality, race condition testing). The two places where the learning should be explicitly referenced in the plan are:

1. **Dependencies section**: Add a note about the adapter sub-package pattern for any new arbiter sub-packages (consistency check types beyond GoalFeature, new confidence components).

2. **Acceptance Criteria AC-1.14**: Add AC-1.14a for automatic re-evaluation of consistency when spec content is edited via chat, matching the phase propagation pattern the plan itself identified as having "no AC" (line 288 of the plan).

Both are minor additions. The plan's authors clearly studied the learnings directory already -- the "Institutional Learnings Applied" section cites three related arbiter patterns and the Research Insights section reflects awareness of actual API shapes. The architecture learning's prevention guidelines are well-absorbed into this plan.