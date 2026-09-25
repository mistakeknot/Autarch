### Reactions

- **Finding**: fd-architecture F5 (composer duplication)
  - **Stance**: agree
  - **Move Type**: defense
  - **Independent Coverage**: yes
  - **Rationale**: I flagged the same problem from the product side (my F4): the rail's Mycroft composer duplicates BB's own thread/spawn interface with no stated differentiation. fd-architecture's framing ("what does Mycroft's composer do that BB's spawn/tell doesn't?") is the implementation-level version of the same UX question — a second composer means the user has to learn and choose between two "start work" affordances for no stated reason, which is a discoverability and mental-model cost, not just a code-duplication cost.
  - **Evidence**: Doc lines 40-41, 88-89, 158 describe the composer three separate times without ever answering "why not just use BB's composer scoped by map selection." My own F4 traces this from the user-flow angle (no stated reason to exist); fd-architecture traces it from the code angle (no BB-native alternative ruled out). Both point at the same missing decision.

- **Finding**: fd-decisions NO-SUCCESS-CRITERIA
  - **Stance**: agree
  - **Move Type**: defense
  - **Independent Coverage**: yes
  - **Rationale**: This is my F2 restated from the decision-framing lens: "earn its space" (Key Decisions #8 / Status) is a vibe, not a metric, and there's no graduation test between the map+rail stage and whatever comes next. Two independent lenses landing on the identical gap raises my confidence this is a real P1, not an artifact of either agent's framing.
  - **Evidence**: My own F2 cites "Key Decisions #8, Status" for the same absence — no usage threshold, no "if mk ignores it for N days it's dead" rule, no defined "next stage" to graduate into. Doc's "Open Questions" section also never returns to define what "earn its space" would look like operationally.

- **Finding**: fd-architecture F2 (dispatch substrate mismatch)
  - **Stance**: partially-agree
  - **Move Type**: distinction
  - **Independent Coverage**: partial
  - **Rationale**: I flagged the bootstrap flow as jumping the dispatch gate (my F5) purely as a product/workflow-sequencing problem — bootstrap-from-rail seems to skip a step Key Decisions #8 otherwise requires. fd-architecture's claim is stronger and more foundational: if Mycroft's actual dispatch substrate is tmux sessions rather than BB threads, then "bootstrap new projects through the rail" isn't just skipping a step in an existing flow, it's assuming a rewritten dispatch layer the brainstorm never names as in-scope. That reframes my F5 from a UX sequencing gap into a feasibility gap — the flow I described as "jumps the gate" may instead be a flow that cannot exist against the current substrate at all. I can't independently verify the tmux-vs-BB-threads claim (that's a code fact outside my read of the doc), so I accept the architectural premise but keep my own contribution: even if the substrate question is resolved, the doc still doesn't tell the user what gate (if any) survives between "click bootstrap in the rail" and "an agent starts working," which is the part a user actually experiences.
  - **Evidence**: My F5 ("Key Decisions #8 vs. bootstrap-from-rail") targets the same doc passage (lines 40-41, 158) from the flow-completeness angle rather than the substrate angle — I treat it as an undefined transition, fd-architecture treats it as an undefined rewrite. Both readings can be true simultaneously.

## P2 Severity Checks

- fd-architecture F8 (Autarch door threads-screen duplication): Agree this is P2 for now, but it borders P1 if the doc ships without stating the TUI threads screen's fate — a user who has two "see my threads" surfaces with silently diverging data is a trust problem, not just cosmetic.
- fd-decisions PREMATURE-LOCK: Agree P2 — locking the pace-layer × ecosystem axis before validating the visualization helps decision-making is a real risk but reversible early, consistent with my own F7 concern about undefined pin semantics under future axis changes.
- fd-decisions OVERPLANNING-PATTERN: Should be considered alongside my F2 — no verdict from 103 prior plans plus no success criterion this time (my F2) compounds into a real repeat-failure risk, but as stated it's P2 since it's a process observation, not a blocking spec gap.
- fd-decisions MISSING-SIGNPOST: Agree P2, and it's the natural companion to my own P1 F2 (no success criterion) — without a success criterion there's nothing for a signpost to trigger on, so fixing F2 would likely resolve this one too.
- fd-perception FADE-001: Agree P2 — "ambient pressure without explicit alert" is real change-blindness risk but plausibly mitigated by the same instrumentation that would answer my F2 (success criterion) and F9 (first-run/dense-estate legibility); not independently blocking.
- fd-perception UNTOUCHED-001: This reads P1 in the doc's own severity note, not P2 — flagging for synthesis: "thread activity ≠ tending" directly undermines the neglect signal I called out in my P0 F1 (neglect data being real for barely half the estate); if the *available* neglect data is also mismodeled, F1's severity should not be discounted on the theory that "at least the covered half is accurate."

## Reactive Additions Index

None.

### Verdict

confirms-findings
