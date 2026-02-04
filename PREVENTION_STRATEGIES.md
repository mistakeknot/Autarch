# Prevention Strategies: Avoiding Over-Engineered Fix Plans

**Problem:** Creating comprehensive multi-phase fix plans before verifying the bug exists, leading to wasted analysis effort when code is already functioning correctly.

**Example from Autarch:** 5-phase plan + 7 research agents → reviewers said "reproduce first" → code was already working.

---

## Prevention Strategies

### 1. **Reproduce Before Plan: The Zero-Phase Rule**

Create a **Phase 0** that must be completed before any planning begins:

#### Phase 0: Verification (Non-Negotiable)
- **Reproduce the bug** with explicit, repeatable steps
  - Provide exact command-line invocation or user interaction sequence
  - Include environment variables, config files, data setup needed
  - Document the "before/after" state visibly
- **Write a minimal test case** that demonstrates the failure
  - Unit test, integration test, or manual reproduction script
  - Test should currently fail and serve as acceptance criteria
- **Verify fix scope** by tracing through the code path
  - Does the bug exist where you think it does?
  - Is it one component or does it cascade?
  - Are there existing mechanisms that should have prevented this?
- **Check git history** for similar issues
  - Has this problem been reported/fixed before?
  - Are there comments in the code explaining why things are this way?
- **Communicate findings with one peer**
  - 5-minute code review: "I reproduced X at line Y because Z"
  - Peer confirms the bug is real, not user error
  - Peer helps spot if code already handles this case

**Acceptance Criteria for Phase 0:**
- Bug reproduction documented in a comment (or gist)
- Failing test committed or shown side-by-side
- No plan is written until Phase 0 is complete and documented

---

### 2. **Limit Planning Depth to Bug Severity**

Map plan complexity to the severity of the bug verified in Phase 0:

| Bug Type | Plan Depth | Phases | Research Agents | Review Rounds |
|----------|------------|--------|-----------------|---------------|
| **P0 (Production Outage)** | Deep | 5+ phases | 2–3 agents | Required |
| **P1 (Major Feature Broken)** | Medium | 3–4 phases | 0–1 agent | Recommended |
| **P2 (Edge Case, Workaround Exists)** | Shallow | 1–2 phases | 0 agents | Optional |
| **P3 (Paper Cut, Cosmetic)** | Minimal | 1 phase (inline fix) | 0 agents | Skip |
| **Unconfirmed** | STOP | Do not plan yet | Do not research | Do not create plan |

**Rule:** If you're creating a 5-phase plan for what might be a P3 bug, stop and verify severity first.

---

### 3. **Implement the "Code-Already-Works" Smell Test**

Before planning, manually check for these signals that the code might already handle the case:

**Code-level checks:**
1. Search for the exact error message / condition in the codebase
   - If it exists in an error handler, you might have found existing protection
   - If it exists in multiple places, convergence suggests the issue is considered important
2. Look for related `if` guards, type assertions, or nil checks
   - Pattern: `if x == nil { return error }`
   - Pattern: `if err != nil { ... }`
   - If guards exist around similar operations, the bug might already be gated
3. Check comments or TODOs near the suspected code
   - Look for phrases like "already handled", "safe because", "guards against"
4. Read the git blame for the suspicious line
   - If it was added recently with a comment, does the comment explain why?
   - If it was changed months ago, what was the previous version doing?
5. Run the code path in a debugger or add logging
   - Single-step through the suspected flow
   - Print variable values at the suspected failure point
   - The bug often disappears when you look closely

**Automated checks:**
- Run existing test suite: `go test ./...`
- Run type checker in strict mode: `go vet ./...`
- If a bug is real, there's often a failing test or warning somewhere

**If you find 3+ of these signals, create a minimal fix (not a plan) and test it first.**

---

## Recommended Checklist

Use this checklist **before** writing any fix plan:

### ✅ Pre-Planning Verification

- [ ] **Reproduce the bug with exact steps**
  - Command: `________________`
  - Expected: `________________`
  - Actual: `________________`
  - Reproducible? Yes / No / Sometimes (intermittent)

- [ ] **Locate the bug in source code**
  - File: `________________`
  - Function: `________________`
  - Lines: `________________`
  - Root cause (1 sentence): `________________`

- [ ] **Check if code already handles this**
  - [ ] Searched codebase for error message / condition → Found in: `________________` or "Not found"
  - [ ] Checked git blame for this line → Last change: `________________`
  - [ ] Looked for related guards / checks → Found: `________________` or "None found"
  - [ ] Ran `go vet`, type checker → Result: "Clean" or "Issues: `________________`"

- [ ] **Write a test that fails without the fix**
  - Test file: `________________`
  - Test name: `Test_________________`
  - Run status: "Fails before fix" / "Inconclusive"

- [ ] **Estimate scope (choose one)**
  - [ ] 1-file fix (< 20 lines)
  - [ ] 2-3 file fix (< 100 lines)
  - [ ] Multi-file refactor (> 100 lines)
  - [ ] Unknown / needs research

- [ ] **Assess risk (choose one)**
  - [ ] P0 (breaks production)
  - [ ] P1 (breaks major feature)
  - [ ] P2 (edge case, workaround exists)
  - [ ] P3 (cosmetic / nice-to-have)

- [ ] **Get one peer confirmation**
  - Peer name: `________________`
  - Peer confirmed: "Bug is real" / "Unclear" / "Already works"
  - Peer notes: `________________`

### ✅ Planning Phase (Only if verification succeeded)

- [ ] **Plan scope matches severity**
  - Severity: `P_` → Planned phases: `_` → Matches table above: Yes / No

- [ ] **Identify all affected code paths**
  - Primary path: `________________`
  - Secondary paths: `________________`
  - Tested paths: `________________`

- [ ] **Identify dependencies**
  - Code dependencies: `________________`
  - Config/data dependencies: `________________`
  - Test environment setup: `________________`

- [ ] **List all review criteria**
  - [ ] Reproducer test passes
  - [ ] No regressions in existing tests
  - [ ] Code follows project conventions
  - [ ] Documentation updated
  - [ ] Performance impact assessed (if relevant)

- [ ] **Plan fits in **one** session**
  - Estimated time: `________________`
  - Can implement + test + review in < 2 hours: Yes / No
  - If "No", break into smaller MRs / checkpoints

---

## Red Flags to Watch For

🚩 **Stop and re-verify if you see these warning signs:**

### Pre-Planning Red Flags

1. **"It should fail because…" instead of "It fails when…"**
   - You're theorizing about a bug, not confirming one
   - Action: Go reproduce it first with concrete steps

2. **Creating a plan before writing a reproducer test**
   - Plans are for implementation, not investigation
   - Action: Write a failing test first; plan after test confirms the bug

3. **Plan has 5+ phases and you haven't run the code yet**
   - You're over-preparing for unknown territory
   - Action: Test your Phase 0 and 1 assumptions in 20 minutes

4. **Multiple research agents / reviewers before reproduction**
   - Premature deepening of analysis
   - Action: Reproduce, then decide if you need help

5. **The bug description says "might be" or "possibly"**
   - Uncertainty = not ready to plan
   - Action: Add conditions to the title until you're certain

### During-Planning Red Flags

6. **Phases that say "investigate" or "determine"**
   - Phases should be concrete actions ("fix", "test", "deploy"), not research
   - Action: Move investigation work to Phase 0, only plan concrete fixes

7. **"Depends on how feature X works" in the middle of a plan**
   - You're planning around unknowns
   - Action: Resolve the dependency in Phase 0 before continuing

8. **Plan assumes a root cause you haven't verified**
   - "The issue is probably in module Y"
   - Action: Confirm the module before planning a multi-phase fix

9. **Different reviewers give contradictory feedback**
   - They're seeing different plans or unclear reproduction
   - Action: Go back to reproduction + simplify the plan

10. **Plan is longer than the code you're fixing**
    - Your plan is over-engineered
    - Action: Challenge every phase — can it be combined or removed?

### Post-Planning Red Flags

11. **Code is already working when you start implementing**
    - The bug report was outdated, or the reproduction was wrong
    - Action: Review Phase 0 — what was missed?
    - Learning: Add a git-blame check to your checklist for next time

12. **Reviewers say "delete phases" or "this is already done"**
    - Your plan included work that was already completed
    - Action: Before planning next time, search the codebase for related fixes

13. **Implementing Phase 1 changes the scope of Phases 2–5**
    - Unknowns were hiding in the plan
    - Action: Only plan 1–2 phases ahead; adapt as you learn

14. **The fix takes 10% of the planned time**
    - Classic sign of over-planning
    - Action: Review what assumptions were wrong; update your checklist

---

## Summary: Three Rules to Live By

### Rule 1: Reproduce Before Plan
**Don't plan a fix for a bug you haven't confirmed.** Phase 0 is mandatory. Write a test that fails.

### Rule 2: Limit Depth to Severity
**A P2 bug doesn't need 7 research agents and 5 phases.** Use the severity table to cap planning scope.

### Rule 3: Spot Uncertainty in Plans
**If your plan says "investigate", "depends on", or "might", you're not ready to plan.** Go back to Phase 0.

---

## Integration with Autarch Workflow

### For Gurgeh (PRD Generation)
When a suspected "spec bug" report comes in (e.g., "interview flow doesn't capture requirements"):
1. Run `gurgeh interview` manually with test data
2. Inspect the generated `.gurgeh/specs/` YAML file
3. Does it actually fail to capture the field? Or is the field there but not visible in the TUI?
4. Only then plan a fix

**Example:** Oracle review said "validation silently ignores errors" → Before planning 3 phases, test it:
```bash
echo 'title: ""' > test.yaml  # Invalid: empty title
go run ./cmd/gurgeh validate test.yaml  # Does it fail or pass?
```

### For Coldwine (Task Orchestration)
When reviewing task rejection flow issues:
1. Create a test project with tasks in specific states
2. Attempt the suspected failure scenario
3. Capture the error message (or absence of error)
4. Read the code path in the debugger
5. Then write a plan

### For Coldwine → Gurgeh Integration
When a cross-tool bug is suspected:
1. Run one tool in isolation first (e.g., Gurgeh alone)
2. Then add the second tool (e.g., Coldwine)
3. At each step, confirm the bug exists or disappears
4. Only then plan a fix that spans both tools

---

## Example: Applying These Rules to the Oracle Review

**Situation:** Oracle review flagged 6 issues in Gurgeh arbiter. Initial response: "Create a 5-phase plan, run 7 research agents."

**Correct approach:**
1. **Phase 0 — Reproduction:**
   - Issue 3 (State pointer escape): "Write a test that triggers concurrent mutations"
   - Issue 4 (Research wiring incomplete): "Run a sprint where research is triggered, verify it executes"
   - Issue 5 (Scan timing): "Trace through phases, verify when quick scan actually runs"
   - Stop planning until you've confirmed which issues are *actually* bugs vs. "works as designed"

2. **Severity assessment:**
   - Pointer escape + concurrent mutation = **P0** (data corruption risk)
   - Research wiring unused = **P2** (feature not fully integrated, but not broken)
   - Scan timing drift = **P3** (optimization issue, not a failure)

3. **Plan depth:**
   - P0: 2–3 phases, 1 research agent (mutex/actor pattern expert)
   - P2: 1 phase, 0 agents (straightforward wiring)
   - P3: Defer (not critical)

4. **Result:**
   - Focused plan: Issue #3 + #4 only
   - No 7 agents needed; 1 focused review sufficient
   - When implemented, code was already largely correct (pointer returns, research wiring was just incomplete, not broken)

---

## For Code Review and Merge Requests

When a reviewer says:
- "Reproduce first" → This is a Phase 0 request. Don't proceed to phases 1+ yet.
- "Delete phases 2–5" → Your plan assumed facts not in evidence. Simplify.
- "58% of this should be deleted" → You over-planned. Recalibrate severity.

**Reviewer signpost:** If 50%+ of a plan is deleted in review, your pre-planning process missed key inputs. Add one more step to your checklist before planning next time.

---

## Resources

- **Related Autarch docs:**
  - `/docs/oracle-architecture-review-2026-02-01.md` — Example of what reviewers flag (use for verification)
  - `/docs/reviews/2026-02-01-oracle-prd-interview-review.md` — Example of "quick wins" vs. "structural improvements" categorization

- **Next step:** Add a Phase 0 checklist to your issue templates / PR templates so every bug report includes reproduction steps before any planning begins.
