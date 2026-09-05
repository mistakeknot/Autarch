---
artifact_type: teardown
status: research-for-provocation
evidence_class: primary (published-documentation)
date: 2026-09-05
bead: Sylveste-fuwn
---

# Gmail teardown: choosing how much work to see

## Lineage and scope

The user named Gmail for presentation choice:

> let's do both as options to test/choose from? similar to cozy/compact settings in gmail, for examle

This supports density customization as a reference. It does not establish a
preference for Gmail's inbox organization or its whole interaction model.
No Gmail frustration was reported; the costs below are analyst inferences.

## Evidence and coverage

Read two official help pages on September 5:

- [Google Workspace: reading and sending tips](https://support.google.com/a/users/answer/11339703?hl=en),
  specifically the inbox-density instructions.
- [Gmail: personalize your inbox](https://support.google.com/mail/answer/9259769?hl=en),
  specifically Quick settings and reading-pane configuration.

The documented sequence is: open Gmail, enter Settings, locate Density, and
choose Default, Comfortable, or Compact. Google also documents attachment
previews for Default. Reading-pane placement has its own choices: no split,
right, or below. These are documented controls, not observed task completion.

[Evidence sidecar](2026-09-05-gmail.findings.json). Coverage is two public
documents; no mailbox was opened or changed. The agent-browser prerequisite
failed with a registry DNS error. Screenshots, CSS tokens, accessibility,
keyboard behavior, focus retention, and preference persistence were not
measured. A live mailbox walkthrough remains outside this pass.

## Choice → tradeoff → Autarch journey

| Documented choice | Inferred benefit and cost | Autarch relevance |
|---|---|---|
| Named density presets in Settings | A small set is easy to discover and compare; labels alone do not explain everything that changes. | Daily walk, step 2: scan the estate. Offer a visible way to compare Cozy and Compact. |
| Density associated with attachment-preview visibility | More context is available before opening an item; the modes can differ in information, not only spacing. | Daily walk, steps 2–3: seeing pending decisions. Decide which evidence and status must remain visible in both modes. |
| Reading pane configured separately | Layout can suit the task independently of density; another preference adds setup choices and combinations. | Session dive, steps 2–3: orient and work. Cozy/Compact should not silently settle where Flere and the artifact appear. |

Journey references: [daily walk](../cujs/autarch-01-daily-walk.json) and
[session dive](../cujs/autarch-02-session-dive.json). The workbench ruling
extends the latter; this teardown does not amend the validated specification.

## Hypotheses for the next dialogue

- Let the user compare both densities against the same selected project.
- Keep decision state and required action discoverable in either density;
  supporting excerpts may expand or collapse.
- Treat a conversation beside an artifact as a layout decision in its own
  right, including how it behaves in a narrow terminal.

These are candidates to provoke with, not agreed features. The unresolved
tradeoff is faster scanning versus more context before opening an item. A
later comparison should use the same real project and pending decision in both
modes, so the user can judge what each makes easier to notice.
