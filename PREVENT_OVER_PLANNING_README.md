# Preventing Over-Engineered Plans: Quick Start

**Problem Solved:** Autarch-73j demonstrated how creating comprehensive fix plans before verifying bugs exist leads to wasted effort. This guide prevents that.

**Key insight:** Create Phase 0 (verification) before planning Phases 1–N. Three reviewers all said "reproduce first."

---

## Three Documents, Three Purposes

### 1. **PREVENTION_STRATEGIES.md** — The Full Framework
**Read this first.** Comprehensive guide covering:
- The "Zero-Phase Rule": Phase 0 is mandatory
- Plan depth mapping (severity → # phases)
- Code-already-works smell test
- Detailed red flags with explanations
- How to apply these rules to Autarch tools

**When:** Design your planning process, train new contributors
**Time:** 20 minutes to read

---

### 2. **PHASE_0_CHECKLIST.md** — The Execution Checklist
**Use this for every bug.** Step-by-step checklist:
- Quick 5-minute version
- Full 15–20 minute version
- Exact reproduction steps format
- Peer confirmation template
- Example: "Validation Silently Ignores Errors" (from Oracle review)

**When:** Starting work on a reported bug
**Time:** 5–20 minutes to complete (depending on bug complexity)

---

### 3. **OVERPLANNING_DETECTOR.md** — Real-Time Detection
**Use this during planning or code review.** Quick signals:
- 14 red/yellow flags to spot over-planning immediately
- Scoring system (0–2 flags = OK, 5+ = cancel plan)
- Quick self-test (5 YES/NO questions)
- Phrases that signal over-engineering
- Integration with Autarch code review process

**When:** Writing a plan, reviewing someone else's plan, code review
**Time:** 2 minutes to scan, 5 minutes to apply scoring

---

## How to Use These Together

### Scenario A: You've Found a Bug

**Step 1:** Open **PHASE_0_CHECKLIST.md**
- Work through the 15–20 minute checklist
- Write a failing test
- Get peer confirmation
- Do NOT proceed without completing this

**Step 2:** Open **PREVENTION_STRATEGIES.md**
- Use the severity table to determine plan depth
- Check the "Code-Already-Works Smell Test"
- Only then begin planning

**Step 3:** Open **OVERPLANNING_DETECTOR.md**
- While writing your plan, scan for red flags
- Count them (scoring system)
- If 5+, cancel and simplify

---

### Scenario B: You're Reviewing Someone's Plan

**Step 1:** Ask: "Is Phase 0 complete?"
- Reproduction steps documented?
- Failing test included?
- Peer confirmed?

**Step 2:** Open **OVERPLANNING_DETECTOR.md**
- Count the red flags in their plan
- If 5+, ask them to simplify and re-verify Phase 0

**Step 3:** Comment:
```
"I see phases 3–5 might not be needed. Let's:
1. Complete Phase 0 with reproduction + test
2. Implement Phase 1 in a single MR
3. Re-evaluate phases 2–5 after we see what's actually needed"
```

---

### Scenario C: You're Planning a Large Feature/Refactor

**Principles still apply:**
- Phase 0 = "What's the current state? Why is it a problem?"
- Phase 1 = "What's the minimal fix?"
- Phases 2–N = "What else needs to change?"

Example: "Validation silently ignores errors" (from Oracle review)
- Phase 0: Test it. Does validation actually ignore errors? Yes.
- Phase 1: Fix writeSpec() to report errors. Add test.
- Phase 2 (optional): If users complain about new error messages, add context/recovery.
- Don't plan Phase 2 until Phase 1 is done.

---

## Red Flags Cheat Sheet

**Stop immediately if:**
- Plan created before a failing test exists
- 5+ phases for a P2/P3 bug
- "Investigate" or "Research" is a phase name
- Plan is longer than the code
- Root cause is "probably" or "likely" (not verified)

**See OVERPLANNING_DETECTOR.md for full list.**

---

## Integration with Existing Processes

### Code Review Checklist
Add to your MR template:
```markdown
## Pre-Planning Verification
- [ ] Reproduction steps documented
- [ ] Failing test included
- [ ] Phase 0 checklist completed
- [ ] Plan severity matches fix scope
```

### Issue Triage
When receiving a bug report:
1. Ask for reproduction steps (don't accept "probably" or "should fail")
2. Have reporter verify it with you
3. Only then prioritize it and assign it

### Planning Meetings
Before discussing a 5-phase fix:
1. "Can anyone reproduce this in 5 minutes?" (If not, Phase 0 is incomplete)
2. "What test confirms this bug?" (If none, Phase 0 is incomplete)
3. "Why does the code already work?" (Run it through the smell test)

---

## Why This Matters for Autarch

Autarch has **multiple integration points** where over-planning costs time:

- **Gurgeh → Coldwine:** A spec bug affects task generation. Phase 0 determines if it's Gurgeh or Coldwine.
- **Gurgeh → Pollard:** A research wiring issue might already be working. Phase 0 tests it first.
- **Bigend (Vauxhall):** TUI issues often depend on Bubble Tea behavior, not Bigend code. Phase 0 isolates this.

**With Phase 0:** "Research wiring is incomplete" → Test it → "Actually, it's working, just unused" → File a follow-up, don't plan a fix.

**Without Phase 0:** Create 3-phase plan → Spend 2 days implementing → Reviewer says "delete phases 2–3, this already works."

---

## Quick Reference

| Document | Length | When to Use | Key Output |
|----------|--------|-----------|------------|
| **PREVENTION_STRATEGIES.md** | 13 KB | Design process, train team | Full framework + examples |
| **PHASE_0_CHECKLIST.md** | 6 KB | Starting work on a bug | Reproducible bug + failing test |
| **OVERPLANNING_DETECTOR.md** | 8 KB | Writing/reviewing a plan | Simplified plan or "cancel" |

---

## Example: Autarch-73j Post-Mortem

**What happened:**
1. Oracle review flagged 6 potential issues
2. Team created a 5-phase plan
3. Team ran 7 research agents to understand scope
4. Three reviewers all said "reproduce first"
5. When finally tested: code was mostly working

**Cost:** ~4–6 hours of wasted planning + research

**Prevention:**
1. Phase 0: Test each issue. "Does issue #3 actually cause corruption?"
2. Severity: P0 (corruption) vs. P2 (unused feature) vs. P3 (optimization)
3. Plan: Only plan for P0 + P1. Defer P2/P3. Total scope: 1–2 phases, not 5.
4. Research: Only 1 focused researcher, not 7.

**Result:** Could have completed in 2–3 hours instead of 6+.

---

## Next Steps

1. **Read PREVENTION_STRATEGIES.md** (20 min) to understand the full framework
2. **Add PHASE_0_CHECKLIST.md to your issue templates** so every bug report includes reproduction
3. **Share OVERPLANNING_DETECTOR.md with code reviewers** so they can spot over-planning in real-time
4. **Post-planning:** Ask one peer to scan your plan with OVERPLANNING_DETECTOR.md before you start implementing

---

## FAQ

**Q: What if I need to plan a large refactor, not a bug fix?**
A: Same approach. Phase 0 = "Current state analysis" (why is refactor needed?). Phase 1 = minimal structural change. Phases 2+ = optional polish.

**Q: What if the bug is intermittent?**
A: Phase 0 takes longer, but still mandatory. Add conditions: "Reproduces when X is true" or "After N consecutive operations." A reproducible bug has conditions; find them.

**Q: What if my peer can't reproduce it?**
A: Extend Phase 0. It's not ready for planning. Work with your peer to isolate conditions (environment, data, config, timing).

**Q: Can I skip Phase 0 for trivial bugs (typos, obvious logic errors)?**
A: Minimal Phase 0: Confirm the typo exists, write a test, get peer confirmation. Still 5 minutes, but required.

**Q: What if I'm wrong about severity?**
A: You can re-assess severity during Phase 1. If you discover P0 is actually P2, simplify phases 2–N accordingly.

---

## Contributing Improvements

Found a gap in these guidelines? Found a false positive (a plan that succeeded despite red flags)?

Update this README and the three documents to capture what you learned. Post-mortems on over-planned or under-planned work are valuable.

---

## Resources in This Repository

- **PREVENTION_STRATEGIES.md** — 14 red flags + smell test + severity table
- **PHASE_0_CHECKLIST.md** — Bug verification checklist (quick + full)
- **OVERPLANNING_DETECTOR.md** — Real-time detection during planning
- **docs/oracle-architecture-review-2026-02-01.md** — Example of reviewer feedback that led to over-planning
- **docs/reviews/2026-02-01-oracle-prd-interview-review.md** — Detailed review showing "quick wins" vs. "structural improvements"

---

**Version:** 1.0 (Feb 3, 2026)
**Status:** Ready for use in all Autarch planning processes
**Created by:** Prevention Strategist for bug Autarch-73j (over-engineered plan post-mortem)
