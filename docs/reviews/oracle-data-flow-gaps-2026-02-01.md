🧿 oracle 0.8.5 — Token mop for agent spillover.
Packed 516 files into 1 bundle (contents counted in token estimate).
Launching browser mode (gpt-5.2-pro) with ~555,702 tokens.
This run can take up to an hour (usually ~10 minutes).
Answer:
According to a document from **February 2, 2026**, here’s what the current Go monorepo implements for the **Research → Spec → Task** flow, where it breaks down, and the **exact file paths + function names** that are the most direct modification points to make the automation seamless.

---

## 1) Pollard → Gurgeh handoff (Research ingestion)

### What exists today

#### A) Pollard produces research artifacts on disk

Pollard persists **structured YAML insights** under `.pollard/insights/` via `internal/pollard/insights/insight.go` (`Save`, `LoadAll`).

Pollard also has a **report summary loader** that reads `.pollard/reports/summary.json` via `internal/pollard/reports/load.go` (`LoadSummary`).

So: Pollard has two “output channels” on disk:

* `.pollard/insights/*.yaml` (insights)
* `.pollard/reports/summary.json` (report summary)

#### B) Gurgeh’s sprint logic can trigger/collect research, but doesn’t inject it into the drafting prompt

In `internal/gurgeh/arbiter/orchestrator.go`, Gurgeh has:

* a `quickScanner` (Pollard quick scan)
* a `research` provider (an abstraction that can run targeted scans and fetch linked insights)


During a phase, Gurgeh *does* run research and store it on the sprint state:

* `runPhaseResearch(...)` calls `o.research.RunTargetedScan(...)`, then `o.research.FetchLinkedInsights(...)`, and writes results into `state.Findings`.
* `runQuickScan(...)` uses `o.quickScanner.QuickScan(...)`, publishes it as an Insight, and stores `state.ResearchCtx` (GitHub/HN hits + summary).

Also `StartWithResearch(...)` can accept `pollardFindings` and publish them, then sets `state.Findings`.

**But:** when Gurgeh generates the next draft text, the research isn’t included in the generation prompt:

* `ProcessChatMessage(...)` builds a string from existing section content + user feedback, then calls `o.generator.GenerateDraft(...)` with `projectCtx` and that combined string — **no `state.Findings`, no `state.ResearchCtx`.**
* The sprint progression path (`Start`, `Advance`) also calls `o.generator.GenerateDraft(...)` without passing research findings.

On the generator side, `internal/gurgeh/arbiter/generator.go` builds a `contextString` only from `projectCtx` + `userInput` (+ optional `scanData` evidence), and then runs the phase generator — still **no research findings channel** in the signature.

#### C) There *is* a manual import path (explicitly non-automated)

`internal/gurgeh/cli/commands/import_research.go` implements a manual flow:

* reads Pollard insights via `discovery.LoadPollardInsights(root)`
* generates a research doc
* writes it under Gurgeh’s research directory
* opens `$EDITOR` for manual PRD edits

This is a clear “human handoff” step, not automation.

#### D) There’s a “bridge intent” via Intermute (but still not prompt-injected)

Pollard has sync scaffolding intended to bridge into Intermute: `internal/pollard/sync/sync.go` (“syncs Pollard scan results and insights to Intermute”).

Gurgeh also has an Intermute syncer, but that’s for spec syncing, not task-block event ingestion. `internal/gurgeh/sync/sync.go` is “synchronization between Gurgeh specs and the Intermute system”.

### The concrete implementation gaps (why automation breaks)

1. **Gurgeh collects research (`state.Findings`, `state.ResearchCtx`) but never feeds it into the text-generation prompt.**
2. **No file watcher** in Gurgeh for `.pollard/insights` or `.pollard/reports`. The only on-disk ingestion path is the **manual** CLI command (`import-research`).
3. Research *does* make it into the exported Spec structure at the end (e.g., findings mapped into `MarketResearch` / `CompetitiveLandscape`), but that is *export-time*, not *draft-time prompt context*.

### Files + functions to modify for Goal #1 (auto-inject Pollard insights into Gurgeh’s prompt)

**Primary injection points (highest leverage):**

* `internal/gurgeh/arbiter/orchestrator.go`

  * `ProcessChatMessage(...)` — append/merge `state.Findings` + `state.ResearchCtx` into the prompt passed to the generator.
  * `Start(...)` and `Advance(...)` — same: ensure generation calls receive research context for each phase draft generation.
  * (Optional) `chatProcessMessage(...)` — if this is used by the TUI streaming path, same fix applies.

* `internal/gurgeh/arbiter/generator.go`

  * `GenerateDraft(...)` — expand the signature to accept a research bundle (e.g., `findings []ResearchFinding`, `researchCtx *ResearchContext`) and incorporate it into `contextString` (similar to how `scanData` evidence is injected today).

**Secondary (if you want on-disk Pollard ingestion without Intermute):**

* `internal/pollard/insights/insight.go`

  * `LoadAll(...)` already exists; use it from Gurgeh to auto-collect insights on each phase or when starting a sprint.
* `internal/pollard/reports/load.go`

  * `LoadSummary(...)` already exists for `.pollard/reports/summary.json`; use it as an additional “research digest” input to Gurgeh prompts.

**If you want to deprecate the manual step:**

* `internal/gurgeh/cli/commands/import_research.go`

  * `ImportResearchCmd.RunE(...)` is currently the explicit manual handoff; either remove it, or refactor shared logic into a reusable loader that the orchestrator can call automatically.

---

## 2) Coldwine → Gurgeh feedback loop (Task blockers updating Spec status/confidence)

### What exists today

#### A) Coldwine has “blocked” as a task status, but the standard CLI block path is local-only

`internal/coldwine/cli/task_block.go` sets `task.Status = specs.StatusBlocked` and writes YAML, but **does not broadcast anything upstream** (no Intermute message, no signal).

#### B) Coldwine does have an Intermute broadcaster for blocked tasks — but it’s not wired into the common block paths

`internal/coldwine/intermute/broadcaster.go`:

* `TaskBroadcaster.BroadcastBlocked(...)` sends an Intermute message with `Type: "task.blocked"` and a payload containing `TaskID`, `StoryID`, and `BlockedReason`.

The payload type is `pkg/intermute.TaskEventPayload` and does **not** include any Gurgeh spec identifier/path (only StoryID/TaskID + reason).

#### C) The agent execution loop treats “blocked” as a terminal state but takes no action

In `internal/coldwine/agent/loop.go`, `ApplyDetection(...)`:

* writes detection result to storage
* if `TaskStateDone` ⇒ enqueue review
* if `TaskStateBlocked` ⇒ **returns nil** (no enqueue, no broadcast)

#### D) Gurgeh has a “needs review” mechanism, but it’s signal-based and Gurgeh doesn’t subscribe to task-block events

Gurgeh can mark a spec as needing review based on signals:

* `internal/gurgeh/signals/review.go` evaluates active signals and sets `NeedsReview` with a reason when thresholds are crossed.

Gurgeh does have an easy way to update spec status in place:

* `internal/gurgeh/specs/load.go` exposes `UpdateStatus(path, status)`.

But Gurgeh’s own emitter is explicitly *not* a background listener; it “checks on spec load — no background process.”
And `pkg/signals/signal.go` does not define any `task_blocked` signal type (only competitor/research/assumption/hypothesis/spec_health/execution_drift/vision_drift).

### The concrete implementation gaps

1. **The “task blocked” event is not guaranteed to be emitted** when users block tasks (CLI path writes YAML only).
2. **Even where “task.blocked” is emitted (Intermute), the payload doesn’t identify a Gurgeh spec**, so Gurgeh can’t deterministically map the block to `.gurgeh/specs/<id>.yaml`.
3. **Gurgeh doesn’t listen for `task.blocked` messages**, and there is no `EventTaskBlocked` / `SignalTaskBlocked` type in the shared signals contract to hook into Gurgeh’s `NeedsReview` logic.

### Files + functions to modify for Goal #2 (ColdWine tasks flag Gurgeh specs “needs revision”)

**Emit a blocker event consistently (ColdWine side):**

* `internal/coldwine/cli/task_block.go`

  * `blockTask(...)`: after persisting the YAML, call a broadcaster (`TaskBroadcaster.BroadcastBlocked`) or emit a `signals.SignalExecutionDrift` (or a new task-blocked signal) so the event exists outside the local YAML file.

* `internal/coldwine/agent/loop.go`

  * `ApplyDetection(...)`: add a branch for `TaskStateBlocked` that *emits* the same upstream event (Intermute message and/or signals emission). Right now blocked is a no-op beyond storage update.

**Make the event mappable to a Gurgeh spec:**

* `pkg/intermute/types.go`

  * `TaskEventPayload`: extend it to include one of:

    * `SpecID` (preferred if you use the same PRD IDs), or
    * `SpecPath`, or
    * `PRDID` / `GurgehSpecID`

  Right now it’s only `TaskID`, `StoryID`, `BlockedReason`.

* `internal/coldwine/intermute/broadcaster.go`

  * `TaskBroadcaster.BroadcastBlocked(...)`: populate the new payload field(s) so downstream consumers can resolve the spec without heuristic mapping.

**Give Gurgeh a first-class “task blocked ⇒ needs review” intake path:**
You have two clean options:

1. **Signals-based (fits existing Gurgeh `NeedsReview` evaluator):**

* `pkg/signals/signal.go`

  * add a `SignalTaskBlocked` (or reuse `SignalExecutionDrift` but with `AffectedField: "tasks"` and clear title/detail). Currently no such type exists.
* Gurgeh-side ingestion should write signals into Gurgeh’s signal store so `review.go` can do its job.

2. **Direct status update (spec file mutation):**

* `internal/gurgeh/specs/load.go`

  * use `UpdateStatus(...)` to set e.g. `"needs_revision"` (you’d be introducing a new status string convention).

**Where to implement the listener on Gurgeh side (most natural place in this repo):**

* `internal/gurgeh/sync/sync.go`

  * add a “pull task events” method alongside `PullSpec(...)` / `PushSpec(...)`, because this file is already the canonical place for “Gurgeh <-> Intermute” synchronization.

> **Answer to “Confirm if EventTaskBlocked events are listened to by Gurgeh”:**
> I found **no Gurgeh subscription/listener** for `task.blocked`, and Gurgeh’s signal system is explicitly “checked on spec load — no background process.”

---

## 3) Agent context access (does the agent automatically include Pollard/Gurgeh files?)

### What exists today

#### Coldwine prompt/context building is centralized in init flow, and it’s not pulling research/spec files

In `internal/coldwine/cli/init_flow.go`, the agent prompt is assembled in:

* `buildAgentPrompt(input InitInput) string` — builds the prompt content
* `writeAgentPrompt(root string, input InitInput) error` — writes it to disk

`buildAgentPrompt` includes:

* `input.Summary`
* the YAML schema (inline)
* instructions about producing epics and tasks

It does **not** read or embed:

* `.pollard/reports/*`
* `.pollard/insights/*`
* `.gurgeh/specs/*`


So the answer to “Does it automatically include relevant files from .pollard/reports/ or .gurgeh/specs/?” is: **No, not in this prompt construction path.**

#### (Related) Gurgeh’s “brief” builder references research files, but only by file path list

Gurgeh’s TUI builds an agent brief (including `ResearchFiles: spec.Research`) in `internal/gurgeh/tui/briefs.go` (`buildResearchBrief`). That’s a list of research files already attached to the spec, not auto-discovered from Pollard output dirs.

### The concrete implementation gaps

1. **No automatic inclusion of `.pollard/*` research artifacts** in the Coldwine agent prompt/context window.
2. **No automatic inclusion of `.gurgeh/specs/*`** in that agent prompt path either (unless it’s manually summarized into `input.Summary`).
3. Gurgeh’s briefs include research **paths** (`spec.Research`) but nothing ensures Pollard artifacts end up in `spec.Research` automatically.

### Files + functions to modify for Goal #3 (inject research reports into agent runtime context)

**ColdWine prompt/context injection point:**

* `internal/coldwine/cli/init_flow.go`

  * `buildAgentPrompt(...)`: extend this function to load and embed:

    * Pollard report summary (`internal/pollard/reports.LoadSummary`)
    * Pollard insights (`internal/pollard/insights.LoadAll`)
    * (Optionally) the relevant Gurgeh spec text (load `.gurgeh/specs/<id>.yaml`, or the PRD content if you standardize IDs)

This is **the exact code block** where the system prompt/context window is currently built for Coldwine’s init-agent workflow.

**If you want Gurgeh briefs to also auto-attach Pollard artifacts:**

* `internal/gurgeh/tui/briefs.go`

  * `buildResearchBrief(...)`: augment `ResearchFiles` to include discovered Pollard artifacts (or references pulled from Intermute), not only `spec.Research`.

---

# Final “edit list” (exact targets aligned to your 3 output goals)

## Goal 1 — Auto-inject Pollard insights into Gurgeh’s prompt

1. `internal/gurgeh/arbiter/orchestrator.go`

   * `ProcessChatMessage(...)`
   * `Start(...)`
   * `Advance(...)`
   * (if used) `chatProcessMessage(...)`

2. `internal/gurgeh/arbiter/generator.go`

   * `GenerateDraft(...)` (expand signature + inject research into `contextString`)

3. (Optional ingestion helpers)

   * `internal/pollard/insights/insight.go`: `LoadAll(...)`
   * `internal/pollard/reports/load.go`: `LoadSummary(...)`

## Goal 2 — Allow Coldwine tasks to flag Gurgeh specs as needs revision

1. `internal/coldwine/cli/task_block.go`

   * `blockTask(...)` (emit upstream event after writing YAML)

2. `internal/coldwine/agent/loop.go`

   * `ApplyDetection(...)` (on `TaskStateBlocked`, emit upstream event)

3. `internal/coldwine/intermute/broadcaster.go`

   * `TaskBroadcaster.BroadcastBlocked(...)` (include a resolvable Gurgeh spec identifier in the payload)

4. `pkg/intermute/types.go`

   * `TaskEventPayload` (add `SpecID`/`SpecPath`/`PRDID`)

5. `internal/gurgeh/sync/sync.go`

   * add a listener/puller method similar to `PullSpec(...)` to ingest `task.blocked` and convert into:

     * signals (preferred), or
     * `specs.UpdateStatus(...)` mutations

6. (If using signals) `pkg/signals/signal.go`

   * add `SignalTaskBlocked` (or reuse `SignalExecutionDrift` with consistent conventions)

## Goal 3 — Inject research reports into the Agent’s runtime context

1. `internal/coldwine/cli/init_flow.go`

   * `buildAgentPrompt(...)` (embed Pollard + Gurgeh context)
   * `writeAgentPrompt(...)` (no structural change required unless you want multiple files / attachments)

2. (Optional) `internal/gurgeh/tui/briefs.go`

   * `buildResearchBrief(...)` (auto-attach Pollard outputs to `ResearchFiles`)

---

If you want, I can also propose a minimal patch strategy that avoids signature churn (e.g., formatting findings into a markdown “Research Context” block inside `orchestrator.go` just before the `GenerateDraft` call), but the list above is the set of **most direct, lowest-ambiguity modification points** to achieve the three automation goals.


17m51s · gpt-5.2-pro[browser] · ↑555.7k ↓3.69k ↻0 Δ559.39k
files=8
