---
artifact_type: research
status: proposed-goal-and-open-design
date: 2026-09-05
bead: Sylveste-fuwn
---

# Proposed next goal: feedback during testing, carried through to a result

## User request

> what makes the most sense for the next goal? I also feel like part of the difficulty of growing/building software at this scale with agents is recording feedback; can we have Autarch leverage Computer Use + some sort of quick feedback feature in order to record my feedback while i am testing a site or app

This provides a concrete journey for the agreed product-manager workbench.
The recommendation is to focus the current brainstorm on this loop and make
it the next delivery goal after its consequential design decisions are settled.
This document is a proposal, not an implementation plan or a completed CUJ.

## Recommended goal

During a real test of one project, the human can record feedback without
losing their place. Autarch preserves the observation and its context; Flere
connects it to the relevant product guidance, proposes work or a reasoned
challenge, and carries the resolved direction through Clavain to a result the
human can review.

Use foundation onboarding to establish the guidance relevant to this one
project and test. This makes feedback the concrete piece of work that proves
the already-agreed foundation-to-delivery journey. It does not imply that
every foundation document must be finished before testing.

## Why this next

The tracker has two Autarch tasks in progress: this foundation/workbench
brainstorm (`Sylveste-fuwn`) and the older catch-up journey (`Sylveste-pilf`).
Neither reports a claimed-by or claimed-at value. The older task's notes leave
a human acceptance question open; it should not displace the user's subsequent
product priority. The project HUD and standalone build repairs are closed.

The following scores are analyst estimates, not measured delivery forecasts.
Effort 5 means multiple sessions; risk includes unproven integration work.

| Candidate within the current direction | Impact / effort / risk (1–5) | What it resolves and defers |
|---|---|---|
| One feedback-to-result loop, grounded in one project's foundations | 5 / 5 / 4 | Tests the missing human-to-agent interaction in daily work. Limits estate breadth while proving capture, interpretation, and follow-through. |
| Foundation authoring and reconciliation alone | 4 / 4 / 3 | Improves project guidance, but leaves the reported difficulty of recording feedback during testing unresolved. |
| More general reference research | 2 / 2 / 1 | Adds comparisons, but yields less evidence about whether Autarch changes the user's working experience. |

The earlier inventory and brief are useful starting points. Additional generic
research now has diminishing returns; the next questions should resolve this
specific journey.

## Proposed experience

1. Select the project and the app or site being tested. If review sessions are
   chosen, start one explicitly and show which surface it observes.
2. At a moment worth discussing, press a shortcut. A small capture surface
   preserves the screen and accepts a short note. Text, voice, and pointing at
   an element are interaction candidates; their first-version scope is open.
3. Save immediately and continue testing. Attach time, the selected app or
   page, screenshot, and available viewport/build context. Unknown context
   stays unknown. A short lookback is possible only if it was being recorded.
4. In Autarch, Flere groups related observations and connects them to existing
   persona, CUJ, roadmap, and design guidance. Keep the original feedback
   alongside any interpretation. Clarify consequential ambiguities at review
   time, without making every capture require a form or a conversation.
5. Resolve the next action: investigate a defect, explore a design alternative,
   update scoped guidance, defer, or challenge the premise with evidence.
   A spontaneous reaction is not automatically a permanent rule or a task.
6. Carry agreed work through Clavain. Bring back the changed app/site, the
   original observation, and verification evidence for a focused retest.
   Human acceptance and an agent's claim of completion remain distinct.

The user's earlier observation that Autarch felt “stark and bare” and that
`--since 24h` was unfriendly is already a useful example: preserve what they
saw and meant, connect the concern to usability, and review a concrete change.
Do not generalize that feedback into a universal aesthetic rule.

## What Computer Use contributes

During testing, capture visible state and available UI context. During a
separate reproduction or verification step, an agent can attempt the journey
and compare the result. Agent control must not compete with the human for the
same test surface. Replaying screenshots or DOM history is also distinct from
executing a reproducible test against a running app.

The likely Mac shape is a small native capture companion with a shortcut and
feedback surface, connected to the Autarch workbench. Browser-specific context
can enrich captures when available. The existing terminal UI alone does not
establish capture outside the terminal; implementation needs a host capability
for that interaction. This is an architectural hypothesis to validate.

Current feasibility evidence:

- Apple's [ScreenCaptureKit sample](https://developer.apple.com/documentation/screencapturekit/capturing-screen-content-in-macos)
  documents filtering capture to selected displays, applications, or windows,
  receiving video/audio samples, and updating a running stream. Its native
  permission flow is part of the capture UX. This establishes a platform
  building block, not a functioning Autarch recorder or rolling buffer.
- The local Flere RPC documentation supports image-bearing prompts, and its
  extension documentation describes custom tools and UI requests. A capture
  can plausibly be passed through that interface, subject to model image
  support and an actual round trip. The
  [Flere assessment](assess-flere-autarch-native-agent.md) still records
  unfinished Clavain integration; its older runtime findings were not rerun
  during this proposal.
- Computer History's status tool returned `stopped` in this session. No
  recording or observation settings were changed, and no activity stream was
  read. The presence of Codex Computer Use/History tools is not evidence of a
  redistributable API that Autarch can embed. Treat reuse as a separate
  integration question, with a native capture path as an alternative.
- A bounded search of Autarch's Door and agent-target packages found no quick
  feedback capture implementation. Alwe's file-context lookup was degraded
  with no local hits; it does not establish absence of other prior art.

## Focused reference mechanisms

These are analyst-selected examples for the requested capture capability,
based on primary documentation; no live product teardown was performed.

- [Jam Instant Replay](https://jam.dev/docs/instant-replay) documents an opt-in
  local lookback of up to two minutes, triggered from the extension or a
  shortcut, with a preview before sharing. It reconstructs DOM activity;
  it does not record arbitrary native-app screen video. This supplies the
  tradeoff between catching the lead-up and needing capture enabled beforehand.
- [Marker.io's browser extensions](https://help.marker.io/en/articles/6495644-browser-extensions)
  demonstrate comments and screenshots captured while visiting a page.
  Optional diagnostic collection requires configuration. This supplies the
  tradeoff between quick feedback and the setup needed for richer evidence.

## Decisions to settle before planning

- **Capture rhythm:** explicit review session with marked moments, or a global
  shortcut that captures only the current moment. The structured question is
  pending; the recommendation is a selected-surface review session with a
  shortcut and a bounded lookback.
- **Input:** text, voice, pointing/markup, and how a saved capture confirms
  success without disrupting the test. Saving must remain useful when Flere
  is unavailable.
- **Observation scope:** selected app/window/tab, visible pause/stop, lookback
  duration, local retention, and what gets included when feedback is handed
  to an agent. Review and removal should be available from the capture.
- **Triage and authority:** which feedback can become routine scoped work,
  which needs product discussion, when an agent challenge waits for the human,
  and how changed guidance affects other active work.
- **Pilot and proof:** choose one actual project and build, capture real
  feedback, and retest the resulting change. Demonstrate that agents apply
  the resolved guidance in a later attempt without the human restating it.

The first delivery should cover one complete loop. Cross-platform recorders,
universal replay, and rollout across the whole estate are separate scope
choices. No new goal widget, implementation plan, or recording was started.
