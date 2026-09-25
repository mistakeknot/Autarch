### Reactions

- **Finding**: F2
  - **Stance**: agree
  - **Move Type**: defense
  - **Independent Coverage**: partial
  - **Rationale**: This is a textbook map/territory failure — the brainstorm's model (BB threads as dispatch substrate) is presented as the operative reality when the actual substrate (tmux sessions per fd-architecture) is different, and the gap is never named, let alone reconciled. My review's core check ("does the document acknowledge the gap between its model and reality?") is failed here in the same pattern I flagged for layer semantics (PARADIGM-001) and dependency data (GRAPH-001): the document narrates a single confident model without surfacing the divergence as an open question.
  - **Evidence**: own findings PARADIGM-001, MAP-001, GRAPH-001 (same document-wide pattern: stated model treated as settled fact); fd-architecture F2's code-level claim is outside my read-only scope and unverified independently.

- **Finding**: F3
  - **Stance**: agree
  - **Move Type**: defense
  - **Independent Coverage**: partial
  - **Rationale**: This is reification in a purer form than what I flagged — the document doesn't just have incomplete data (my GRAPH-001/MAP-001: 5 edges vs ~98 gardens, 47/98 missing ecosystem data), it treats a placement interface (`GraphSource`) as an existing dependency the map can read through when, per fd-architecture, it doesn't exist in code at all. That's a stronger claim than mine: not "the model is under-populated," but "the model has no implementation to be populated from." If true, it upgrades my P1s from a data-completeness problem to a does-not-exist-yet problem.
  - **Evidence**: own findings GRAPH-001, MAP-001 (established the map's dependency/ecosystem model is unreliable); F3's specific claim that `GraphSource` is absent from code is an architecture-domain fact I cannot verify directly but which is consistent with and would deepen my findings.

- **Finding**: F1
  - **Stance**: partially-agree
  - **Move Type**: distinction
  - **Independent Coverage**: partial
  - **Rationale**: I accept the pattern this finding describes — a stated invariant ("Autarch holds no state") functioning as an unexamined mental model rather than a verified fact, which is exactly the map/territory failure mode I flagged in DOCS-001 (metadata/filename signals treated as ground truth about neglect). I reject that I can independently confirm the code-level claim itself ("already false in the existing code") since my review is document-only; I can only confirm the document never states this as an assumption to be checked, which is the perception-lens half of the finding.
  - **Evidence**: own finding DOCS-001 (document treats an unverified signal as settled truth); no code access to confirm or deny the specific "holds no state" claim.

## P2 Severity Checks

- F7: Agree this is at least P2 for architecture's framing, but from an information-quality lens it should track with my P1 GRAPH-001/MAP-001 rather than be scored independently lower — the same 5-edges/~98-gardens gap that makes the dependency model P1-incomplete in my review doesn't become less consequential because it's "known and carried forward."
- PREMATURE-LOCK: Agree this is P2 — it's a real model-lock-in risk (pace-layer × ecosystem axis fixed before validating utility) but it's one instance of a broader pattern already captured at P1 in PARADIGM-001, so P2 as a standalone item is reasonable.
- OVERPLANNING-PATTERN: Should be considered alongside signal/noise quality — 103 prior plans with no recorded verdict is a real "what missing data would change the conclusion" gap, but P2 is fair since it's a process-history observation rather than a live data-integrity defect like MAP-001/GRAPH-001.
- MISSING-SIGNPOST: Agree this is P2 — absence of a "if X at date Y, pivot to Z" trigger is a real sensing-mechanism gap (same family as my GLOW-001/FADE-001 change-blindness concerns) but doesn't independently block the current design the way the P1 data-completeness items do.

## Reactive Additions Index

- P1 | PERCEPT-R1 | "Multiple: Key Decisions / What We're Building / Position Means Fixed Regions" | Peer findings F1–F3 (fd-architecture) collectively surface a document-wide reification pattern beyond what I scoped: the brainstorm repeatedly narrates unimplemented or contested interfaces/rules (`GraphSource`, "holds no state," BB-thread dispatch) as if they were already-operative reality, without flagging them as open assumptions. This is broader than my individual MAP-001/GRAPH-001/PARADIGM-001 findings, which focused on data completeness and semantic drift — it suggests the map/territory gap is systemic to the document's narrative voice, not confined to the map feature itself. (provenance: reactive)

### Verdict

adds-evidence
