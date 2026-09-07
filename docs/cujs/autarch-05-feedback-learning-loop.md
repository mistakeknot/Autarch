# autarch-05-feedback-learning-loop

Design status: accepted for implementation, 2026-09-05. Live validation: pending.
Persona: the human guiding product, design, strategy, taste and discernment
while engineering agents perform scoped work. Source: [accepted rulings](../brainstorms/2026-09-05-feedback-learning-loop.md).

1. Open the project workbench and establish guidance relevant to the next slice.
2. Invoke the companion from the terminal workbench. It selects that terminal
   window and retains the project, review view and tmux pane context. Confirm
   the target and start a local review recording; select manually when the
   source is unavailable.
3. Test while adding typed and spoken observations through a shortcut. Correct
   a transcript without losing the original audio. See saved/recording status.
4. Close and reopen the TUI while recording continues; reconnect to the same
   review. Pause, resume, stop and play back the retained session.
5. Review Flere's outcome groups, observations, evidence, uncertainties and
   challenges. Answer a structured question and immediately see the next one.
6. Inspect the proposed change and enduring guidance, scope and rationale.
   Accept exactly that revision; see queued/deferred/running/blocked/failed state.
7. Follow evidence from the original moment through work and execution to the
   actual build. Read a short retest checklist and try that build.
8. Record the verdict. In a later related task, observe the accepted guidance
   already in context and applied, with any evidence-backed challenge visible.

Recognition condition: the human can trace their intervention to a changed
product and a subsequent better-informed attempt without repeating the ruling.
This does not replace the deliberately declined project-wide success field.

Failure paths: agents unavailable, incomplete history, transcription failure,
storage failure, interrupted recording, stale approval or question, uncertain
project, duplicate retry, blocked dependency and ineligible model all remain
visible. Acknowledged feedback is retained; failed writes never claim saved.

User ruling, 2026-09-07: the companion should auto select the window and context
of the terminal or tmux workbench it was invoked from. Invocation identifies a
window before the companion takes focus. A later focus change must not select
another window of the same terminal app. The global shortcut refreshes the
foreground terminal target; it reuses a known tmux context only while the original
clients still display that pane. Unknown terminal context goes to intake for
assignment, and an existing draft must be saved before switching its source.
Automatic selection does not start recording or accept a proposed change.
