# Over-Planning Detector: Quick Reference Guide

**Use this during planning or code review to catch over-engineering in real-time.**

---

## The 14-Point Over-Planning Checklist

### RED FLAGS (Stop immediately and reconsider)

🚩 **#1: Plan before reproducer**
- [ ] I created a fix plan before writing a test that fails
- **Fix:** Write the failing test first. That's your proof the bug is real.

🚩 **#2: More than 3 research agents**
- [ ] Plan includes research agents: 4, 5, 6, 7, ...
- **Fix:** For P1 bugs, max 2 agents. For P2, zero agents.

🚩 **#3: 5+ phases for a P2/P3 bug**
- [ ] Severity is P2 or P3, but plan has 4–5 phases
- **Fix:** Use severity table to cap phases. P2 = 1–2 phases. P3 = inline fix.

🚩 **#4: "Investigate" or "Research" as a phase name**
- [ ] Plan says: "Phase 2: Investigate module X" or "Phase 3: Research implications"
- **Fix:** Investigation happens in Phase 0. Phases 1–N should be concrete actions.

🚩 **#5: Uncertain root cause in the plan**
- [ ] Plan assumes: "The issue is probably in X" or "likely caused by Y"
- **Fix:** Verify the root cause before planning. Uncertainty = extend Phase 0.

🚩 **#6: Plan is longer than the code**
- [ ] Fix involves 50 lines of code but plan is 3 pages long
- **Fix:** Delete 50% of the plan. If you can't delete phases, they're not needed.

🚩 **#7: Multiple contradictory reviewer comments**
- [ ] Reviewers disagree on scope, priority, or approach
- **Fix:** Simplify the plan and re-verify Phase 0. Complexity breeds disagreement.

🚩 **#8: "Depends on whether X works"**
- [ ] Middle of the plan: "If X is true, do A. If X is false, do B."
- **Fix:** Determine X in Phase 0. Plan assumes only one path.

---

### YELLOW FLAGS (Investigate further)

⚠️ **#9: More than 2 unknown dependencies**
- [ ] Plan lists: "depends on config Y", "depends on module Z", "depends on API W"
- **Action:** Resolve at least 2 of these in Phase 0 before proceeding.

⚠️ **#10: Plan requires architectural changes**
- [ ] Plan says: "Refactor module X" or "Add new abstraction Y"
- **Action:** Is the refactor necessary to fix the bug? Or just nice-to-have?
  - If necessary → include in the plan
  - If nice-to-have → separate into a follow-up MR

⚠️ **#11: Phases that say "and" (combining concerns)**
- [ ] Phase title: "Implement fix AND refactor tests AND update docs"
- **Action:** Split into separate phases. Each phase should have one primary goal.

⚠️ **#12: Plan has >5 review criteria**
- [ ] Phase says: "tests pass, no perf regression, docs updated, style matches, code coverage >90%, ..."
- **Action:** Keep top 3 criteria. Others are defaults.

⚠️ **#13: Uncertainty between plan and implementation**
- [ ] Peer says: "I'd approach this differently" on Phase 1
- **Action:** You made assumptions about implementation that weren't in the plan. Plan less detail.

⚠️ **#14: Plan survives less than 24 hours**
- [ ] You wrote the plan, and >50% is discarded in the first review
- **Action:** Add "phase 0 validation" step to your next plan.

---

## Scoring System

**Count your flags:**

- **0–2 red flags:** ✅ Plan is reasonable. Proceed.
- **3–4 red flags:** ⚠️ Simplify the plan. Delete 30% of phases.
- **5+ red flags:** 🚫 Cancel this plan. Return to Phase 0.

- **0–1 yellow flags:** ✅ Acceptable.
- **2–3 yellow flags:** ⚠️ Likely to need adjustment during implementation.
- **4+ yellow flags:** 🚫 Reconsider plan structure.

---

## Quick Self-Test

**Answer these YES or NO:**

1. Can I reproduce this bug in < 5 minutes? **YES / NO**
   - If NO, extend Phase 0.

2. Can I explain the root cause in 1 sentence? **YES / NO**
   - If NO, you don't understand the bug yet.

3. Is my plan < 3 phases? **YES / NO**
   - If NO, is severity P0? If not, delete phases.

4. Have I written a failing test? **YES / NO**
   - If NO, do that before planning.

5. Can I implement Phase 1 in < 1 hour? **YES / NO**
   - If NO, Phase 1 is too big.

---

## The "Autarch-73j" Anti-Pattern

**What happened:** 5-phase plan, 7 research agents, 3 reviewers all said "reproduce first" → Code was already working.

**How to avoid it:**

- ❌ Don't: "The arbiter orchestrator might have race conditions. Let me plan 5 phases to fix them."
- ✅ Do: "Run concurrent tests. Do they fail? No? Then no race condition. Done."

- ❌ Don't: "Research integration might be incomplete. Let's plan a deep refactor."
- ✅ Do: "Test the research integration. Does it work? Yes? Then it's not a bug, file a follow-up."

- ❌ Don't: "Create a plan with 7 research agents to understand the scope."
- ✅ Do: "Read the code for 30 minutes. Write a 1-page summary. Then decide if you need help."

---

## Phrases That Signal Over-Planning

**If your plan includes these, reconsider:**

1. "might", "possibly", "likely", "probably" → Replace with facts. Verify Phase 0.
2. "investigate", "research", "determine" → Do this in Phase 0, not in a phase.
3. "depending on whether X" → Determine X first.
4. "could cause", "may require", "might need" → De-risk before planning.
5. "future work" → That's a follow-up MR, not this plan.
6. "architectural change" → Is it required to fix the bug? Or nice-to-have?
7. "and" (combining multiple concerns) → Split into separate phases.

---

## The Simplicity Test

**Read your plan out loud to a rubber duck (or a peer).**

If they ask:
- "Why didn't you test that first?" → You skipped Phase 0.
- "But doesn't the code already handle that?" → You didn't search the codebase.
- "Can you delete phases 3–5?" → You over-planned.
- "What happens if assumption X is wrong?" → You're making untested assumptions.
- "Is this even a bug?" → Go back and reproduce.

**Any of these questions = reconsider the plan.**

---

## Post-Implementation: Learning from Over-Plans

**After a plan that was cut by >30%, ask:**

1. What did I get wrong in Phase 0?
   - Did I assume complexity that wasn't there?
   - Did I skip a reproducer test?
   - Did I misread the code?

2. What's missing from my checklist?
   - Add one item to Phase 0 based on what you missed.

3. Can I automate this check?
   - E.g., "Always search git blame for this function before planning"
   - E.g., "Always run the code path in debugger first"

4. Did reviewers give the same feedback?
   - If 3 reviewers say "delete phase X", phase X was never needed.

---

## Integration with Autarch Workflow

### In Code Review

**Reviewer spotting over-planning:**
```
Comment: "Let's verify Phase 0 first:
- Does the interview flow actually fail to capture requirements?
- Can you show me the generated YAML?
- Is this blocking users or just cosmetic?

Once Phase 0 is confirmed, I'll review the plan."
```

### In Planning Meetings

**Project lead seeing 7 research agents:**
```
"Before we schedule research, let's:
1. Reproduce the issue in 10 minutes
2. Search the codebase for similar fixes
3. Sketch a 1-phase minimal fix
4. Then decide if research is needed"
```

### In Issue Triage

**Issue template enhancement:**
```markdown
## Reproduction Steps
- [ ] I can reproduce this with: ___
- [ ] Test case: ___
- [ ] Expected behavior: ___
- [ ] Actual behavior: ___

**Only after filling these out, create a fix plan.**
```

---

## Template: Simplified Plan After Over-Planning Detection

**Original plan:** 5 phases, 7 agents, 3 reviewers arguing

**Revised plan:**

```markdown
## Phase 0 (Already Complete)
- [x] Reproduced: Run `gurgeh interview --config test.yaml`, verified spec.Status is empty
- [x] Root cause: buildSpecFromInterview() doesn't set Status field (line 47)
- [x] Already fixed? No, git blame shows this was added 3 weeks ago and never set
- [x] Peer confirmed: Yes, reviewer confirmed bug is real

## Phase 1 (Implementation)
1. Set `Status: "draft"` in buildSpecFromInterview()
2. Add test: TestInterviewSpecHasStatus
3. Verify: `go test ./cmd/gurgeh/... -v`

## Phase 2 (Integration)
- Verify downstream tools (Coldwine, Bigend) handle "draft" status correctly
- Run integration tests

Total time: 1 hour
Research needed: None
```

---

## Resources

- **PREVENTION_STRATEGIES.md** — Full framework with examples
- **PHASE_0_CHECKLIST.md** — Detailed checklist for bug verification
- **Recent example:** `/docs/oracle-architecture-review-2026-02-01.md` (see how Oracle review led to over-planning initially)

---

**Bottom line:** If you count >5 red flags, your plan is over-engineered. Return to Phase 0 and verify the bug exists before proceeding.
