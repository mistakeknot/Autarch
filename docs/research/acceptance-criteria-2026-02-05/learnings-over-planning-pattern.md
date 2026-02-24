# Learnings: over-planning pattern

**Source:** Agent result from session b817bfb0
**Task ID:** a89b41e
**Date:** 2026-02-05

---

I now have a thorough understanding of both documents and the actual codebase state. Here is my analysis.

---

## Analysis: Does "Over-Planning Before Reproduction" Apply to the Acceptance Criteria Plan?

**Verdict: Yes, the learning is highly relevant. The acceptance criteria plan exhibits a structurally similar anti-pattern, but at the feature level rather than the bug level.**

### The Key Insight from the Learning

> **"Reproduce before planning. The reviewers were right -- 58% of elaborate planning was unnecessary."**

And more specifically:

> **"We skipped Phase 0."** The plan document itself said: "Phase 0: Reproduce First (NEW - REQUIRED)" ... But we wrote Phases 1-5 before executing Phase 0.

The learning document codifies this into a rule:

> **Before writing ANY fix plan, complete Phase 0:** Write exact reproduction steps, execute reproduction steps, write a failing test, check git blame, assess severity, get peer confirmation -- ONLY THEN plan phases 1-N.

### How the Anti-Pattern Manifests in the Acceptance Criteria Plan

The original learning was about planning bug fixes for bugs that might not exist. The acceptance criteria plan has the analogous problem: **it defines acceptance criteria for features that do not yet exist, without first verifying what the current codebase can actually do.** This is "over-planning before reproduction" applied at the feature-specification level rather than the bug-fix level.

Here are the specific instances:

#### 1. Criteria for completely non-existent features (no code path to test)

These acceptance criteria cannot be verified because the underlying feature has zero implementation:

- **AC-1.3, AC-1.4** (badge pulse, `Pollard (N)` count): No badge component exists. No Pollard tab TUI exists. Zero Go files match "badge" or "pulse" in combination with Pollard.
- **AC-1.5** (Pollard 3-pane layout with Inbox/Accepted/Rejected/Deferred): No Pollard TUI tab exists. The grep for `pollard.*triage`, `finding.*inbox`, `pollard.*3.pane` returned zero Go files.
- **AC-1.6, AC-1.7** (agent pane triage, edit preview): No `feedback.yaml` mechanism exists anywhere in Go code. The only reference is in the plan document itself.
- **AC-1.9** (research component reflects review coverage of unreviewed findings): The confidence calculator exists at `internal/gurgeh/arbiter/confidence/calculator.go`, and it does accept a `researchQuality` float64 -- but there is no mechanism that feeds "unreviewed finding count" into this parameter. The parameter is a simple 0-1 float with no finding-awareness.
- **AC-1.17** (`autarch_list_findings`, `autarch_triage_finding` MCP tools): These MCP tools do not exist. The grep for these tool names returned only the plan document.
- **AC-3.6 through AC-3.9** (feedback.yaml preference learning, global preferences, rolling window): `feedback.yaml` does not exist as a concept in any Go file. `.pollard/feedback.yaml` is never read or written. `~/.autarch/pollard-preferences.yaml` is entirely imaginary.
- **AC-3.4a** (signal deduplication on `(spec_id, type, affected_field)` tuple): As the plan itself acknowledges in Gap 3, the broker is a pure fan-out with no deduplication. But the acceptance criterion is written as if this feature should be verified, not built.
- **AC-4.7** (`autarch_reserve_paths`, `autarch_release_paths` MCP tools): Do not exist.
- **AC-4.8** (Coldwine acquires Intermute reservations when teammate claims task): No Agent Teams integration exists in Coldwine. The grep for `AgentTeams|agent.teams` in `internal/coldwine` returned zero files.
- **AC-2.8, AC-2.9** (Agent Teams shared task list, plan approval gating): No Agent Teams bridge code exists in Coldwine.

#### 2. Criteria that assume implementation details for unbuilt bridges

- **AC-4.2** ("Overlapping reservation request rejected"): The plan itself documents in Gap 1 that Intermute's `Reserve()` does a simple INSERT with no overlap check. This criterion is testing a mechanism that provably does not exist.
- **AC-5.7** (Bigend displays Agent Teams structure): No code reads `~/.claude/teams/` for team structure display.

#### 3. The plan's own "Research Insights" section is self-contradictory

The plan identifies **four critical implementation gaps** (lines 189-211) where features don't exist:
- Gap 1: Intermute `Reserve()` has no glob overlap detection
- Gap 2: Coldwine-to-Agent-Teams bridge mechanism is unspecified
- Gap 3: Signal deduplication is purely in-memory with no persistence
- Gap 4: Signal transport path has no bridge between Signals and Intermute

Yet the acceptance criteria table above those gaps *defines testable criteria as if those gaps don't exist*. The plan diagnoses the disease in its appendix but writes the prescription as if the patient is healthy.

### The Parallel to the Learning's Red Flags Table

The learning document provides a "Red Flags for Over-Engineering" table. Three flags directly apply:

| Red Flag | From Learning | How It Applies Here |
|----------|--------------|---------------------|
| Plan before reproducer test | "You don't know if the bug exists" | You don't know if the feature exists -- and for many criteria, it provably doesn't |
| "Investigate" as a phase name | "You should investigate BEFORE planning" | The plan has 10 open questions and 4 known gaps, but still defines 60+ acceptance criteria |
| Plan is longer than the fix will be | "Over-engineering" | 430-line acceptance criteria plan for features where the implementation doesn't exist yet |

### What a "Phase 0: Verify Existing Behavior" Would Look Like

Following the learning's corrected process (`Report -> Reproduce -> If reproducible -> Minimal Plan -> Fix -> Test`), the acceptance criteria plan should have included a Phase 0 that:

1. **Audits what actually exists today.** Run each tool (`./dev autarch tui`, `go run ./cmd/pollard scan`, `go run ./cmd/gurgeh serve`) and document what works end-to-end right now. For each CUJ, note which steps are possible today and which steps hit a wall.

2. **Categorizes criteria into tiers:**
   - **Tier A (Verifiable Now):** The feature exists; the acceptance criterion is testing existing behavior. Examples: AC-1.10 (spec export to `.gurgeh/specs/`), AC-1.15 (spec versioning -- `SaveRevision` exists in `evolution.go`), AC-X.1 (servers bind to loopback), confidence calculator basics.
   - **Tier B (Partially Exists):** Some code exists but the full flow is not wired. Examples: AC-1.14 (GoalFeature consistency -- `consistency/engine.go` exists), AC-1.12 (research phases -- `research_phases.go` exists but only 4 of 8 phases are configured).
   - **Tier C (Does Not Exist):** No code path exists. The criterion is speculative. Examples: All feedback.yaml criteria, all Agent Teams bridge criteria, all MCP tool criteria, all Pollard TUI criteria, badge/pulse criteria.

3. **Defers Tier C criteria** until the underlying feature is at least partially implemented. Writing acceptance criteria for imaginary features is the feature-planning equivalent of writing a 5-phase bug fix plan before reproducing the bug.

### Where to Incorporate This in the Plan

The plan should add a section immediately after "Prior Context" and before "Acceptance Criteria":

**Phase 0: Codebase Verification (MANDATORY before writing AC)**

For each CUJ, answer:
- Can I exercise this CUJ end-to-end today?
- Which steps work? Which steps fail?
- For steps that fail: is it because the code doesn't exist, or because it's broken?
- For code that doesn't exist: defer the acceptance criterion to the implementation plan, not the testing plan.

Then restructure the acceptance criteria into "verifiable now" vs. "verifiable after implementation."

### Summary

The over-planning-before-reproduction learning applies directly. The acceptance criteria plan defines 60+ criteria, at least 25 of which test features that have zero implementation. This is the same structural error as the original learning -- elaborate planning layered on top of unverified assumptions about what the system can do. The plan even identifies 4 critical gaps in its own research section but proceeds to define criteria as if those gaps are already filled.

The cost calculation from the learning is instructive:

> | Activity | Time Spent | Value Delivered |
> |----------|------------|-----------------|
> | Initial plan creation | ~1 hour | 10% (only Phase 0 was useful) |
> | Research agent deepening | ~30 min | 0% (all speculative) |

The acceptance criteria plan was deepened by 11 research agents. The research insights section (Gaps 1-4, performance analysis, security findings) is genuinely valuable. But the acceptance criteria themselves for non-existent features deliver no testing value until the features are built -- and by then, the criteria will likely need revision based on what was actually implemented.