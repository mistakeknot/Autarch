# Agent workbench live walkthrough

Date: 2026-09-13

Scope: actual Autarch project workbench use on this Mac against separately
signed-in Claude Code and Codex tmux sessions. This is product evidence, not a
claim about account ownership, provider billing, or deployment.

## Journey and results

1. Opened the Autarch project workbench at reviewed core commit `d128981` and
   selected Codex target `/private/tmp/tmux-501/default`, server
   `1925/1788560628`, session `$30`, window `@30`, pane `%30`, PID `3977`.
2. Previewed and approved a full implementation handoff for `Sylveste-fuwn`.
   The durable record reported `delivered` before the pane began work.
3. While Codex was working, previewed and explicitly approved **Interrupt and
   redirect**. Autarch sent the normal stop-turn interaction, observed a fresh
   provider-ready state, then delivered an investigation-only replacement.
   The durable record contains both `interruption: delivered` and
   `delivery: delivered`; no automatic retry occurred.
4. Codex returned a bounded recommendation: show the selected session display
   label on the narrow agent row and truncate only that label.
5. A proposed delta that changed authority from investigation to implementation
   was rejected before delivery. Reopening Autarch restored the outcome,
   `Sylveste-fuwn`, exact `%30` target, and unsent draft. Neither earlier
   message was replayed. The restored instruction was then approved as a fresh
   full implementation handoff.
6. Codex changed only `internal/door/workbench.go` and
   `internal/door/workbench_test.go`; its focused race test passed. Autarch
   records this as an agent report, separate from verification.
7. Switched in the workbench to Claude Code target on the same tmux server,
   session `$165`, window `@206`, pane `%206`, PID `85612`. The exact read-only
   review message was previewed and delivered. Claude Code reported **PASS**
   and reran the focused test. Its stale local plugin hooks emitted errors;
   Autarch did not replay the already-delivered request.
8. Rebuilt and reopened the changed application at 42 by 18 columns. The live
   row rendered `AGENT · Claude Code · %206 · after-th…`, kept the next-action
   hierarchy intact, and did not exceed the frame. The persisted draft was
   empty and the exact pane selection returned.
9. Recorded the reviewer report, a fresh local race-test check, and runnable
   build `/tmp/autarch-mvp-live` against exact external handoff
   `dad56ced162e5db157320914848e8f03`. No user acceptance verdict was recorded.

## Usability and correction effort

- Selecting a distant pane by repeated `a` presses was functional but costly;
  the separate all-session search remains the better discovery route.
- The first automation paste omitted tmux's literal-LF flag, converting
  newlines to carriage returns. Autarch rejected the control bytes before any
  agent input. Repeating the paste with literal LF produced the exact multiline
  confirmation and delivered successfully.
- Follow-up selection was initially one-way. Changing authority correctly
  required a full handoff, but returning from delta mode required reopening the
  app. The resulting correction makes `f` toggle back to fresh full context and
  adds a regression test.
- Claude Code had stale plugin-hook paths. The requested review still completed;
  no login change, plugin reinstall, permission response, or automatic retry was
  attempted.

## Evidence boundaries

- Agent-reported checks, independently rerun checks, and the human verdict are
  distinct fields in the durable external-handoff store. No Clavain `Execution`
  record was created. The current walkthrough result has no human verdict;
  acceptance remains with the user.
- The final independent governed boundary review reported **PASS** and exercised
  build, vet, race, and real isolated tmux checks. Its noted stale-readiness,
  local actor-attribution, and tracker-drift risks remain explicit rather than
  being represented as solved identity or provider telemetry.
- Autarch did not refresh authentication, change an account, purchase usage, or
  fall back to API billing. Per-task subscription consumption and comparative
  token savings were not measured and remain **unknown**.

## Final verification

- Governed independent Fable review of the complete send/interrupt boundary:
  **PASS**.
- Independent Claude Code review of the live-produced narrow-layout delta:
  **PASS**.
- Governed independent Fable review of the final narrow-layout and follow-up
  toggle delta: **PASS**.
- `go test -race -p 1 -count=1 ./...`: pass.
- `go vet ./internal/door ./pkg/agenttransport ./pkg/review`: pass.
- `go build ./cmd/...`: pass.
- One parallel full-race attempt reproduced the existing one-second asynchronous
  source-opening fixture timeout. That unchanged test passed 10 of 10 standalone
  runs, and the full controlled-parallelism run above passed without editing or
  weakening the fixture.
