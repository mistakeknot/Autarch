---
artifact_type: plan
bead: Sylveste-pilf
stage: implementation
---
# Finish the daily catch-up

Goal: after a day away, mk understands what changed and what needs a decision
in about a minute, can inspect the evidence, and can open the original session
to answer. This executes autarch-01 v1.5 and the entry portion of autarch-02.

User ruling, 2026-09-04, Codex session 01a06e87-a950-7933-9db4-6b576cbf8f79:
“Show the question and supporting context in Autarch, then take me to the
original session to answer.” No replies are sent by the catch-up view.

Alignment: renders evidence from the existing world and makes one daily
workflow usable. Autarch keeps no decisions or session truth of its own.
Conflict/Risk: historical questions must not masquerade as live requests;
an idle terminal prompt is not evidence that a human decision is pending.

## Must-haves

- A fresh checkout builds without neighboring Sylveste repositories or an
  untracked go.work. CI tests the same dependency versions.
- Bare autarch opens with changes, not a wall of absent product cards.
- Transcript context survives an agent stopping. Provider identity, source
  path, timestamp and quoted text remain visible; runtime is a separate fact.
- Only unresolved questions supported by source evidence earn a decision row.
  Replied-to questions and working agents do not become false waiting alerts.
- Selecting a request opens its question and supporting context first. A
  separate explicit action opens the exact original session.
- Failed reads are visible, and quiet projects do not appear merely because
  an old dirty file still exists. Existing project and thread views remain.

## Evidence and prior learnings

The September 4 review reproduced: 103 project rows, 53 activity rows, 43
existing tmux seats and zero project-attributed sessions. ReadThreads only
looked up transcripts when the current command classified as Claude. The
briefing listed dirty-only projects regardless of when the files changed.
The focused door tests pass with normal tmux access; standalone main CI fails
on sibling replace paths. Existing statedetect patterns mistake generic
input prompts for questions, so they cannot establish pending decisions alone.

## Task 1: land the existing work with reproducible dependencies

Files: go.mod, go.sum, .github/workflows/ci.yml, agents/development.md.
Track build repair in Sylveste-i3r3. Integrate cuj-lineage onto main in a clean
checkout; pin published module revisions, remove sibling replacements, use
go-version-file in CI, and install tmux there for the actual interaction test.
Reproduce with GOWORK=off go build -mod=readonly ./cmd/autarch, then verify
go test -race ./internal/door and the CI-equivalent build/test commands.
Commit the dependency repair and push main before building the next feature.

## Task 2: read conversation evidence independently of runtime

Files: internal/door/transcript.go, conversation.go, conversation_test.go,
threads.go, threads_test.go, threads_view.go, cmd/autarch/threads.go.
Add bounded, provider-aware Claude/Codex transcript reading. Preserve the last
actual user/assistant exchange, unresolved structured questions, and source
provenance. Look up historical context for stopped named sessions; never
attribute an old Claude conversation to a currently running Codex process.
Test real-format fixtures: answered questions disappear, tool results and
bookkeeping do not count as replies, no old question leaks after later work,
stopped threads retain context, and errors/timeouts remain explicit.

## Task 3: the catch-up and question detail

Files: internal/door/catchup.go, catchup_test.go, model.go, briefing.go,
briefing_test.go, threads_model.go, threads_view.go, cmd/autarch/door.go.
Use the same parsed evidence for the opening summary, an explicit needs-you
view, and full question detail. Keep orientation before action. Show the last
substantive reported outcome beside commit evidence; label transcript quotes
as reports, not verified delivery. Move dirty-only rows out of the opening
changes view. Support scroll, back, refresh, and exact-session handoff.
Test narrow terminals and selection stability while background reads finish.
No pending question may be silently dropped by a viewport cap.

## Task 4: verify the actual return to work

Files: internal/door/tmux_capture_test.go, docs/cujs/autarch-01-daily-walk.json,
docs/research/2026-09-04-catch-up-validation.md, README.md.
Drive the real binary in isolated tmux: opening summary, pending question,
evidence, exact source-session switch, and answered question removed on refresh.
Run against the real estate and inspect coverage, latency and usefulness;
record aggregate evidence without publishing private transcript text. Show mk
the actual screen for the one-minute recognition check. Build/install from
the tested commit, push main, inspect current CI, and close the beads only
when their respective outcomes hold.

## Product questions

- Stopped conversation entry: pending mk's choice. Evidence reading and live
  session switching proceed independently; do not invent a resume policy.
- Ask further product/design/taste questions through the structured question
  tool. Previously settled garden-salon and orientation rulings still apply.

These tasks are sequential and share model state; execute in this session.
