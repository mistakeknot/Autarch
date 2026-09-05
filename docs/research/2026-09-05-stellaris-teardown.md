---
artifact_type: teardown
status: research-for-provocation
evidence_class: primary (published-release-notes)
date: 2026-09-05
bead: Sylveste-fuwn
---

# Stellaris teardown: keeping a workbench navigable

## Lineage and scope

The [August 31 teardown](2026-08-31-stellaris-teardown.md) records the user's
reason for this reference: “context switching and deep diving and then
watching/waiting for intervention points”. It also records losing the thread
and interrupt fatigue among the failure modes to avoid.

This supplement examines view customization now that the user has selected a
project workbench. It does not refresh every claim in the earlier teardown or
assert what the latest Stellaris release contains.

## Evidence and coverage

Read Paradox's [November–December 2023 release notes on Steam](https://store.steampowered.com/news/posts/?appids=281990&enddate=1702648565&feed=steam_community_announcements)
on September 5, 2026. The relevant sections are Dev Diaries 324 and 325 and the
3.10.1 release notes. This is historical primary evidence, not live gameplay.

The publisher describes selecting a custom Outliner tab and choosing its
contents. The same information can appear in several tabs or together on one;
their example separates peace and war views. The release history also adds
keyboard tab navigation and fixes new-entry indicators not clearing when tabs
are visited through shortcuts. [Evidence sidecar](2026-09-05-stellaris.findings.json).

Coverage: one published archive, three relevant release entries, zero live
game sessions. No screenshots, timing, current controls, or accessibility were
measured. No new claim about player satisfaction follows from these notes.

## Choice → tradeoff → Autarch journey

| Documented choice | Inferred benefit and cost | Autarch relevance |
|---|---|---|
| Select which information appears in each tab | A view can suit the current activity; information outside it becomes easier to overlook. | Daily walk, steps 2–3: survey projects and notice what waits on the human. |
| Repeat information across tabs, or collect it on one | Cross-cutting concerns can remain visible; repetition and configuration can become their own clutter. | Session dive, steps 2–4: orient, work, and steer. A pending foundation decision may matter while reading either the roadmap or sprint progress. |
| Keyboard navigation updates new-entry indicators | Entry by shortcut has the same acknowledgement effect; inconsistent routes make attention markers unreliable. | Returning to work after Flere or Clavain updates. Opening a view must be distinguished from answering a decision. |

Journey references: [daily walk](../cujs/autarch-01-daily-walk.json) and
[session dive](../cujs/autarch-02-session-dive.json).

## Hypotheses for the next dialogue

The comparison to test is an integrated project overview against views focused
on foundations, current work, and decisions. The former supports orientation;
the latter gives each activity more room but creates places to forget.
Customizable views are one possible response, with an additional setup cost.
The user has chosen workbench-first, not a particular tab structure.

A concrete scenario can expose the tradeoff: while reviewing a roadmap,
Clavain returns a question that conflicts with a persona assumption. Compare
keeping the question and its supporting context beside the roadmap with
moving into a dedicated decision view. Determine what preserves the thread
before choosing a layout.

The game's pause mechanism does not establish permission to pause external
agents. Autarch's existing orientation-before-obligation ruling still applies.
These hypotheses await dialogue; no new CUJ or implementation plan is derived.
