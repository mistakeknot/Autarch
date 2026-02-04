This run can take up to an hour (usually ~10 minutes).
Answer:
Below is a code-referenced review of the PRD interview / creation path as implemented in the files you shared, covering: interview flow, PRD schema, creation logic, validation, and the “prd-interview” skill integration.

---

## 0) What exists today (so we’re reviewing the same thing)

There are **two-ish “creation” paths** and **two different “PRD” representations**:

### Creation paths

1. **CLI interview**: `Autarch/internal/gurgeh/cli/commands/interview.go`

   * Interactive prompts (default) OR non-interactive via flags / YAML config
   * Optional scan, then writes a `specs.Spec` YAML via `writeSpec()`
   * Optionally runs **agent suggestions** and **research** bootstrapping

2. **Quick create**: `Autarch/internal/gurgeh/cli/commands/create.go`

   * `gurgeh create --title ... --summary ...` writes a `specs.Spec` YAML directly
   * `--interactive` attempts to delegate to `InterviewCmd()`

### “PRD schema” representations

* **Legacy/feature spec**: `specs.Spec` in `Autarch/internal/gurgeh/specs/schema.go`
  This is what `InterviewCmd` and `CreateCmd` write.
* **New “version PRD + Features”**: `specs.PRD` + `specs.Feature` in `Autarch/internal/gurgeh/specs/prd.go`
  This looks like a “PRD per version milestone” model (features inside), but **the interview/create flows don’t write this format**.

That mismatch is central to the gaps.

---

## 1) Strengths of the current design

### A. Multiple execution modes with clean precedence

In `InterviewCmd()` (`interview.go`):

* Supports **interactive** and **non-interactive** (flags or `--config`) via `InterviewConfig`.
* Precedence is sensible: config file → overridden by flags:

  ```go
  if vision != "" { interviewCfg.Vision = vision }
  ...
  if requirements != "" { interviewCfg.Requirements = splitInput(requirements) }
  ```
* Nice addition: **plan mode**:

  ```go
  if planMode { return runInterviewPlan(...) }
  ```

  This is great for automation and CI-style flows (you can generate a plan, review, then apply).

### B. Vision gating is a strong product/process guardrail

This block:

```go
summaries, _ := specs.LoadSummaries(project.SpecsDir(root))
if specs.NeedsVisionSpec(summaries) { ... runVisionSprint(...) }
```

…is an excellent mechanism to prevent feature PRDs from being created without an overarching vision spec.

### C. Arbiter sprint path is a good “propose-first” backbone

`runArbiterSprint()` / `runVisionSprint()`:

* Creates orchestrator, sets scanner, runs through phases, exports spec.
* Automatically accepts each draft, and prints confidence:

  ```go
  fmt.Fprintf(out, "Confidence: %.0f%%\n", state.Confidence.Total()*100)
  ```

This is a strong foundation for a future “guided but fast” workflow.

### D. Central write+validate hook is good architecture

`writeSpec()` centralizes:

* dir creation
* ID creation (`specs.NextID`)
* timestamping `CreatedAt`
* YAML marshal + write
* validation + storing warnings

This is the right shape: all creation paths should go through it.

### E. Validation includes “evidence hygiene” (rare and valuable)

In `specs/validate.go`, `validateEvidenceRefs()` ensures evidence paths:

* are not absolute
* are under `.praude/research`
* actually exist on disk

That’s a practical constraint that keeps research claims anchored.

### F. Skill doc provides a strong intended UX framework

`autarch-plugin/skills/prd-interview/SKILL.md` is well structured:

* Context → Requirements → Success Criteria → Scope boundaries
* “Ask one question at a time”
* “Summarize before moving on”
  It’s a good target design, even if the CLI doesn’t yet implement it.

---

## 2) Gaps / missing functionality (big ones)

### A. The CLI interview collects only 4 fields, but schema expects far more

In `InterviewCmd` interactive mode, you prompt for:

* Vision
* Users
* Problem
* Requirements

But `specs.Spec` (`schema.go`) includes (non-exhaustive):

* Goals, NonGoals, Assumptions
* Acceptance criteria
* Files to modify
* Research paths
* Market research & competitive landscape (and evidence refs!)
* Hypotheses, StructuredRequirements
* Complexity, EstimatedMinutes, Priority
* VisionRef, review cadence fields, etc.

Today `buildSpecFromInterview()` populates only:

* `Title`, `Summary`
* `Requirements` (string list)
* minimal `StrategicContext`, `UserStory`
* two placeholder `CriticalUserJourneys`

Everything else is left empty/zero.

**Impact:** your generated PRDs are structurally “wide” but semantically mostly blank.

---

### B. Validation errors are silently ignored in creation flows

This is a critical logic bug.

`specs.Validate()` returns:

```go
type ValidationResult struct {
  Errors []string
  Warnings []string
}
```

But `writeSpec()` only returns/stores warnings:

```go
res, err := specs.Validate(raw, specs.ValidationOptions{Mode: specs.ValidationSoft, Root: root})
...
if len(res.Warnings) > 0 { specs.StoreValidationWarnings(...) }
return path, id, res.Warnings, nil
```

**`res.Errors` is ignored.**
So even if validation finds errors (e.g., missing required fields, invalid spec type), creation still reports success.

Same issue in `applyReadySuggestions()`:

```go
res, err := specs.Validate(updated, specs.ValidationOptions{Mode: specs.ValidationSoft, Root: root})
...
if len(res.Warnings) > 0 { _ = specs.StoreValidationWarnings(...) }
```

Errors are ignored again.

**Impact:** “hard” invariants aren’t enforced anywhere, even when detected.

---

### C. Status handling is inconsistent and often empty

* `CreateCmd` sets:

  ```go
  Status: "draft",
  ```
* `buildSpecFromInterview()` does **not set Status**, so it will serialize as empty string.

Validation only warns on invalid status **if status is non-empty**:

```go
if doc.Status != "" && !validStatus(doc.Status) { ... }
```

**Impact:** downstream tools that rely on `spec.Status` will behave inconsistently depending on creation path.

---

### D. `gurgeh create --interactive` delegation is broken-ish

In `CreateCmd()`:

```go
if interactive {
  interviewCmd := InterviewCmd()
  return interviewCmd.RunE(cmd, args)
}
```

Problems:

* You’re calling `InterviewCmd().RunE` directly, **without Cobra parsing interview flags**.
* You pass `cmd` from the create command, not the interview command, so `cmd.Flags()` etc. are mismatched.
* Users cannot do: `gurgeh create --interactive --skip-scan` (that flag belongs to interview, won’t parse here).

**Impact:** confusing CLI behavior + impossible to pass interview-specific knobs through `create`.

---

### E. Directory / naming split: `.gurgeh` vs `.praude`

You have inconsistent storage roots:

* `create.go` writes to:

  ```go
  specsDir := filepath.Join(cwd, ".gurgeh", "specs")
  ```
* `specs/prd.go` uses:

  ```go
  specsDir := filepath.Join(projectPath, ".praude", "specs")
  ```
* validation evidence requires `.praude/research`:

  ```go
  prefix := filepath.Clean(filepath.Join(".praude", "research"))
  ```

Skill doc says output to `.gurgeh/specs/...`.

**Impact:** tooling will drift: validation may reject paths created by other parts; users will find multiple hidden directories; migrations become messy.

---

### F. `specs.CreateTemplate` / `CreateBlank` likely emit invalid YAML

`specs/create.go`:

```go
doc := fmt.Sprintf(`id: "%s" title: "Example PRD Title" created_at: "%s" status: "draft" strategic_context: ...
```

There are multiple YAML keys on the same line without flow mapping syntax.

Same in `CreateBlank`.

**Impact:** any command that relies on these templates will produce files that fail `yaml.Unmarshal`.

---

### G. “Agent suggestions auto-apply” is race-prone

In `autoApplySuggestions()`:

1. create suggestions file (`suggestions.Create`)
2. write brief
3. `launcher(profile, briefPath)`
4. immediately `applyReadySuggestions(root, id, suggPath)`

If `launchAgent` starts an external process and returns quickly (common), the suggestions file likely isn’t populated yet, so:

```go
ready := suggestions.ParseReady(raw)
if suggestions.IsEmpty(ready) { return false, nil }
```

**Impact:** “auto-apply” often does nothing, silently.

---

### H. Arbiter sprint “ReviseDraft” might be a no-op (depending on API)

In `runArbiterSprint()`:

```go
orch.ReviseDraft(state, cfg.Problem, "cli input")
...
state = orch.AcceptDraft(state)
```

If `ReviseDraft` returns a new `state` (functional style), this ignores it. You *do* assign `state = orch.AcceptDraft(state)` but not revise.

**Impact:** injected CLI inputs might not actually enter the final spec.
(If `ReviseDraft` mutates orchestrator internal state, then it’s OK, but the inconsistent style is a code smell.)

---

## 3) UX issues in the interview flow (interactive + non-interactive)

### A. “Draft PRD ready” is misleading

You build a draft from scan summary:

```go
draft := buildDraftSpec(summary)
fmt.Fprintln(out, "Draft PRD ready.")
...
confirm, _ := promptYesNo(..., "Confirm draft? (y/n) ")
```

But that `draft` is **never merged into** the final `spec` created by `buildSpecFromInterview()`.

**User perception:** “I confirmed the draft PRD that was generated from scan.”
**Reality:** they just confirmed “continue.”

**Fix direction:** either (1) actually use draft content to seed fields, or (2) rephrase: “Repo scan complete. Continue to interview?”

---

### B. Requirements prompt claims newline support but reads one line

Prompt says:

```go
"Requirements (comma or newline separated): "
```

but you call `promptLine(...)` which (by name/typical behavior) reads a single line.

Yes, `splitInput()` supports newlines, but interactive input likely can’t enter multi-line naturally.

---

### C. Requirements parsing will duplicate IDs and doesn’t respect user formatting

`parseRequirements()` always generates new IDs:

```go
id := formatReqID(i + 1)
out = append(out, id+": "+part)
```

So user input like:

* `REQ-007: Must support SSO`
  becomes:
* `REQ-001: REQ-007: Must support SSO`

Also it splits on commas, so requirements containing commas get mangled.

---

### D. Question order doesn’t match your own skill doc, and loses semantics

Skill doc Phase 1: Vision → Problem → Beneficiary.

CLI asks:

* Vision
* Users
* Problem
* Requirements

Not fatal, but the generated output also loses explicit “Problem” and “Users” fields (they’re embedded indirectly into `Title/Summary/UserStory.Text`).

---

### E. Generated user story text is awkward

`buildSpecFromInterview()`:

```go
Text: "As a user, " + firstNonEmpty(users, "I need", "I need") + ", " + summary,
```

If `users = "admins"`, you get:

> “As a user, admins, …”

You likely intended something like:

> “As an admin, I want … so that …”

---

### F. Non-interactive mode can unintentionally create low-quality specs

Non-interactive is triggered if **any** of the four fields is provided:

```go
nonInteractive := configFile != "" || vision != "" || users != "" || problem != "" || requirements != ""
```

So `praude interview --requirements "foo"` becomes non-interactive, and will create:

* title = `"New PRD"` (fallback)
* summary = `"Summary pending"` (fallback)
* user story = `"As a user, I need, Summary pending"` (fallback)

No prompt to fill missing values.

---

## 4) Schema completeness (what’s good, what’s missing, what’s inconsistent)

### A. `specs.Spec` is broad and forward-looking (good)

You cover many PRD-grade concerns:

* Goals / NonGoals
* Assumptions with decay
* Hypotheses with metric/baseline/target/timebox
* Research artifacts + evidence references
* CUJs linked to requirements
* Complexity / estimate / priority

This is more complete than many PRD schemas.

---

### B. But the schema is internally inconsistent / duplicated in a few places

1. **Two requirements systems**

* `Requirements []string` (unstructured)
* `StructuredRequirements []Requirement` (Given/When/Then)

No validation enforces either, and nothing keeps them in sync.

2. **Two “PRD” concepts in the repo**

* `Spec` is what interview creates.
* `PRD` (`specs/prd.go`) is version-scoped and has `Features []Feature`.

  * Yet the CLI “create PRD” flow does not create a `PRD` file.
  * `MigrateSpecToPRD` exists, suggesting you’re mid-migration.

3. **Status vocabularies diverge**

* `specs/prd.go`: `draft/approved/in_progress/done`
* `specs/validate.go`: `"interview", "draft", "research", "suggestions", "validated", "archived"`

Migration (`MigrateSpecToPRD`) casts `FeatureStatus(spec.Status)` which can yield invalid feature status values.

---

### C. Missing explicit first-class fields for what you interview

You interview `Users` and `Problem`, but schema has no:

* `problem_statement`
* `target_users/personas`

You compress those into:

* `Summary`
* `UserStory.Text`

That’s lossy (harder to validate/search/report later).

---

### D. Validation currently covers only a small subset of schema

`specs.Validate()` checks:

* required fields: ID/Title/Summary
* type validity (if set)
* status validity (if set)
* CUJ IDs/priority/linked requirement existence
* market research + evidence refs existence
* competitive landscape + evidence refs existence

It **does not** validate:

* acceptance criteria present/IDs unique
* files_to_modify presence/shape
* complexity/priority ranges
* goals/non-goals/assumptions format or IDs
* structured requirements consistency
* user_story.hash correctness
* vision_ref existence / alignment

Given how broad `Spec` is, current validation is too shallow to keep quality high.

---

## 5) Suggestions for improvement (specific, code-focused)

I’ll split this into **quick wins (high ROI)** and **structural improvements**.

---

### Quick wins (do these first)

#### 1) Enforce `ValidationResult.Errors`

Update `writeSpec()` and `applyReadySuggestions()` to fail or at least report errors.

Example pattern:

```go
res, err := specs.Validate(raw, opts)
if err != nil { ... }
if len(res.Errors) > 0 {
  return path, id, append(res.Errors, res.Warnings...), fmt.Errorf("validation failed: %s", strings.Join(res.Errors, "; "))
}
```

At minimum: print errors alongside warnings in `InterviewCmd`:

```go
if len(res.Errors) > 0 { ... }
```

Right now, “errors” are basically dead code.

---

#### 2) Always set `Spec.Status` and `Spec.Type` in creation

In `buildSpecFromInterview()` (and probably in `buildDraftSpec()`), set:

* `Type: specs.SpecTypePRD`
* `Status: "draft"` (or `"interview"` if you want a pipeline state)

This aligns with `CreateCmd` behavior and makes downstream tooling consistent.

---

#### 3) Fix `gurgeh create --interactive` delegation

Don’t call `RunE` directly with the wrong command context.

Two clean options:

* **Option A:** Make `interview` a real subcommand under the same root, and have create call it via Cobra execution (`SetArgs` + `Execute`).
* **Option B (simpler):** Extract the core logic of `InterviewCmd.RunE` into a shared function like:

  ```go
  func runInterview(out io.Writer, in io.Reader, root string, cfg InterviewConfig, opts interviewOpts) error
  ```

  Then `create` and `interview` both call `runInterview`.

---

#### 4) Make requirement parsing resilient

Replace `parseRequirements()` with logic:

* If a line already starts with `REQ-###:` keep it
* If it starts with `- ` bullet, strip it
* Only generate IDs for unlabeled lines

This prevents `REQ-001: REQ-007: ...` duplication.

Also: stop splitting on commas by default for interactive input; prefer multi-line list entry (see below).

---

#### 5) Reword the scan “draft” prompt (or actually use it)

Right now “Confirm draft?” is misleading because the draft isn’t used.

Either:

* change copy to “Repo scan complete. Continue?”
  or
* incorporate scan output into the created spec (see structural improvements).

---

### Structural improvements (bigger but worth it)

#### 6) Implement the skill’s phased interview in the CLI

Your skill doc is *much* better UX than the current 4-field prompt.

Map skill phases to schema fields:

* **Context**

  * Vision → (either `VisionRef` or a `vision_summary` field)
  * Problem → add a `problem_statement` field OR store as structured block in Summary
  * Beneficiary → add `target_users/personas` field

* **Requirements**

  * Must-haves / Nice-to-haves → could become:

    * `StructuredRequirements` with `Type=functional` + status
    * or split requirements into “MVP” vs “Later”
  * Constraints → map to `Requirement.Constraints` and/or add a top-level `constraints` list

* **Success Criteria**

  * Map to `Goals[]` (Metric + Target)
  * Add “failure modes” maybe as `risks` or `non_goals`

* **Scope boundaries**

  * `NonGoals[]`
  * `Assumptions[]`

Even if you don’t add new fields, you can *at least* populate existing `Goals`, `NonGoals`, `Assumptions`, and `Acceptance`.

---

#### 7) Add a real multi-line list prompt

Introduce `promptMultiline(reader, out, label, terminator)` (e.g. end with blank line). Use for:

* requirements
* goals
* non-goals
* assumptions
* acceptance criteria
* CUJ steps

This eliminates the comma-splitting problems and matches how humans enter lists.

---

#### 8) Use scan results to seed “files_to_modify” and/or “strategic context”

Right now scan produces only:

```go
summary = "Scan summary: N files, M bytes"
```

and isn’t stored.

Better uses:

* pre-fill `FilesToModify` with top-level directories/components detected
* store scan summary as a research/evidence artifact (so it’s preserved)
* use scan result to propose candidate `CriticalUserJourneys` steps (even as placeholders)

If you don’t want heavy inference, just attach the scan summary into:

* `Metadata` (add a `scan_summary` field) or
* `Research` with a generated `.md` artifact

---

#### 9) Unify `.gurgeh` vs `.praude` once and for all

Pick one hidden directory name and update:

* `create.go` specsDir
* `specs/prd.go` Save/Load paths
* `validate.go` `isResearchPath()`
* skill doc output paths

Right now, your validation hardcodes `.praude/research`, while other code uses `.gurgeh/specs`.

This is one of the biggest “paper cut → big confusion” sources.

---

#### 10) Decide whether `specs.Spec` or `specs.PRD` is canonical

You currently have:

* `Spec` = detailed feature PRD
* `PRD` = version container with `Features`

But CLI “Create a PRD” creates a `Spec` file, not a `PRD` file.

Two coherent end states:

* **State A:** Rename `Spec` to `FeatureSpec` (or similar), and make CLI create Features inside a PRD version file.
* **State B:** Deprecate `specs.PRD` and keep `Spec` as the PRD artifact.

Right now you’re in an ambiguous middle.

---

#### 11) Integrate the PRD reviewers into the post-create pipeline

You already have the parallel review framework:

```go
review.RunParallelReview(ctx, spec, review.DefaultReviewers())
```

Add it at the end of `InterviewCmd` after `writeSpec()`:

* print overall score
* list issues with severity and location
* optionally fail if below threshold (or store results in metadata)

This gives you meaningful “quality validation” beyond structural YAML checks.

---

#### 12) Fix `specs/create.go` template YAML formatting

`CreateTemplate`/`CreateBlank` should emit valid YAML (one key per line, proper indentation), or generate via `yaml.Marshal` from a struct instead of hand-written YAML strings.

Given you already have `Spec` types, the cleanest fix is:

* instantiate `Spec{...}`
* `yaml.Marshal(spec)`
* write file

---

## Summary of the biggest issues to fix first

If you only do 3 things, do these:

1. **Stop ignoring `ValidationResult.Errors`** (in `writeSpec` and suggestion apply path).
2. **Fix directory + command consistency** (`.gurgeh` vs `.praude`, and create→interview delegation).
3. **Bring CLI interview closer to the skill phases** (at least Goals/NonGoals/Assumptions + better requirements entry).

If you want, I can propose a concrete refactor plan (function signatures + where to move shared logic) that keeps behavior stable while you fix the biggest correctness issues (validation errors + path consistency) first.


7m13s · gpt-5.2-pro[browser] · ↑13.63k ↓4.99k ↻0 Δ18.62k
files=9
