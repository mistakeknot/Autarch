# Phase 0: Bug Verification Checklist

**Goal:** Confirm a bug actually exists before writing a fix plan. This checklist must be completed before any planning begins.

---

## Quick Version (5 minutes)

```
VERIFY:
1. [ ] Run the code. Does it fail? Yes / No / Sometimes
2. [ ] Where does it fail? File: ___ Line: ___
3. [ ] Why does it fail? ________________
4. [ ] Is this already fixed? (git blame, search codebase)
5. [ ] One peer confirms: Yes / No

ONLY THEN: Write a plan
```

---

## Full Checklist (15–20 minutes)

### Reproduction
- [ ] Bug is reproducible with **exact** steps:
  ```
  Command: ________________
  Expected: ________________
  Actual: ________________
  Every time? Yes / No / Intermittent
  ```

- [ ] I can see the failure with my own eyes
  - [ ] Error message or visible malfunction
  - [ ] Not hearsay or assumption
  - [ ] Screenshot / log output attached (if possible)

### Code Location
- [ ] Found the code path that fails
  - File: `________________`
  - Function: `________________`
  - Lines: `________________`

- [ ] Root cause (write in one sentence):
  ```
  ________________
  ```

### Already Fixed?
- [ ] Searched codebase for this error/condition
  - Result: Found in `________________` OR "Not found"

- [ ] Checked git blame for this line
  - Last change: `________________`
  - Reason: `________________`

- [ ] Looked for related guards/checks nearby
  - Found: `________________` OR "None found"

### Failing Test
- [ ] Created a minimal test case that reproduces the bug
  - Test file: `________________`
  - Test name: `TestBug________________`
  - Status: Fails without fix ✓ / Passes (inconclusive)

- [ ] Test is minimal (< 20 lines, if possible)
  - [ ] Only includes necessary setup
  - [ ] No unrelated assertions

### Type/Static Checks
- [ ] Ran `go vet` on affected files → "Clean" or Issues: `________________`
- [ ] No obvious type errors prevent the code path from running

### Severity Assessment
Choose **one:**
- [ ] **P0** — Production is broken, customers affected immediately
- [ ] **P1** — Major feature is broken, people can't use it
- [ ] **P2** — Edge case broken, workaround exists or feature still mostly works
- [ ] **P3** — Cosmetic issue, polish, or "nice to have"
- [ ] **Unconfirmed** — Still investigating (STOP, don't plan yet)

### Peer Confirmation
- [ ] Asked one peer to review Phase 0
  - Peer: `________________`
  - Feedback: "Bug confirmed" / "Unclear" / "Already working"
  - Peer notes: `________________`

---

## When to STOP and Re-Verify

❌ **Stop if:**
- You're describing the bug as "probably" or "likely" — go reproduce it
- The test passes even though you expected it to fail — the bug may not exist
- Peer says "I can't reproduce it" — don't plan yet, investigate further
- You find 3+ guards in the code that should have prevented this — code may already handle it
- Severity is unclear — test both "normal" and "edge case" scenarios

---

## When to PROCEED to Planning

✅ **Only proceed to planning if:**
- [ ] Bug is reproducible **every time** (or at least consistently)
- [ ] You've located the exact code responsible
- [ ] A test confirms the failure
- [ ] One peer has confirmed the bug is real
- [ ] You've ruled out "already fixed" scenarios (git blame, search)
- [ ] Severity is clear

---

## Planning Scope (After Phase 0)

Once Phase 0 is complete, use this table to decide plan depth:

| Severity | Plan Depth | Phases | Research | Review |
|----------|------------|--------|----------|--------|
| **P0** | Deep | 4–5 | 1–2 agents | Required |
| **P1** | Medium | 2–3 | 0–1 agent | Recommended |
| **P2** | Shallow | 1–2 | 0 agents | Optional |
| **P3** | Minimal | 1 phase (inline) | 0 agents | Skip |

---

## Example: Gurgeh "Validation Silently Ignores Errors"

### Phase 0 (Oracle review flagged this)

**Reproduction:**
```bash
# Create an invalid spec (empty title)
echo 'id: "spec-001"
title: ""
summary: "test"' > test-spec.yaml

# Try to write it
go run ./cmd/gurgeh interview --spec test-spec.yaml

# Expected: Error reported
# Actual: ??? (need to test)
```

**Code location:**
```
File: internal/gurgeh/specs/validate.go
Function: Validate()
Issue: errors in ValidationResult are computed but never returned
```

**Already fixed?**
```
Git blame: Last changed 3 weeks ago in commit abc123
Reason: "Add validation modes (soft/hard)"
Search: ValidationResult.Errors is assigned but checked nowhere in writeSpec()
```

**Failing test:**
```go
func TestValidationErrorsNotIgnored(t *testing.T) {
  invalid := specs.Spec{
    ID: "test",
    Title: "",  // Required field
    Summary: "test",
  }
  res, err := specs.Validate(invalid, specs.ValidationOptions{})
  assert.True(t, len(res.Errors) > 0, "Expected validation error for empty title")
  // Currently PASSES (no error) — that's the bug
}
```

**Severity:** P1 (data quality issue, but Gurgeh still works; you just get bad specs)

**Plan scope:** 1–2 phases (enforce errors in writeSpec + add this test)

---

## Tips for Efficient Phase 0

1. **Use the debugger** — Single-step through the suspected code path rather than guessing
2. **Add logging** — Print variable values at the suspected failure point
3. **Check git history** — `git blame` and `git log -p` reveal *why* code is this way
4. **Search systematically** — Grep for error messages, function names, even variable names
5. **Ask the original reporter** — "Exactly which button did you click? What did you see?"
6. **Pair with one peer** — Explaining it to someone else often reveals the bug faster

---

## After Phase 0: Before You Plan

Do NOT write a plan until:
1. [ ] This checklist is 100% complete
2. [ ] Your test fails (or you've confirmed the bug with manual steps)
3. [ ] Peer has confirmed Phase 0 findings
4. [ ] You understand the severity

**If any of those is unclear, extend Phase 0. Do not start planning.**

---

## Version History

- **2026-02-03** — Initial version, derived from Autarch-73j post-mortem (over-engineered plan)
