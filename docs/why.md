---
artifact_type: card
card_version: 1
project: autarch
status: confirmed
confirmed_by: mk
confirmed_at: 2026-09-09
ratification:
  ruled_by: mk
  ruled_at: 2026-09-09
  ruling: "Record the accepted direction in Autarch’s canonical product documents, preserving earlier decisions and explicitly documenting amendments"
  transcribed_by: "Codex Astra integrator, from the user-supplied implementation plan"
  session_id: "01a084f9-5422-7501-aa29-8ac0b4b8ea94"
  source: "docs/decisions/2026-09-09-human-ratification.md"
  supersedes: "docs/why.md@2e8968536e6f34b5bc54081f0c8e099e412a94eb"
line: "Direction for agent-built projects"
fields:
  persona:
    state: confirmed
    value: "An experienced product leader directing interconnected software and research projects through agents."
    ratification_ref: "docs/decisions/2026-09-09-human-ratification.md"
    supersedes: "docs/why.md@2e8968536e6f34b5bc54081f0c8e099e412a94eb#fields.persona"
    evidence:
      - { path: "docs/personas.md", scope: project }
      - { path: "docs/decisions/2026-09-09-human-ratification.md", scope: project }
  pain:
    state: confirmed
    value: "Agents work across the gardens around the clock while the gardener is away, and nothing shows what moved or what waits on them. Standing chat threads stand in for the walk, the gardener is the only bus carrying context between gardens, and those threads die by compaction and lose what they knew"
    ratification_ref: "docs/why.md@2e8968536e6f34b5bc54081f0c8e099e412a94eb#fields.pain"
    evidence:
      - { path: "docs/research/2026-08-31-session-archaeology.md:110-126", scope: project }
  cuj:
    state: confirmed
    ref: "autarch-06-reviewed-project-visit"
    path: "docs/cujs/autarch-06-reviewed-project-visit.md"
    value: "One project visit that produces an independently reviewed implementation plan for a meaningful outcome. Implementation and live validation remain pending."
    ratification_ref: "docs/decisions/2026-09-09-human-ratification.md"
    supersedes: "docs/why.md@2e8968536e6f34b5bc54081f0c8e099e412a94eb#fields.cuj"
    evidence:
      - { path: "docs/cujs/autarch-06-reviewed-project-visit.md", scope: journey }
  success:
    state: confirmed
    value: "Observable progress on chosen outcomes. Reduced context switching and wasted agent work support that result."
    reason: "Qualitative success direction accepted on September 9; no measured improvement or numerical threshold is asserted."
    ratification_ref: "docs/decisions/2026-09-09-human-ratification.md"
    supersedes: "docs/why.md@2e8968536e6f34b5bc54081f0c8e099e412a94eb#fields.success"
    evidence:
      - { path: "docs/decisions/2026-09-09-human-ratification.md", scope: project }
  guardrail:
    state: confirmed
    value: "Project files remain authoritative. Retain original conversations and attributed source records independently of the application installation; the graph is a rebuildable projection. Exploratory answers are not rulings, and accepted guidance or completed review cannot start implementation."
    ratification_ref: "docs/decisions/2026-09-09-source-and-graph-ownership.md"
    supersedes: "docs/why.md@2e8968536e6f34b5bc54081f0c8e099e412a94eb#fields.guardrail"
    evidence:
      - { path: "docs/decisions/2026-09-09-source-and-graph-ownership.md", scope: project }
      - { path: "docs/decisions/2026-09-09-planning-and-execution.md", scope: project }
decisions:
  - "docs/decisions/2026-09-09-human-ratification.md"
  - "docs/decisions/2026-09-09-source-and-graph-ownership.md"
  - "docs/decisions/2026-09-09-sourced-working-context.md"
  - "docs/decisions/2026-09-09-planning-and-execution.md"
  - "docs/research/2026-08-31-vision-capture.md"
  - "docs/research/2026-08-31-estate-ontology.md"
  - "docs/decisions/2026-02-01-spec-vs-prd-canonical-type.md"
---

# Why Autarch

Help you choose worthwhile work, give agents coherent direction, and judge what
they deliver. The first proving journey is one Autarch project visit ending in
a reviewed implementation plan. The workbench and runtime journey remain in
implementation; this card records direction, not a completed user visit.

The [mission](../MISSION.md), [vision](autarch-vision.md), [persona](personas.md),
[philosophy](../PHILOSOPHY.md), [roadmap](roadmap.md), and
[design standards](design/standards.md) form the current product foundation.
The [September 9 amendment](decisions/2026-09-09-human-ratification.md) preserves
the earlier gardener and tending-journey lineage while recording the new focus.
The commercial hypothesis remains untested and the guidance-retention benchmark
has not yet established a quality or cost result.

## September 2 card and ratification — verbatim history

The historical success field was deliberately declined. The September 9
instruction supersedes that field with qualitative direction. The old card,
including its original human ruling and source references, is preserved below.
Its present-tense descriptions refer to that earlier checkpoint.

```text
---
artifact_type: card
card_version: 1
project: autarch
status: confirmed
confirmed_by: mk
confirmed_at: 2026-09-02
ratification:
  ruled_by: mk
  ruled_at: 2026-09-02
  ruling: "Stamp as drafted: persona, pain, cuj and guardrail confirmed as written; success stays declined. mk's answer to the structured question: '1 - also it may be good to include session id / agent used for further attribution'"
  transcribed_by: "Claude Fable 5.1 (claude-fable-5-1)"
  session: "https://claude.ai/code/session_01PrZont6NaR2LzfjVbwK1CN"
  session_id: "5920c9b1-6a3f-4a7d-8566-e6067aaeaf01"
  goal: "intercore e961b2a6"
line: "Solo gardener of ~98 gardens that never halt"
fields:
  persona:
    state: confirmed
    value: "Solo gardener of a ~98-project, agent-heavy, terminal-native estate: one human governing their own gardens while agents run in them around the clock"
    evidence:
      - { path: "docs/cujs/autarch-01-daily-walk.json:5", scope: journey }
      - { path: "docs/cujs/autarch-02-session-dive.json:5", scope: journey }
      - { path: "docs/cujs/autarch-03-seasonal-reshaping.json:5", scope: journey }
      - { path: "docs/cujs/autarch-04-canon-tending.json:5", scope: journey }
      - { path: "docs/research/2026-08-31-vision-capture.md:10-13", scope: project }
      - { path: "README.md:18", scope: project }
  pain:
    state: confirmed
    value: "Agents work across the gardens around the clock while the gardener is away, and nothing shows what moved or what waits on them. Standing chat threads stand in for the walk, the gardener is the only bus carrying context between gardens, and those threads die by compaction and lose what they knew"
    evidence:
      - { path: "docs/research/2026-08-31-session-archaeology.md:110-126", scope: project }
      - { path: "docs/research/2026-08-31-session-archaeology.md:56-64", scope: project }
      - { path: "docs/research/2026-08-31-stellaris-teardown.md:9-15", scope: project }
      - { path: "docs/cujs/autarch-01-daily-walk.json:6", scope: journey }
      - { path: "README.md:7", scope: project }
  cuj:
    state: confirmed
    ref: "autarch-01-daily-walk"
    path: "docs/cujs/autarch-01-daily-walk.json"
    value: "The daily walk: the fast-layer journey and the most-exercised one. It is one of four validated journeys on the tending-cadence spine (walk, dive, seasonal reshaping, canon tending); this field holds the one the shipped surface serves"
    evidence:
      - { path: "docs/cujs/autarch-01-daily-walk.json:2", scope: journey }
      - { path: "docs/cujs/README.md:9-21", scope: journey }
  success:
    state: declined
    reason: "Nothing in the repo states a project-scope measure. The four CUJ success_conditions are per-journey recognition tests (scope: journey), and the vision statement names a direction without a number. The repo does record what Autarch exists to dissolve, with an August 2026 baseline: four daily companion threads, 15 of 16 long threads spanning many gardens, 16 of 16 never parked"
    needs: "mk's own statement of how they would tell it worked. One candidate the baseline could bound: the number of standing companion threads mk keeps alive, direction down, as the walk, parking and cross-garden discourse land"
    evidence:
      - { path: "docs/research/2026-08-31-session-archaeology.md:128-137", scope: project }
      - { path: "docs/research/2026-08-31-session-archaeology.md:44-50", scope: project }
  guardrail:
    state: confirmed
    value: "Autarch never becomes a system of record. It transcribes cards, sessions and the graph and holds no world state; world edits flow through it as mk's attributed acts, and deleting it loses only local preferences such as pins and the last-visit stamp"
    evidence:
      - { path: "docs/research/2026-08-31-vision-capture.md:17-22", scope: project }
      - { path: "docs/research/2026-08-31-estate-ontology.md:16-18", scope: project }
      - { path: "docs/cujs/autarch-01-daily-walk.json:46", scope: journey }
      - { path: "docs/cujs/autarch-04-canon-tending.json:47", scope: journey }
decisions:
  - "docs/research/2026-08-31-vision-capture.md"
  - "docs/research/2026-08-31-estate-ontology.md"
  - "docs/decisions/2026-02-01-spec-vs-prd-canonical-type.md"
---

# Why Autarch

Autarch is the HUD over one person's estate of projects: about a hundred
gardens, most of them repositories, with agents working in them around the
clock. The gardener cannot watch all of them, and the world never pauses, so
Autarch stops the gardener instead. Opening it shows what moved since the last
visit before anything asks for a decision. Then the rows, quiet unless
attention-worthy. Then enter a garden, or close it owing nothing.

It owns nothing. Cards live in the repositories as `docs/why.md`, sessions live
in tmux and the transcript store, the estate's structure lives in the graph.
Autarch transcribes them and holds no world state. The preferences it keeps
locally are pins and the time of the last visit.

Four journeys make one spine (`docs/cujs/README.md`): the daily walk, taken in
minutes several times a day; the session dive, an hour or an evening in one
garden; seasonal reshaping, at season boundaries; and canon tending, rare and
permanent. All four were walked with mk and validated on 2026-09-01. The walk's
first organ, the catch-up briefing, shipped the same day.

What is not yet known is how anyone would tell it worked. The repo records what
Autarch exists to dissolve, the standing chat threads that today do the walk's
job by hand, and an August 2026 baseline for them. No number has been named, so
the success field says so rather than guessing.

A note for the ruling: `README.md` still describes Autarch as four operational
tools and a dashboard layer, written in January 2026. The vision capture of
2026-08-31 supersedes that framing. The README has not been rewritten.
```
