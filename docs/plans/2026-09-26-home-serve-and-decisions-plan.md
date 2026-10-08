---
artifact_type: plan
bead: mk-okek
stage: design
revision: 4
requirements:
  - S0: one read-only `autarch serve` (build step 0)
  - S1: decisions owed, picked and ruled, in the Home plugin (build step 1)
  - S1v: the vizier in Home (catch-up, delegated rulings, the vizier's conversation)
---
# Home, steps 0 and 1: `autarch serve`, decisions and the vizier

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Bead:** `mk-okek` (Home epic, hub tracker, filed 2026-09-26 under G-0). S0 is `mk-okek.1`
and S1 is `mk-okek.2`; S1 depends on S0. The vizier work is a revision to this epic, not a new
one (vizier brainstorm, Prior Art).

**Revision 4** (2026-09-29). It folds in review round 3 (C-1 to C-14), brainstorm decisions 24
and 25, the vizier brainstorm (decisions 1–12), the vizier spike, the approvals question and
the success-measure baseline. Where each item landed is in "Revision 4 changes" below.
Review rounds 4 to 10 (D-1 to D-17, E-1 to E-13, F-1 to F-8, G-1 to G-7, H-1 to H-6, I-1
to I-4, J-1) are folded into this revision in place; see the per-round dispositions tables.

**Rulings (mk):**
- 2026-09-26: G-5 accepted (P-7 feed via `agents.configure`; P-5 wake scope). G-9: mark
  undeliverable in v1; successor resolution deferred.
- 2026-09-26: **Command picks are cut from v1, and signing is deferred to step 3 (Lattice).**
  An option can carry a pre-written **instruction**. Picking it wakes the asking agent with
  exactly that text, and the agent does the work under its own sandbox and gates. The feed is
  built from Home's own pick records, not from ruling files. Ruling files stay as the readable
  record, unsigned. This revises `[D14]`, `[D16]`, `[D18]`, `[D21]` and `[D22]` for v1, and
  supersedes G-2, G-7 and G-8.
  - mk's reason: "I just want to context switch less and think more deeply about
    product/design/taste problems (and QAing what I should be QAing)."
- Decision 24 (brainstorm, 2026-09-29): **the Home plugin owns decisions**, in its own SQLite
  database. `autarch serve` is read-only for decisions. Hub beads are at most a one-way copy,
  never read back. This revises brainstorm decisions 3 and 9 and the "one queue" item of
  decision 21, and removes rev 3's bd tracker, owner lock and decisions API.
- Decision 25 (brainstorm, 2026-09-29): structured turn endings, one entry per ask with
  mentions, a machine-blocker lane, one runbook, standing rulings after the trial (P-11).
- Vizier decisions 1–12 (2026-09-29): the vizier is Home plus a summoned overlay (P-10,
  Tasks 1.6, 1.8, 1.12).
- mk (2026-09-29): approvals "should be a core functionality in Aleph". Where the record lives
  was ruled by mk on 2026-09-29 (G-11): **Aleph core** (P-12).

**Goal:** mk sees what agents owe mk in one full-page Home tab and picks in place. A pick is
recorded once, even when retried or interrupted, and the answer (with the asker's
pre-written instruction, if any) reaches the asking thread at least once, or is shown to mk
as uncertain with Resend and Dismiss. A caller claiming the vizier thread can rule
in mk's place only on options the asker marked reversible and the bounded delegation policy
allows, and mk sees and can override every such ruling. Who picked (`mk`, `vizier`) is
**advisory attribution** until Aleph provides a host-attested caller (G-15); no record here
authorizes a merge, deploy or release. Each pick leaves a ruling file.

**Architecture:**
- `autarch serve` is one loopback, token-guarded Go service. It mounts the existing Bigend,
  Gurgeh and Signals handlers under prefixes, shares one Signals broker, and adds a read-only
  `GET /api/projects`. It holds no decision state and has no decision writer.
- The Home plugin (`integrations/bb-plugin-autarch/`) owns decisions. Its server code runs
  inside bb's server process (`plugin-runtime.ts:1644`). It stores decisions in
  `bb.storage.database()`, a better-sqlite3 database at `<dataDir>/plugins/<id>/data.db` in WAL
  mode. Filing, picking and finishing an obligation are each one transaction.
- The plugin also delivers wakes through `bb.sdk.threads`, builds the feed for
  `bb.agents.configure`, registers the `bb home` CLI, and renders Home.
- Agents file with `bb home ask`. Go callers (`autarch decide file`, the MCP tool, Mycroft) file
  through `internal/homeask`, which runs `bb home ask`. One JSON schema and one set of
  canonicalization vectors are tested on both sides.
- Home executes nothing. A pick is one row update plus **obligations** (write the ruling file,
  wake the asker). One reconcile function carries out obligations.
- MCP moves into the one binary as `autarch mcp`, still speaking stdio.

**Tech Stack:**
- Go 1.25: `net/http` ServeMux patterns, cobra, `os.Root`.
- The bb plugin SDK: TypeScript, `bb.server` entry, `bb.storage.database()` (better-sqlite3,
  `backend-contract.ts:190`), React `navPanel` (`app-contract.ts:70`, "owns its whole route"),
  the host `ThreadChat` component, `testing/fake-sdk.ts`.
- `bd` is no longer on the write path. It is read only by Mycroft's existing `bd ready`.

**Prior learnings / inputs:**
- [Brainstorm](../brainstorms/2026-09-24-one-place-in-aleph-brainstorm.md), decisions 13–25,
  cited `[Dn]`. They are binding except where the rulings above narrow them.
- [Vizier brainstorm](../brainstorms/2026-09-29-vizier-window-brainstorm.md), cited `[Vn]`.
- [Vizier spike](../research/2026-09-29-vizier-window-spike.md) (OQ1, OQ2, approvals).
- [Baseline](../research/2026-09-29-vizier-window-baseline.md) (OQ6).
- [bd write guarantees](../research/2026-09-26-bd-write-guarantees.md). It confirmed C-1 (a
  duplicate `create --id` overwrites and reopens the bead, erasing the pick) and C-2 (metadata
  plus label updates tore in 7 of 200 kill -9 runs). It is why decisions left bd.
- Prototype `<thread-storage>/thr_uqy4fzn88x/reports/home-stack-prototype.html` (layout).
- [One-pager](../onepagers/2026-09-24-one-place-in-aleph.md); CUJs
  [autarch-07](../cujs/autarch-07-decide-and-continue.json) and autarch-09 (autarch-07 is
  revised when G-6 walks it).
- [Estate-map trial plan](2026-09-23-estate-map-trial-plan.md), WI-5, which later puts the real
  map into this plugin.

---

## Decision context (routing)

```json
{"reasons":["foundational-invariants","broad-consequences"],
 "rationale":"Home decisions are a shared protocol for every agent, Mycroft, the vizier and the rail; picks carry mk's approval of instructions agents act on, delegated rulings let the vizier act in mk's place, and approvals are proposed as an authorization record other enforcers trust.",
 "investigation_active":false,"domain":"agent"}
```

- **Planning role:** resolved to planning-astra (gpt-6-astra), fallback planning-opus. This
  revision was authored by **claude-opus-5-5**, the running session and the declared fallback
  seat.
- **Review role:** review-astra (gpt-6-astra, xhigh), another lab; review-opus is excluded
  (`producer_model_conflict`). Dispatch: `$CLAVAIN_SELECTED_ROOT/scripts/dispatch.sh --role
  plan-review --producer-identity=claude-opus-5-5`, with `CLAVAIN_DECISION_CONTEXT` at
  `<thread-storage>/thr_awr853efiy/home-plan-decision.json`.
- **Status:** rounds 1 to 3 returned needs-rework (22, 19 and 14 findings, cited `[A-n]`,
  `[B-n]`, `[C-n]`; `home-plan-review.md`, `home-plan-review2.md`,
  `thr_awr853efiy/home-plan-review3.md`; revision 3 is commit `f134c3e`). Rounds 4 to 10
  returned needs-rework (17, 13, 8, 7, 6, 4 and 1 findings, cited `[D-n]` to `[J-n]`,
  `thr_uqy4fzn88x/home-plan-review4.md` to `home-plan-review10.md`); each round's partial
  resolutions are finished by the next. mk's ruling B made G-2 to G-6 moot; round 10's one
  (P2) is fixed below.
  - **Round 11 on this revision is required before execution.** It must cover the items in
    "Foundational items for cross-lab review".
  - Still unverified in every round: the GitHub repository identity and the devhost CI status.

## Revision 4 changes

| Item | Theme | Where it landed |
|---|---|---|
| C-1 | `bd create --id` upserts, erasing a pick | Moot: decisions leave bd (decision 24). The store's unique `request_id` insert refuses a second row (Task 1.2) |
| C-2 | Metadata and labels tear | Moot: one SQLite transaction per write, no labels. The optional hub copy is derived and never read (Out of scope) |
| C-3 | Mutex does not fence unknown outcomes | Conditional updates (`WHERE pick_id IS NULL`, `WHERE state = … AND attempt = …`) and two-connection race tests (Tasks 1.2, 1.4) |
| C-4 | Supersede obligations outside the pick; `pending` ambiguous | One decision-level `obligations` table; `pending` means exactly `state = 'pending'`; `dismissed` terminal; tests for a replacement filed before a pick and for dismiss-then-restart (Task 1.4) |
| C-5 | Failed supersession leaves both pickable | No close step: `supersedes` is a stored reference with a unique index, and a pick refuses a superseded decision in the same conditional update (Tasks 1.2, 1.4). Automatic closing is deferred |
| C-6 | Revision and identity omit scope | Filing identity and revision cover project, root, asker and thread; revision recomputed at pick; shared Go/TS vectors; a reused `pick_id` with other parameters is refused (Tasks 1.1, 1.4) |
| C-7 | One bad ruling file blocks all writes | Ready once the store is open; failures stay per obligation with error and retry time (Task 1.4, scenario `ruling-file-blocked`) |
| C-8 | `threads.send` may queue; queued rows vanish on archive | Obligation state `queued` with the queued row id; done on `message.dispatched`; undeliverable on thread archive or delete; serialized drains; a late failure never overrides a success (Task 1.7) |
| C-9 | Truncation drops own answers | Own answers reserved first; structured line fields; UTF-16 budget of 4096; own-answer age 30 days; non-BMP and same-day tests (Task 1.4 feed, Task 1.7) |
| C-10 | Mycroft suggests its own decision beads | Mycroft skips `decision`-type and `home:*` beads; `project:*` label maps to project; files owned in the manifest (Task 1.9) |
| C-11 | Acceptance takes stale or evidence-free records | One `run_id`, commit, tree and clean-tree flag; per-scenario evidence; cleanup bound to the run's ids; rejection fixtures (Task 1.10, criteria 13–14) |
| C-12 | Task 1.4 tests need Task 1.5's Feed | Rev-3 Tasks 1.4 and 1.5 merged into Task 1.4 |
| C-13 | File-retry expects exit 4 then 0 | Recovery read that finds the decision exits 0 at once; a separate `file-unknown` case exits 4, then retry returns the same decision (Tasks 1.9, 1.10) |
| C-14 | `os.OpenRoot` follows a symlinked root | Root identity (dev, ino) saved at filing; the writer refuses a changed root and any symlink component (Task 1.3) |
| Decision 24 | Plugin SQLite storage | Architecture, P-3a, Tasks 1.2–1.4; `serve` read-only (Task 0.2, criterion 3); `bb plugin remove` leaves `data.db` (Task 1.2, G-3) |
| Decision 25 | Structured asks | P-11, Task 1.5; Clavain Stop hook is a follow-up precondition (G-12) |
| V1–V12 | The vizier | P-10, Tasks 1.6, 1.8, 1.12; success measure section |
| OQ1 | `ThreadChat` in a panel | Resolved yes from docs; verified by real-bb scenario `vizier-chat` (Task 1.11) `[E-12]` |
| OQ2 | Overlay window | Needs an Aleph core desktop change; Task 1.12 gated on G-10 |
| OQ3 | Override semantics | Resolved: the asker decides undo or go forward, because it marked the option reversible (P-10) |
| OQ4 | Read marker | Resolved by V11: per account, in the plugin database; only rendered decision items count (Task 1.8) |
| OQ5 | "A then B" order | **mk decision** (G-13) |
| OQ6 | Success measure | Resolved by the baseline (Success measure section) |
| Approvals | First-class approvals | P-12, Task 1.13 (interim, advisory); location is **Aleph core**, ruled by mk 2026-09-29 (G-11) |

## Round 4 dispositions

All seventeen are accepted. None is disputed; D-1 and D-5/D-6 are fixed by narrowing claims
and adding gates rather than by building authentication or trusted approvals here.

| Item | Theme | Where it landed |
|---|---|---|
| D-1 | Caller context is forgeable | P-10 and P-12 say **advisory attribution**; new gate G-15 (host-attested caller, Aleph core); delegation settings changes are panel-only, pinned, and suspend delegation until seen (Task 1.6). Partly fixed by design: enforcement waits on G-15 |
| D-2 | A careless `reversible` delegates consequential work | P-6: the mark is necessary, never sufficient; bounded delegation policy (per-project opt-in, daily cap); merge/deploy/release/publish gates independent of it; approval tokens refused in any filed text; new schema example (Tasks 1.1, 1.6) |
| D-3 | Override races an in-flight wake | **Superseded by Round 7 (ruling B)**: the ordered void notice is cut. Kept: state `sending`, queued rows retracted where the SDK allows, one unordered `void-notice` (Tasks 1.6, 1.7) |
| D-4 | Overridden rulings stay in the feed | Effective vs historical picks; feed and caches use effective rulings; own feed shows the void; mentions copied to the replacement (Tasks 1.4, 1.6) |
| D-5 | Advisory approvals have no safe path to trust | P-12 and Task 1.13: interim records are **non-authorizing** and never promoted; trusted approvals need a reviewed core authentication and audit contract (new gate G-16); prior art updated to mk-qap9 rev 7 and release v8 §5.1 |
| D-6 | Consume does not bind the operation | Moved into the G-16 contract requirements (Task 1.13); the interim has no consume command; `single_use:false` removed; tuple and expiry rendered before the pick |
| D-7 (C-6) | Mentions bypass identity | One `requests` registry for decisions and mentions, read by recovery; mentions need semantic equality or an explicit `mention_of` (Tasks 1.2, 1.4) |
| D-8 (C-5) | Supersession guard is incomplete | Predecessor check and insert in one conditional transaction; explicit `superseded`/`withdrawn`/`stale` outcomes; both interleavings tested (Tasks 1.2, 1.4) |
| D-9 (C-8) | Queued wakes stranded or resurrected | Early events buffered by row id; queue state reconciled after binding and every 60 s; unresolvable rows become `uncertain` for mk, never resent on marker absence alone (Task 1.7) |
| D-10 | Migration vs Aleph's activation order | Expand-only staged migrations with `min_reader_version`; failed-activation test; degraded, retrying initialization (Task 1.2, scenario 7) |
| D-11 | Machine blockers cannot progress or resolve | `progress`, `resolve`, `withdraw` with `updated_at`, persistent recipients and wakes (Task 1.5) |
| D-12 | "Mark all seen" can erase unseen rulings | Acknowledges a rendered, visible snapshot of item ids only; hidden tabs pause timers (Tasks 1.7, 1.8) |
| D-13 | Export not crash-idempotent | Immutable per-store segments keyed by `(store_id, seq)`; boundary tests (Task 1.2) |
| D-14 | Test order cannot run | Scaffold moves to Task 1.1; real-bb run needs a permitted isolated install or G-3 (Task 1.11) |
| D-15 (C-11) | Stale evidence; artifact identity | External run id, typed evidence, evidence outside the checkout bound to a product commit, stale-run and malformed fixtures; criterion 3's pipeline fixed (Tasks 0.2, 1.10, 1.11, criteria 3, 13, 14) |
| D-16 | `--thread` lost at the proxy | Recipient vs caller defined; `ExecFiler` sets `BB_THREAD_ID`; conflicts refused (Task 1.9, `ask-cli-proxy`) |
| D-17 | New threads miss the project feed | Shared per-project cache restored and combined with own lines (Task 1.7) |

## Round 5 dispositions

All thirteen accepted; none disputed. E-4 removes batching; E-13 (with D-1) narrows the
claim, and authenticated exclusivity waits on G-15.

| Item | Theme | Where it landed |
|---|---|---|
| E-1 | Semantic equality merges unrelated asks | Key binds a structured `subject` and all answer semantics; no `subject`, no automatic mention (P-11, schema, Task 1.1) |
| E-2 | Override does not order delivery | **Superseded by Round 7 (ruling B)**: the `after_id` chain is cut. Kept: voided wakes are never retried (P-5, Tasks 1.2, 1.7) |
| E-3 | Copying mentions violates `UNIQUE` | Mentions keyed `(decision_id, thread)`; `requests` keeps uniqueness and the original result (Task 1.2) |
| E-4 | Retraction deletes other queued answers | One message per obligation, no batching; mixed-row test (P-5, Task 1.7, criterion 10) |
| E-5 | Crash before binding resurrects cancelled work | `op` marker correlation independent of the response; otherwise `uncertain` with Resend/Dismiss (P-5, Task 1.7) |
| E-6 | Recovery skips identity | `bb home get` returns identity and scope; mismatch exits 2 (Task 1.9, criterion 10) |
| E-7 | Mention matching bypasses supersession | Replacement filing is an exclusive branch; `supersedes` with `mention_of` refused (P-11, Tasks 1.1, 1.4) |
| E-8 | Obligations lack recipient and payload | Outbox rows snapshot recipient, payload, `op`, `after_id`; resolve/withdraw notices are rows (P-5, Tasks 1.2, 1.5) |
| E-9 | G-11b reopens an authorizing path | G-11b withdrawn; no `APPROVED-*` tokens in the interim, tested (Task 1.13, G-11) |
| E-10 | Retry during serve outage says "nothing filed" | Known request ids resolved locally before serve (Task 1.4) |
| E-11 | Criterion 14 accepts historical evidence | Criterion 14 runs the real harness with its own run id at HEAD; reports are history (Task 1.11, Verification) |
| E-12 | Real `ThreadChat` check before install | Moved to real-bb scenario `vizier-chat`; unit tests use the stub (Tasks 1.8, 1.11, criterion 14) |
| E-13 | Criterion 15 overstates authentication | Advisory claimed-id checks with forged-id and direct pick RPC tests; exclusivity is G-15 (Task 1.6, criterion 15) |

## Round 6 dispositions

All eight were accepted; none disputed. F-8 narrows the claim (G-15). Rows marked
superseded describe machinery cut in round 7.

| Item | Theme | Where it landed |
|---|---|---|
| F-1 | Ambiguous send failures retried | Only a proven pre-acceptance rejection returns to `pending`; any other failure is reconciled, then `uncertain` until Resend; tested without a crash (P-5, Task 1.7 step 0 (e), outcomes, criterion 10) |
| F-2 | Several outstanding attempts per obligation | `attempts` table, one row per send with its own handle and state; Resend adds an attempt, earlier ones keep reconciling (Tasks 1.2, 1.7, criterion 10). **Superseded by Round 7 (ruling B)**: the "never delivered" rule and its tests are cut |
| F-3, F-5 | Cancelling V releases the answer; feed bypasses the barrier | **Superseded by Round 7 (ruling B)**: the `answer` envelope, `releaseBarrier` and the feed barrier are cut; the replacement answer and feed line only name what they supersede (P-10) |
| F-4 | Marker presence is not delivery | Match recipient, accepted `client/turn/requested` event, full framed marker `[home-msg <op> #<n>]` and snapshotted payload; persist the matching id; none, several or conflicting matches stay `uncertain`; quoted, edited and duplicate tests (P-5, Task 1.7, criterion 10) |
| F-6 | `steps-done` fan-out breaks `UNIQUE(op)` | `op` names its recipient (`steps-done:<decision>:<thread>`); the recipient set is inserted in one transaction; two-mentioner, retry and restart tests (P-5, Tasks 1.2, 1.5, criteria 6, 15) |
| F-7 | Acceptance can exercise an older install | `scripts/build-identity.mjs`, `health.build_sha256` and `autarch version --json`; acceptance builds, installs, reloads, then verifies loaded plugin and Go binary identity; stale-loaded-plugin rejection and preflight-abort tests (Tasks 1.9, 1.10, 1.11, criterion 14) |
| F-8 | Criterion 15 contradicts direct pick | Split: forged `rule` obeys delegation limits and is pinned; direct `pick` RPC shows the admitted attribution bypass (`by: mk`, no limits, no wake for ruling-only); human picks are not limited; separation is G-15 (Task 1.6, criterion 15) |

## Round 7 dispositions

mk's ruling B (P-10): override is best effort, with one void notice and a supersedes line.
Cut: the barrier, the `answer` envelope, `void_after` and the override `after_id` chain,
`neverDelivered`, `releaseBarrier`, and the `voided` state. None disputed.

| Item | Theme | Disposition |
|---|---|---|
| G-1 | Resend cannot start its attempt | Fixed: one-shot `resend_permit`, set by Resend, cleared by the claim that inserts the attempt; tested through the real drain with restarts (P-5, Tasks 1.2, 1.7, criteria 6, 10) |
| G-2 | Override vs terminal states | Moot by mk's ruling (B), fix kept: `voided_at` is the orthogonal suppression marker and the `voided` state is gone, the simplest way to keep `done` terminal; tested after `done` and in flight (Tasks 1.2, 1.6, 1.7) |
| G-4 | A → B → C drops the barrier | Moot by mk's ruling (B): no barrier to inherit; non-delegability is inherited along `supersedes` so the vizier cannot rule C (P-10, Tasks 1.2, 1.6); finished by H-3 |
| G-3, G-5, G-6 | Edited envelope; `neverDelivered`; per-thread barrier vs shared cache | Moot by mk's ruling (B): no envelope, `neverDelivered` or per-thread feed release remains; a bound row mk edits is his message, the notice always goes, and the shared project cache stands (P-5) |
| G-7 | Acceptance identifies only part of the product | Fixed: path install of a clean HEAD export (Aleph loads source), harness-owned `serve`, identities of plugin source, rendered app, daemon and `autarch`; stale plugin, frontend and daemon rejected (Tasks 0.2, 1.7, 1.9, 1.10, 1.11, criterion 14); finished by H-6 |

**Removed** (only tested cut machinery): tests Envelope, Multiple attempts, Release anyway
(1.6), Feed barrier (1.4), Void ordering (1.7), `neverDelivered` store assertions (1.2);
criterion clauses 6 (wait on every attempt), 9 (feed barrier), 10 and 15 (envelope, waiver).

## Round 8 dispositions

All six accepted; none disputed. G-2's fix is retained; G-1, G-4 and G-7 are finished here.
Only criterion 15's unconditional notice order is removed, replaced by confirmation (H-5).

| Item | Theme | Where it landed |
|---|---|---|
| H-1 | Reconciliation strands a Resend | Permit precedence: a row holding `resend_permit` stays `pending` until delivery, Dismiss or `voided_at`; reconcile-before-drain and blocked-drain tests (Tasks 1.2, 1.7) |
| H-2 | A selected, unclaimed wake sends after override | `claim` re-checks the whole `dueMessages` predicate with the attempt insert; send only after it succeeds; Stale selection test for a wake and an owner notice (Tasks 1.2, 1.7) |
| H-3 | A → B → C loses recipients and lineage | Every replacement copies mentions; `overriddenRuling` walks `supersedes`; wake and feed name A; chain test via M's feed and fan-out (Tasks 1.2, 1.4, 1.6, criterion 15) |
| H-4 | A late void notice says wait again | The notice names the replacement and waits only if its answer has not arrived; Notice after the answer test, duplicates included (P-10, Task 1.6, criterion 15) |
| H-5 | Uncertain owner notice treated as ordered | No ordering: the owner notice asks for confirmation via `bb home get --id`, which returns lifecycle state; Late owner delivery test (Tasks 1.5, 1.7, criterion 15) |
| H-6 | Plugin never uses the scratch token | `serveTokenFile` and `serveProjectDirs`, read server-side; preflight checks authenticated project roots (dev, ino); different default token and older daemon test (Tasks 1.7, 1.10, criterion 14) |

## Round 9 dispositions

All four accepted; none disputed. H-1, H-2, H-4 and H-5 are resolved; H-3 and H-6 are
finished by I-3 and I-4. I-2 takes the smaller fix, which removes machine-ask replacement.

| Item | Theme | Where it landed |
|---|---|---|
| I-1 | A replayed Resend authorizes another attempt | Resend carries the displayed `attempt` and a per-click `click_id`, checked in its `UPDATE`; a replay returns the first result; replay tests incl. restart (Tasks 1.2, 1.7, criterion 10) |
| I-2 | Supersession leaves a machine owner notice live | Ordinary supersession is `decide`-only in v1; a machine or steps ask ends only by resolve or withdraw; refusal test (Tasks 1.2, 1.4, 1.5) |
| I-3 | A's waiting line never clears in A → B → C | The void line names and waits on the chain's current tip; picking C clears it in both feeds (Tasks 1.4, 1.6) |
| I-4 | Discovery lists children of a scan root | Scratch projects live under one scratch discovery parent passed as `--project-dir`; Go test against the real resolver; preflight test uses real `serve` (Tasks 0.4, 1.10, criterion 14) |

## Round 10 dispositions

Accepted; not disputed. I-1 to I-4 are resolved.

| Item | Theme | Where it landed |
|---|---|---|
| J-1 | Chain test asks for a fan-out a `decide` C cannot have | Assertion dropped from the Task 1.6 chain test; Task 1.5 already covers resolve fan-out to asker and mentioners and `steps-done` with two mentioners; the criteria had no such clause |

## Foundational items for cross-lab review

Round 11 must review these explicitly. Each lets something act in mk's name, so a flaw is
costly and hard to see from the author's seat.
1. **`by: vizier`** (P-10, Task 1.6): who may record a pick other than mk, how the plugin
   knows the caller is the vizier thread, and whether calling it advisory until G-15 is
   honest enough.
2. **`reversible`** (P-6, P-10): the asker's own mark is necessary for delegation, and the
   bounded policy plus independent merge/deploy/release gates must hold when it is careless.
3. **The superseding override** (P-10, Tasks 1.6–1.7): the old pick stays but stops being
   effective, a new decision is owed to mk, the old wake is suppressed, and one void notice
   goes to the asker. Best effort (ruling B): no order is promised, an asker may act on the
   old ruling before the notice arrives, and the replacement names what it supersedes.
4. **Approvals** (P-12, Task 1.13): interim records authorize nothing; the core contract
   (G-16) is what other enforcers would trust.

## Plan-level decisions (settled here; reviewers, push on these)

- **P-1: Gurgeh goes multi-project by path, not by process.** `/gurgeh/{project}/…` resolves
  `{project}` through `projects.Resolve` (Task 0.2), builds a `gurgeh/server.Server` for that
  root lazily and caches it. An uninitialized project returns 404; the service never runs
  `EnsureInitialized`.
- **P-2: MCP attaches as a subcommand, not as a shim.** Step 0 moves MCP into the binary as
  `autarch mcp` and keeps `cmd/autarch-mcp` as an alias. Step 1 adds one MCP tool,
  `autarch_file_decision`, which files through `internal/homeask` like the CLI.
- **P-3: `serve` has a token and an Origin check even on loopback** `[A-22]`.
  - Every route except `GET /health` needs `Authorization: Bearer <token>` (token in
    `~/.autarch/serve.token`, 0600, header only, no `?token=`), a `Host` of
    `127.0.0.1:<port>` or `localhost:<port>`, and no `Origin` or an allowlisted one (empty by
    default; WebSocket upgrades included). This also covers Bigend's permissive `CheckOrigin`
    (`internal/bigend/daemon/server.go:380`).
  - The token is the same-user boundary accepted in `[D21]`.
- **P-3a (rev 4): one writer, the plugin's database** (decision 24; replaces the owner lock).
  - Every decision write runs in bb's server process, in a SQLite transaction on the plugin's
    `data.db`. There is no second writer process, and no `bd` write.
  - Filing is an insert guarded by a unique `request_id`. A pick is
    `UPDATE … WHERE pick_id IS NULL AND superseded_by IS NULL AND revision = ?`, and succeeds
    only when one row changed.
  - Plugin code shares bb's event loop, so a transaction holds no network or file I/O.
  - Removing the plugin leaves `data.db` behind (`plugin-service.ts:1513-1561`). A reinstall
    reads it; migrations must accept an older schema.
  - Aleph runs a candidate plugin's factory before disposing the old instance, and keeps the
    old one if the candidate fails (`plugin-runtime.ts:1608`, `:1664`). So migrations are
    **expand-only and staged** `[D-10]`: the old instance must keep working on the migrated
    `data.db` (Task 1.2).
- **P-4: a pick is a record plus obligations, and it is idempotent** `[B-1]` `[B-2]` `[C-3]`.
  - Owed means a decision with no pick, not superseded and not withdrawn.
  - The surface sends a `pick_id` generated once per click. A retry with the same `pick_id`
    and the same parameters returns the recorded pick. The same `pick_id` with other
    parameters is refused 409 `pick id reused` `[C-6]`. A different `pick_id` on a picked
    decision gets 409 `already ruled`.
  - Obligations are rows: `ruling-file`, and one **outbox message** per notification
    (`wake`, `void-notice`, P-11 notices). `pending` means exactly `state = 'pending'` `[C-4]`.
  - Nothing else is state. Home runs nothing.
- **P-5: wakes are scoped, one durable outbox row per message** `[A-4]` `[A-15]` `[B-4]`
  `[C-8]` `[E-2]` `[E-4]` `[E-5]` `[E-8]` `[F-1]` `[F-2]` `[F-4]` `[F-6]`.
  - Wakes go to instruction and needs-context picks (ruled G-5), and to every delegated
    ruling (P-10). Ruling-only outcomes by mk reach the asker through the feed.
  - **The outbox.** Every message is one row, written in the same transaction as the
    lifecycle change that causes it, snapshotting its recipient thread, exact payload,
    operation identity (`op`, unique and naming its one recipient, e.g.
    `wake:<decision>:<pick_id>`, `void:<decision>`, `steps-done:<decision>:<thread>`) and
    ordering key (`after_id`, the row it must follow). A notice to several threads is one
    row per thread, all inserted in one transaction `[F-6]`. The drain never derives a
    message from other tables. **One message per row, no batching**, so removing one
    queued row affects exactly one row.
  - A row is sent only after its `after_id` row is finished (Task 1.2). **Suppression is
    orthogonal** `[G-2]`: `voided_at` marks a row that is never sent or resent again, but it
    never changes the row's delivery state, so `done` stays `done` and an attempt already
    in flight still settles and is recorded.
  - **Attempts are tracked one by one** `[F-2]`. Each send is an `attempts` row with its own
    transport handle (queued row id or message id), state (`sending`, `queued`, `delivered`,
    `not-delivered`, `uncertain`) and evidence. An attempt is **settled** when it is
    `delivered` or `not-delivered`. Resend adds exactly one new attempt through a one-shot
    permit (Task 1.7) `[G-1]`; earlier uncertain attempts keep being reconciled, and any
    `delivered` attempt completes the row.
  - Each attempt's message starts with the framed marker `[home-msg <op> #<attempt>]`,
    known before sending, so correlation never depends on the send response.
  - **Delivery evidence** `[F-4]`: an attempt is `delivered` only on a `delivery: "sent"`
    response, a `message.dispatched` event for its bound queued row, or an accepted
    `client/turn/requested` event in the recipient thread whose input equals the snapshotted
    payload with its framed marker exactly. The matching event or row id is stored on the
    attempt. A quotation of the marker, an edited row found by search, or more than one or
    conflicting matches leave the attempt `uncertain`. A bound queued row that mk edits in
    Aleph and then dispatches counts as delivered: the edit is his `[G-3]`. Queue state is
    reconciled by row id, never inferred from a missing marker `[D-9]`.
  - **Only a proven pre-acceptance rejection is retried automatically** `[F-1]`: a failure
    that step 0 of Task 1.7 shows cannot have reached the server (connection refused before
    the request was written, or a documented pre-acceptance error) settles the attempt
    `not-delivered` and the row returns to `pending` with backoff. Any other failure is an
    unknown outcome: the attempt is reconciled, and unless delivery or non-delivery is
    established it stays `uncertain`.
  - **`uncertain`** `[E-5]`: a row with an unsettled attempt and none delivered (a crash in
    `sending`, including before the handle was stored; an ambiguous error; an unreadable
    queued row) is `uncertain`, listed on Home with **Resend** and **Dismiss**. Home never
    resends it on its own. Delivery is therefore at least once, or shown to mk as
    uncertain.
  - An undeliverable wake is listed on Home, however old, until mk dismisses it. Successor
    resolution is deferred (G-9).
- **P-6: kinds, instructions and reversibility** `[D22]`.
  - Kinds are `instruction`, `needs-context` and `ruling-only`, inferred as in rev 3. An
    instruction is plain text of at most 2,000 characters, shown and delivered exactly, never
    parsed or run by Home.
  - An option may carry `reversible: true`, set by the asker at filing. The mark is
    **necessary, never sufficient** for delegation `[D-2]`: the bounded delegation policy
    (P-10) must also allow it. An option carrying an `approval` spec (P-12) cannot be
    reversible.
  - **Merge, deploy, release and publish authorization is independent of the mark.** Those
    gates (the SylvesteOps merge hook, release plan v8's signed approvals) keep requiring mk
    through their own checks. No Home text, delegated or not, can satisfy them: filing refuses
    any question, label, instruction or reason containing an approval token
    (`APPROVED-[A-Z]+`), and every wake says it is not merge, deploy, release or publish
    authorization.
  - Only an asker with a thread can offer `instruction` or `needs-context`, or mark an option
    reversible. Mycroft files ruling-only, non-reversible options.
- **P-7: the feed goes through `bb.agents.configure`** (ruled G-5) `[A-11]`. The plugin builds
  it from its own store and injects it at session start and resume. `bb home feed --json`
  exposes the same data to other consumers. The Clavain per-turn hook `[D17]` stays deferred.
- **P-8: the feed comes from Home's pick records, not from files.** A feed line comes only from
  a pick row. A hand-written ruling file reaches no agent. Editing `data.db` by hand is the
  same-user risk of `[D21]`.
- **P-9: Home does not commit ruling files.** They go into the project's working tree;
  committing is the project's normal flow.
- **P-10: delegated rulings and overrides** `[V4]` `[V7]` `[V8]`.
  - The vizier rules with `bb home rule`. The plugin accepts it only when the CLI's
    `ctx.threadId` equals the `vizierThreadId` setting, and only on a reversible option of a
    decision filed by a thread. The pick records `by: "vizier"` and a reason.
  - **Attribution is advisory** `[D-1]`. The CLI takes the thread from `BB_THREAD_ID`
    (Aleph `apps/cli/src/context-env.ts:48`) and the server copies the client-submitted id
    (`apps/server/src/routes/plugins.ts:438`); panel RPC carries no attested caller. Any local
    thread can pose as the vizier or call the panel's `pick` and be recorded `by: mk`. So
    `by` is a label, not an authority check, until Aleph derives the caller from the host
    (G-15). Nothing may treat it as authorization.
  - **Bounded delegation policy** `[D-2]`, because attribution is advisory: the vizier may
    rule only when (a) the asker marked the option reversible; (b) the option has no
    `approval` spec and the decision is not an override replacement or a replacement of
    one (non-delegable is inherited along `supersedes`) `[G-4]`; (c) mk enabled
    delegation for that project (`delegation.projects`, empty by default); and (d) fewer than
    `delegation.dailyCap` (default 10) delegated rulings were made in the last 24 h.
    Delegated rulings never satisfy merge, deploy, release or publish gates (P-6).
  - **Delegation settings are protected** `[D-1]`: `vizierThreadId` and `delegation.*` change
    only through the panel's settings RPC, never the CLI. Every change is an event pinned in
    catch-up, and delegation is suspended until mk has seen it. This is also advisory until
    G-15, but no change is silent.
  - A delegated ruling never mints an approval (P-12), and irreversible options and Mycroft's
    decisions stay owed to mk.
  - Every delegated ruling wakes the asker and is listed in catch-up as "vizier ruled X on Y",
    pinned until mk has seen it, with an Override button. There is no veto window: reversible
    actions go ahead at once.
  - **Override files a superseding decision.** The old pick stays immutable as history but is
    no longer **effective** `[D-4]`: an effective ruling is a pick whose decision has no
    replacement. The new decision copies the question, options and mentions, sets
    `supersedes`, is not delegable, and is owed to mk.
    - **Override is best effort in v1** (mk's ruling B, round 7). In its transaction the
      override sets `voided_at` on the old wake W, so W is **never sent or resent again**
      (an attempt already in flight still settles) `[E-2]` `[G-2]`; asks the wake loop to
      remove a queued attempt of W where the SDK allows; marks the old ruling `void` in the
      rail and the feed; and inserts **one** `void-notice` to the asker, an ordinary outbox
      row (`op` `void:<decision>`, no `after_id`): "mk is overriding the vizier's ruling on
      <A>; the replacement is <B>. Treat that ruling as void. If you already acted on it,
      decide whether to undo or continue and say which in your turn ending. If mk's answer
      on <B> (or a later replacement) already reached you, it stands and this notice does
      not change it; otherwise wait for it" `[H-4]`. Mentioning threads see the ruling as
      void in their feed.
    - **The replacement answer names what it supersedes.** mk's pick on the replacement is an
      ordinary pick; its wake and its feed line both say "this supersedes the vizier's
      ruling on <A>", A found through the chain (Task 1.2) `[H-3]`, so whichever message
      arrives last, the asker knows which answer stands.
    - **No delivery order is promised.** The old wake, the notice and the replacement can
      arrive in any order, and **an asker may act on the old ruling before the notice
      arrives**. mk sees the voided ruling in the rail; the feed drops it from effective
      answers, with no publication barrier. **OQ3:** the asker decides undo or go forward,
      because it marked the option reversible; Home does not model undo.
  - Only `by: vizier` picks can be overridden in v1. An agent can still supersede its own open
    decision.
- **P-11: structured asks** `[D25]`.
  - An ask has a kind: `decide` (a decision, as above), `steps` (steps only mk can do), or
    `machine` (a blocker no agent in the thread can clear). "Nothing" files nothing.
  - A filing attaches a **mention** to an open ask from another thread instead of a second
    entry only when it names that ask (`mention_of: <id>`) or is **semantically equal** to
    it `[D-7]` `[E-1]`: both carry a structured `subject` (what the ask is about and the
    action asked, e.g. `autarch/catch-up: collapse order`), and they share project, kind and
    `semantic_key`, which is sha256 over the canonical kind, subject, normalized question,
    and every answer semantic: option ids, labels, kinds, instructions, reversible marks,
    approval specs and recommendation, steps, and machine class and detail. Without a
    `subject` there is no automatic mention; a paraphrase needs `mention_of`. A matching
    `ask_key` alone only shows "possibly related: <id>". Home shows "also mentioned in N
    threads", and mentioning threads receive the ruling through the feed.
  - A replacement (`supersedes`) is an exclusive branch `[E-7]`: it cannot carry
    `mention_of`, and it is never matched as a mention.
  - Every filing, decision or mention, is one row in a single `requests` registry, so a retry
    returns the same result whichever it was `[D-7]`.
  - Machine blockers go to the machine lane. They reach mk's Asks only when no owner is known.
    The owner reports `progress`, and the owner or asker can `resolve` or `withdraw` `[D-11]`.
  - Steps from all threads form one runbook. When mk marks a thread's steps done, that thread
    is woken.
  - The Clavain Stop hook that writes these at turn end is Clavain's (G-12). The decision-20
    net stays as a fallback. Standing rulings come after the trial and are owned by Clavain;
    the schema reserves `by: "rule:<id>"` so Home can show which rule answered.
- **P-12: approvals are first-class records** (spec in Task 1.13).
  - **Recommendation: Aleph core owns the approval record and its check API**, reusing
    mk-qap9's content binding and audit and release plan v8 §5.1's mk approval record
    (mk-7l2o), not a third scheme. Home is only the surface where mk picks.
  - Prior art as of 2026-09-29 `[D-5]`: mk-qap9 is at **revision 7**, a lighter version that
    explicitly drops same-user enforcement and ships convenience, content binding and audit;
    its revision 6 broker is reference only
    (`aleph-worktrees/biometric-root-approval/plans/2026-09-27-biometric-root-approval.md`).
    Release v8 §5.1 requires a presence-key signature by mk, enumerated transitions bound to
    prior and new semantic hashes, and two validity dates (`expires_at`,
    `batch_valid_until`). Deferring Home ruling-file signing waives none of that.
  - Reasons: an enforcer cannot depend on a removable plugin whose `data.db` survives removal;
    "per account" needs core's account identity; an atomic consume in the plugin covers only
    callers that go through the plugin; and a `by: mk` row reachable through ordinary RPC is
    forgeable (D-1), so moving it into core would move the forgery.
  - Only mk's own pick mints; `by: vizier` never does.
  - **Interim Home records are non-authorizing** `[D-5]`. They show mk's intent and are
    never promoted, imported or converted into authoritative approvals. Trusted approvals
    need the reviewed core authentication and audit contract first (G-16). The SylvesteOps
    text hook stays the enforcing check. **mk ruled 2026-09-29 (G-11): approvals live in Aleph core.**

## Must-Haves

**Truths:**
- `autarch serve` answers `GET /health` with every mounted subsystem's status. It refuses
  `--addr 0.0.0.0:…`, returns 401 without the token and 403 for an Origin not allowed. It has
  no decision route and no decision writer.
- Existing clients keep working under prefixes: `/bigend/api/sessions`,
  `/gurgeh/{project}/api/specs`, and `/signals/ws` carrying Gurgeh's signals.
- An agent inside a bb thread runs `bb home ask` (or `autarch decide file`) and exactly one
  decision row exists, even across retries and racing retries. It carries the question, the
  options (id, kind, instruction, reversible), the project and its root identity, the asker
  and the thread. If Home is not ready the agent is told nothing was filed (exit 3); if the
  outcome is unknown and the recovery read cannot tell, it is told a retry is safe (exit 4).
- Home lists owed asks across projects, ranked. It shows each option's label, kind,
  reversibility and exact instruction, the recommendation, mentions, undeliverable wakes and
  per-obligation failures.
- A pick is recorded once, across a retry, a kill of bb's server or two racing surfaces. A
  ruling file is written, and instruction and needs-context picks wake the asker once with
  the label and exact instruction.
- A caller claiming the vizier thread can rule only on reversible options of thread-filed
  decisions that the bounded delegation policy allows; the claim is advisory, and only G-15
  makes it exclusive. Every such ruling is shown to mk, pinned until seen, and can be
  overridden by a superseding decision; an override voids the old ruling in the feed, never
  sends the voided wake again, and sends one void notice; the replacement's wake and feed
  line say which ruling they supersede. Delivery is best effort: an asker may act on the old
  ruling before the notice arrives, and mk sees the voided ruling in the rail. A direct pick RPC is recorded `by: mk` without delegation limits; `by` is
  advisory attribution (G-15), and no Home text satisfies a merge, deploy, release or
  publish gate.
- Each answer or notice is one outbox message, and each send of it one attempt; it counts
  as delivered only on exact matching evidence, arrives at least once or is shown to mk as
  uncertain, and is never resent without mk (one Resend, one attempt) unless proven
  rejected before acceptance.
- New sessions, including a thread's first, receive their project's recent effective rulings
  and the thread's own answers, as labels, with own answers never crowded out.
- Mycroft's suggestions and out-of-allowlist dispatches appear as ruling-only decisions. It
  never schedules a decision bead. `DecisionQueue` holds no private list.
- Catch-up lists facts first, ranked, with vizier notes marked and citing fact ids.

**Artifacts:**
- `internal/serve/`: `serve.go` (`New`, `Handler`, `Run`), `auth.go`, `projects.go`
  (`Resolver`, `Resolve`, `Projects`).
- `cmd/autarch/`: `serve.go`, `mcp.go`, `decide.go` (`file`, `list`, `stats`), `exit.go`
  (`ExitError{Code}`).
- `schema/home-ask/v1/`: `ask.schema.json`, `vectors.json` (canonicalization and identity
  vectors, shared by Go and TS).
- `internal/homeask/`: `model.go` (`Ask`, `Option`, `Kind`, `Validate`, `Identity`,
  `Revision`), `filer.go` (`Filer`, `ExecFiler`, `ErrHomeDown`, `ErrOutcomeUnknown`).
- `internal/mycroft/escalate/escalate.go`: `DecisionQueue` backed by `homeask.Filer`.
- `integrations/bb-plugin-autarch/`: `package.json`, `server.ts`, `contract.ts`, `model.ts`,
  `store.ts`, `migrations.ts`, `ruling.ts`, `service.ts`, `asks.ts`, `delegation.ts`,
  `wakes.ts`, `feed.ts`, `catchup.ts`, `cli.ts`, `export.ts`, `app.tsx`, `ui/`, `e2e/`,
  `scripts/check-e2e.mjs`, `__tests__/`, `README.md`.

**Key links:**
- `serve.Run`: `EnsureLocalOnly` → `Listen` → `Serve`; shutdown on context end.
- `bb home ask` → `service.file` → one transaction: insert the `requests` row (unique
  `request_id`) with a decision or a mention → return.
- `service.pick` → one transaction: conditional update of the pick, insert obligations, insert
  the event → after commit, `reconcile(decision)` writes the ruling file and the wake loop is
  nudged.
- `reconcile` handles `ruling-file`. `wakes.drain` sends every outbox row (`wake`,
  `void-notice` and the P-11 notices) from `dueMessages()`, one row per message and one drain
  at a time `[E-4]`.
- `autarch decide file`, the MCP tool and Mycroft → `homeask.ExecFiler` → `bb home ask`.
- The panel calls plugin RPC only; the plugin holds the serve token.

---

## Step 0: one read-only `autarch serve`

Step 0 finishes, and its tests pass, before any step 1 task starts `[A-17]`. No step 0 file
imports `internal/homeask` or any decisions code `[B-16]`.

### Task 0.1: expose the existing servers as handlers

**Files:**
- Modify: `internal/bigend/daemon/server.go:38-86`. Add `Handler() http.Handler`, returning
  `s.mux`.
- Modify: `internal/gurgeh/server/server.go:25-48`. Add `Handler()`, which calls `routes()`
  once through a `sync.Once`. `ListenAndServe` uses the same once.
- Modify: `pkg/signals/server.go:21-54`. Same pattern.
- Test: `internal/bigend/daemon/handler_test.go`, `internal/gurgeh/server/handler_test.go`,
  `pkg/signals/handler_test.go`.

**Steps:**
1. Write failing tests. Each wraps `Handler()` in `httptest.NewServer` and asserts
   `GET /health` returns 200. The Gurgeh test uses a temp root with an empty specs directory.
   The Signals test asserts calling `Handler()` twice does not panic.
2. Run `go test -race ./internal/bigend/daemon/ ./internal/gurgeh/server/ ./pkg/signals/`.
   Expect FAIL (`Handler undefined`).
3. Implement, rerun, expect PASS.
4. Commit: `serve: expose bigend, gurgeh and signals as handlers`.

<verify>
- run: `go test -race ./internal/bigend/daemon/ ./internal/gurgeh/server/ ./pkg/signals/`
  expect: exit 0
</verify>

### Task 0.2: `internal/serve`, one mux, project resolution, read-only

**Files:**
- Create: `internal/serve/serve.go`, `internal/serve/auth.go`, `internal/serve/gurgeh.go`,
  `internal/serve/projects.go`.
- Test: `internal/serve/serve_test.go`, `internal/serve/serve_bind_test.go`,
  `internal/serve/projects_test.go`, `internal/serve/readonly_test.go`.

**Design:**
- `Config{Addr, ProjectDirs []string, TokenPath string, AllowOrigins []string}`. The default
  `Addr` is `127.0.0.1:8110`, so it does not collide with a running Bigend daemon on 8100.
- `projects.go` `[A-19]`:
  - `Resolver` is built from the Bigend project list. Each root goes through
    `filepath.EvalSymlinks` and must stay inside a configured `ProjectDirs` entry (also
    resolved).
  - `Resolve(name)` maps a base name to one resolved root. A shared base name is an error
    (`ambiguous project "x": /a/x, /b/x`).
  - The list is re-read at most every 30 s.
  - `Projects()` returns `[{name, root, dev, ino}]`, where `dev` and `ino` come from `stat` on
    the resolved root. The plugin saves them at filing and the ruling writer checks them
    `[C-14]`.
- Mounts: `/bigend/` (`StripPrefix` over `daemon.Handler()`), `/signals/` (Task 0.3), and
  `/gurgeh/{project}/` (P-1). An unknown project, or one without `.gurgeh`, returns 404.
- `GET /api/projects` returns `Projects()`. It is the only route `serve` adds.
- **Read-only for decisions** (decision 24): there is no `/api/decisions` route and no
  decision code. Gurgeh's existing project-file endpoints are unchanged.
- `GET /health` needs no token. It returns `{"status":"ok","bigend":…,"gurgeh":{…},
  "signals":…,"projects":N,"build":{"revision":…,"exe_sha256":…}}`; `build` comes from
  `runtime/debug.ReadBuildInfo` and a hash of the running executable, the same helper
  `autarch version` uses (Task 1.9) `[G-7]`.
- `auth.go` implements P-3: `LoadOrCreateToken(path)` (32 random bytes, hex, mode 0600; a
  looser mode is refused). The middleware checks `Origin` (403), then `Host` (403), then the
  bearer header with `subtle.ConstantTimeCompare` (401). No query-string token.
- `Run(ctx)`: `netguard.EnsureLocalOnly(Addr)`, `Listen`, `Serve`, and `Shutdown` with a 5 s
  timeout when `ctx` is done. Rev 3's `BeforeListen`/`AfterListen` hooks are removed with the
  owner lock and reconcile loop they served.

**Steps:**
1. Write failing tests:
   - `/health` returns 200 without a token.
   - `/bigend/api/sessions` returns 401 without the token, and 200 with it.
   - `?token=<valid>` with no header returns 401 `[A-22]`.
   - A bad `Host` (`evil.example:8110`) returns 403.
   - `Origin: http://evil.example` with a valid token returns 403, on a plain route and on a
     WebSocket upgrade to `/signals/ws`.
   - A WebSocket upgrade to `/signals/ws` with the bearer header and no Origin succeeds.
   - `/gurgeh/demo/api/specs` returns 200 for an initialized temp project and 404 for an
     unknown one.
   - `Resolve` refuses duplicate base names, refuses a root whose symlink points outside
     `ProjectDirs`, and follows a symlink retargeted between two calls once re-read.
   - `GET /api/projects` returns each root with the `dev` and `ino` of its resolved path; a
     root replaced by a new directory at the same path reports a new `ino`.
   - `ReadOnly`: `POST`, `PUT`, `PATCH` and `DELETE` to `/api/decisions` and `/api/projects`
     return 404 or 405, and no handler outside the Bigend, Gurgeh and Signals mounts accepts
     a non-GET method.
   - The token file is created with mode 0600, and a 0644 token file is refused.
   - Bind test: `Run` with `0.0.0.0:0` or `[::]:0` returns the netguard error.
2. Run and expect FAIL. Implement. Run and expect PASS.
3. Commit: `serve: one read-only loopback mux with token and origin checks`.

<verify>
- run: `go test -race ./internal/serve/`
  expect: exit 0
- run: `go test -race -run 'Bind|ReadOnly' ./internal/serve/`
  expect: exit 0
- run: `deps=$(go list -deps ./internal/serve/) && ! printf '%s\n' "$deps" | grep -qE 'internal/(decisions|homeask)'`
  expect: exit 0 (a failing `go list` fails the check `[D-15]`)
</verify>

### Task 0.3: one Signals broker, and Gurgeh publishes into it `[A-14]`

**Files:**
- Modify: `internal/gurgeh/server/server.go`. Add `WithPublisher(func(signals.Signal))`.
  `refreshSignals` (`:132`) uses it when set, and keeps today's
  `psignals.NewClient(psignals.DefaultServerURL())` path otherwise.
- Modify: `internal/serve/serve.go`. `New` calls `signals.NewBroker()` once, passes it to
  `signals.NewServer(broker)`, and gives each Gurgeh server `WithPublisher(broker.Publish)`.
- Test: `internal/serve/signals_test.go`, `internal/gurgeh/server/publisher_test.go`.

**Steps:**
1. Write failing tests: with a publisher set, a Gurgeh spec update calls it and dials nothing
   (the default URL points at a closed port); through `serve`, a subscriber on `/signals/ws`
   receives a Gurgeh spec update within 2 s.
2. Implement, rerun, commit: `serve: one signals broker shared with gurgeh`.

<verify>
- run: `go test -race ./internal/serve/ ./internal/gurgeh/server/`
  expect: exit 0
</verify>

### Task 0.4: the `autarch serve` command

**Files:**
- Create: `cmd/autarch/serve.go`. Modify: `cmd/autarch/main.go:74-88`
  (`root.AddCommand(serveCmd())`). Test: `cmd/autarch/serve_test.go`.

**Flags:** `--addr` (default `127.0.0.1:8110`), `--project-dir` (repeatable; defaults to the
Bigend daemon's discovery defaults), `--token-file` (default `~/.autarch/serve.token`),
`--allow-origin` (repeatable; default none). Rev 3's `--hub-dir` is dropped: `serve` no longer
touches the hub. SIGINT and SIGTERM cancel the context. The startup line prints the address
and the token file path, never the token.

**Steps:**
1. Failing tests: `--addr 0.0.0.0:0` returns an error containing `local`; a second test starts
   on `127.0.0.1:0`, polls `/health`, and cancels; a third, with `--project-dir` a temp
   parent holding project P, resolves P through `GET /api/projects` and not the parent
   (the harness recipe, Task 1.10) `[I-4]`.
2. Implement; run `go build ./cmd/... && go test -race ./cmd/autarch/ -run Serve`.
3. Commit: `autarch serve: one command for the consolidated service`.

<verify>
- run: `go build ./cmd/...`
  expect: exit 0
- run: `go test -race ./cmd/autarch/ -run Serve`
  expect: exit 0
</verify>

### Task 0.5: MCP into the one binary (P-2)

**Files:**
- Create: `cmd/autarch/mcp.go` (`autarch mcp --project <dir>` calls
  `mcp.NewServer(path).Run(ctx)`). Modify: `cmd/autarch/main.go`.
- Modify: `cmd/autarch-mcp/main.go`: keep its flags, call the same function, and print
  `autarch-mcp is an alias for "autarch mcp"` on stderr.
- Modify: `autarch-plugin/.claude-plugin/plugin.json`: the MCP command becomes `autarch mcp`.
  **Ship-class file:** fd-safety reviews it, and it is republished only with mk's approval
  (G-4).
- Test: `cmd/autarch/mcp_test.go` pipes `initialize` plus `tools/list` through stdin and
  asserts `autarch_list_prds` is listed.

**Steps:** write the test, confirm it fails, implement, confirm it passes, commit:
`mcp: serve from the autarch binary; autarch-mcp stays as an alias`.

<verify>
- run: `go test -race ./cmd/autarch/ -run MCP`
  expect: exit 0
</verify>

### Task 0.6: point the old entry points at `autarch serve`

**Files:**
- Modify: `cmd/bigend/main.go:104` (`runDaemon`), `cmd/autarch/main.go` (the Bigend `--daemon`
  path near `:345`) and `internal/gurgeh/cli/commands/serve.go`. Each keeps working and
  prints `deprecated: use "autarch serve" (mounted at /<prefix>/)` on stderr. Nothing is
  removed.
- **Not** `internal/signals/cli/serve.go` `[B-19]`: Pollard's watcher publishes without a
  token to the standalone Signals server on 8092, so that server stays, undeprecated. The
  fix is a follow-up bead (Task 1.11).
- Modify: `./dev` to add `./dev serve`. Modify `AGENTS.md` to document `autarch serve`, its
  prefixes, the token, the Origin allowlist, `GET /api/projects`, that it holds no decisions,
  and that Pollard still publishes to the standalone Signals server.

**Steps:** run `go test -race ./cmd/... ./internal/gurgeh/... ./internal/signals/...`, then
commit: `serve: deprecation notices on the standalone servers`.

<verify>
- run: `go test -race ./cmd/... ./internal/gurgeh/... ./internal/signals/...`
  expect: exit 0
</verify>

**Pollard's server (8090) is not mounted.** It joins if Home needs it.

---

## Step 1: decisions and the vizier in the Home plugin

All TypeScript paths below are under `integrations/bb-plugin-autarch/`. Test files run with
the scaffold's runner (`npm test -- <file>` runs one file). Store tests open better-sqlite3 on
a temp file directly; the fake SDK's `storage.database()` is used only in wiring tests.

### The ask schema (the shared protocol)

One JSON schema, `schema/home-ask/v1/ask.schema.json`, is the filing request for every
caller. Both Go and TS validate against it and pass the vectors in
`schema/home-ask/v1/vectors.json` `[C-6]`.

```json
{"v":1,
 "kind":"decide",
 "request_id":"req_7f3a…",
 "ask_key":"catch-up collapse order",
 "subject":"autarch/catch-up: collapse order",
 "project":"Autarch","project_root":"~/projects/Autarch",
 "asker":"thread","thread":"thr_abc123",
 "question":"Collapse routine catch-up items per project or per day?",
 "recommendation":"project",
 "supersedes":"",
 "mention_of":"",
 "options":[
   {"id":"project","label":"Collapse per project","kind":"instruction","reversible":true,
    "instruction":"On branch feat/bb-catchup, group routine catch-up items per project, run npm test, commit locally, and report. Do not push or merge."},
   {"id":"day","label":"Collapse per day","kind":"instruction","reversible":true,
    "instruction":"On branch feat/bb-catchup, group routine catch-up items per day, run npm test, commit locally, and report. Do not push or merge."},
   {"id":"ask","label":"Show me both first","kind":"needs-context"}],
 "steps":[],
 "machine":null}
```

Reversible options are local, undoable work. A push to a shared branch, a merge, a deploy
or a release is never marked reversible in examples or docs, and is authorized only by its
own gate (P-6) `[D-2]`.

- `kind` is `decide`, `steps` or `machine` (P-11). `steps` carries ordered step text for mk
  and no options. `machine` carries `{class, detail, owner_thread?}` and no options.
- `asker` is `thread` (with `thread`) or `mycroft` (no thread). `thread` is the
  **recipient** of wakes; the caller's `ctx.threadId` is attribution only and, when present,
  must equal it `[D-16]`.
- `mention_of` optionally names an open ask this filing mentions (P-11) `[D-7]`; it is
  refused together with `supersedes` `[E-7]`.
- `subject` (optional, 1–120 characters, normalized like `ask_key`) names what the ask is
  about and the action asked; automatic mentions need it (P-11) `[E-1]`.
- **Filing identity** is sha256 over the canonical JSON of
  `{scope:{project, project_root, asker, thread}, body}`, where `body` is every other field
  except `request_id`. A missing `request_id` defaults to that identity, so a blind retry is
  idempotent `[A-10]`.
- **Revision** is sha256 over the canonical decision including the scope: project, root,
  asker, thread, question, option ids, labels, kinds, instructions, reversible marks, approval
  specs, recommendation and `supersedes` `[A-8]` `[C-6]`.
- Canonical JSON: keys sorted by UTF-16 code unit, no whitespace, strings NFC-normalized,
  absent and empty optional fields dropped. The vectors cover key order, NFC, non-BMP
  characters and empty fields.
- **The feed never carries the asker's free text.** It carries only the picked label
  `[D21]`. The instruction goes back only to the thread that wrote it.

### Task 1.1: plugin scaffold, shared schema, vectors, and both models

**Step 0 — scaffold first** `[D-14]`: run `bb plugin new bb-plugin-autarch` under
`integrations/` (the scaffold refuses an existing directory, `plugin-scaffold.ts:1424`, so
nothing may create it earlier), add `typecheck`, `test` and `e2e` scripts to `package.json`,
run `npm install && npm test` on the empty scaffold, and commit
`bb-plugin-autarch: scaffold`. Every later `npm test` depends on this.

**Files:**
- Create: `schema/home-ask/v1/ask.schema.json`, `schema/home-ask/v1/vectors.json`,
  `internal/homeask/model.go`, `integrations/bb-plugin-autarch/model.ts`.
- Test: `internal/homeask/model_test.go`, `integrations/bb-plugin-autarch/__tests__/model.test.ts`.

**Design** (identical rules on both sides):
- Kind inference and limits as in rev 3: no kind and no instruction is `needs-context`; an
  instruction and no kind is `instruction`; an instruction on another kind is an error;
  instructions 1–2,000 characters, valid UTF-8, no NUL; option ids unique `[a-z0-9-]{1,32}`,
  defaulting to a slug of the label; 2–6 options for `decide`; labels 1–80 characters;
  `recommendation` names an option; `request_id` 1–128 of `[A-Za-z0-9:_.-]`.
- `reversible` is refused on an option with an `approval` spec, and on any option when
  `asker` is `mycroft`. Mycroft may offer only `ruling-only`.
- `ask_key` is lowercased, whitespace-collapsed, 1–120 characters; it defaults to the
  question's first 120 characters normalized the same way. It never decides a mention alone.
- Any question, label, instruction, step or reason matching `APPROVED-[A-Z]+` is refused
  (`approval tokens are not accepted in Home text`) `[D-2]`.
- `Identity(ask)`, `Revision(ask)` as in the schema section, and `SemanticKey(ask)` (P-11),
  empty when `subject` is absent. `supersedes` with `mention_of` is refused `[E-7]`.

**Test cases:**
- Every validation rule, both default kinds, and the reversible refusals.
- An `APPROVED-MERGE owner/repo#1 abc` line in any text field is refused, on both sides.
- `SemanticKey` differs when kind, subject, question, any option label, instruction,
  reversible mark or approval SHA, or machine detail differs, and not when only `ask_key`
  or key order differs `[E-1]`. "Remove feature A?" and "Remove feature B?" with the same
  yes/no options get different keys; so do two blockers of one machine class with different
  detail; an ask without `subject` gets none.
- `supersedes` and `mention_of` together are refused on both sides `[E-7]`.
- Both sides produce the vector outputs byte for byte (`TestVectors`, `model.test.ts`).
- Changing any scope field (project, root, asker, thread) changes both identity and revision
  `[C-6]`. Reordering keys does not.

Commit: `homeask: shared ask schema, vectors, and Go and TS models`.

<verify>
- run: `go test -race ./internal/homeask/ -run 'Validate|Revision|Identity|Semantic|Vectors'`
  expect: exit 0
- run: `cd integrations/bb-plugin-autarch && npm test -- __tests__/model.test.ts`
  expect: exit 0
</verify>

### Task 1.2: the store

Depends on Task 1.1 (which created the scaffold).

**Files:**
- Modify the scaffold's `server.ts` (entry, wiring only). Create `store.ts`, `migrations.ts`,
  `export.ts`.
- Test: `__tests__/store.test.ts`, `__tests__/store-crash.test.ts`,
  `__tests__/migrations.test.ts`, `__tests__/export.test.ts`.

**Design:**
- The store takes a `Database` (better-sqlite3). `server.ts` passes `bb.storage.database()`.
  On open: `journal_mode = WAL` (set if the host has not), `foreign_keys = ON`,
  `busy_timeout = 2000`, then migrations in one transaction.
- **Staged migrations** `[D-10]`: `schema_meta` holds `schema_version` and
  `min_reader_version`. Migrations are expand-only: new tables, nullable or defaulted
  columns, new indexes that current data satisfies. Nothing is dropped, renamed or retyped,
  and `min_reader_version` rises only in a later release, after every running instance can
  read the new form. An instance starts if its code version ≥ `min_reader_version`, and
  otherwise refuses with a visible error. So an older instance that Aleph keeps after a
  failed candidate activation keeps working.
- **Degraded, retrying initialization** `[D-10]`: the factory never throws on a locked or
  busy database. It registers its surfaces in a not-ready state and retries opening every
  5 s (up to 60 s between tries), because releasing the lock does not reload a failed
  factory. A newer-than-supported schema stays not-ready with its error.
- Tables:
  - `requests(request_id PK, identity, result ('decision'|'mention'), decision_id, thread,
    at)`: **one registry for every filing** `[D-7]`. The recovery read uses it.
  - `decisions(id PK, request_id UNIQUE, identity, revision, semantic_key, subject, kind, ask_key,
    project, project_root, root_dev, root_ino, asker, thread, owner_thread NULL, body_json,
    supersedes UNIQUE NULL REFERENCES decisions(id), delegable, filed_at, updated_at,
    withdrawn_at NULL, resolved_at NULL)`.
  - `picks(decision_id PK REFERENCES decisions, pick_id UNIQUE, option_id, revision, by,
    surface, reason, picked_at, params_hash)`. `by` is `mk`, `vizier` or `rule:<id>`;
    `surface` is `home`, `overlay` or `cli`.
  - `obligations(id PK, decision_id, kind, state, attempt, recipient NULL, payload NULL,
    op UNIQUE NULL, after_id NULL REFERENCES obligations(id), voided_at NULL,
    resend_permit DEFAULT 0, resend_click NULL, last_error, next_try_at, updated_at)`. `state`
    is `pending`, `sending`, `queued`, `uncertain`, `done`, `undeliverable` or `dismissed`
    `[C-4]` `[D-3]` `[D-9]`; `voided_at` is the orthogonal suppression marker `[G-2]`.
    Message rows (every kind but `ruling-file`) are the P-5 **outbox**: `recipient`,
    `payload` and `op` are snapshotted at insert and never recomputed; `after_id` is the row
    this one must follow `[E-2]` `[E-8]`. `attempt` counts attempts rows.
  - `attempts(id PK, obligation_id REFERENCES obligations, n, state, handle NULL,
    evidence NULL, error NULL, at, UNIQUE(obligation_id, n))` `[F-2]`: one row per send;
    `state` is `sending`, `queued`, `delivered`, `not-delivered` or `uncertain`; `handle`
    is the queued row or message id; `evidence` is the matching response, event or row id
    `[F-4]`. `delivered` and `not-delivered` are never left.
  - `queue_events(queued_row PK, type, at)`: dispatch or cancel events that arrive before
    the send response binds the row `[D-9]`.
  - `mentions(decision_id, thread, request_id, at, PRIMARY KEY(decision_id, thread))`
    `[E-3]`. Request uniqueness lives only in `requests`; a mention row is a recipient of a
    decision, so an override can copy it to the replacement without touching the original
    registry row.
  - `events(seq PK AUTOINCREMENT, at, type, decision_id, detail_json)`, append-only. It is the
    source of catch-up facts and the export.
  - `seen(account, item_id, seen_at, PRIMARY KEY(account, item_id))`, `notes(id, at, text,
    cites_json)`, `settings_kv` (including a random `store_id` made by the first migration).
- Primitives, each one transaction:
  - `insertDecision(d)` and `insertMention(m)`: each inserts its `requests` row first; a
    `request_id` conflict returns the existing registry row (decision or mention, compared
    by identity by the caller) and inserts nothing. There is no upsert path `[C-1]` `[D-7]`.
  - `insertReplacement(d)` `[D-8]`: in one `BEGIN IMMEDIATE` transaction, inserts the
    replacement only if both are `kind: decide` (v1: machine and steps asks end only by
    resolve or withdraw) `[I-2]`, the predecessor exists, has no pick, is not withdrawn or
    resolved, has the same asker, thread and project, and has no replacement (`INSERT … SELECT
    … WHERE NOT EXISTS …`). With `changes == 0` it reads the reason in the same transaction
    and returns `not-replaceable`, `already-ruled`, `withdrawn` or `already-superseded`. Every
    replacement copies the predecessor's mention rows as `(replacement, thread)` in the same
    transaction; the original `requests` rows keep pointing at the original decision
    `[D-4]` `[E-3]` `[H-3]`. A replacement of a non-delegable decision is non-delegable
    `[G-4]`. In override mode (Task 1.6) the predecessor must instead have a `by: vizier`
    pick. `overriddenRuling(d)` walks `supersedes` to the nearest ancestor with a
    `by: vizier` pick, or none `[H-3]`.
  - `recordPick(p, obligations)`: an insert into `picks` guarded by `NOT EXISTS` on a pick,
    `NOT EXISTS` on a decision whose `supersedes` is this id, `withdrawn_at IS NULL`,
    `resolved_at IS NULL` and `revision = ?`. Success requires `changes == 1`; then
    obligations and one event are inserted in the same transaction `[C-3]` `[C-5]`. With
    `changes == 0` it returns, from the same transaction, exactly one of `already-ruled`,
    `superseded`, `withdrawn` or `stale` `[D-8]`.
  - `transition(obligation, from, to, attempt, fields)`:
    `UPDATE obligations SET … WHERE id = ? AND state = ? AND attempt = ?`, `changes == 1` or
    it reports a lost race. `done` and `dismissed` are never left, setting `voided_at`
    never changes `state`, and a row with `voided_at` never enters `pending` or `sending`
    `[E-2]` `[G-2]`.
  - A row is **finished** when it is `done`, `undeliverable`, `dismissed` or `uncertain`, or
    has `voided_at` and no attempt `sending` or `queued`.
  - `dueMessages()`: `pending`, due, `voided_at IS NULL`, `after_id` NULL or finished, and
    no unsettled attempt of its own unless `resend_permit = 1` `[G-1]`.
  - `resend(row, attempt, click_id)` `[I-1]`: `UPDATE … SET state = 'pending',
    resend_permit = 1, resend_click = ? WHERE id = ? AND state = 'uncertain' AND attempt = ?
    AND voided_at IS NULL`; with `changes == 0` and `resend_click = click_id` it returns
    the earlier success, otherwise `stale`.
  - **Permit precedence** `[H-1]`: while `resend_permit = 1` the row stays `pending`;
    reconciliation never derives `uncertain` over it. Only a `delivered` attempt (→ `done`),
    Dismiss or `voided_at` clears the permit.
  - `claim(row, attempt)` `[H-2]`: one transaction that re-checks the whole `dueMessages`
    predicate for that row (including `voided_at IS NULL`) in the `UPDATE … WHERE`, then
    inserts attempt n, sets `sending` and clears the permit. The drain sends only after
    `changes == 1`; otherwise it skips the row.
- **Export** `[D-13]`: a nightly job writes immutable segments to
  `~/.autarch/home-export/<store_id>/events-<first_seq>-<last_seq>.jsonl` (temp file, `fsync`,
  rename), then advances the cursor in `settings_kv`. At start the cursor is the larger of
  the stored cursor and the highest `last_seq` among existing segments, so a crash on either
  side of the rename neither duplicates nor loses events. Each line carries
  `(store_id, seq)`. A retained `data.db` after reinstall keeps its `store_id`; a reset
  database gets a new one and a new directory. It lives outside the plugin folder so it
  outlives removal.
- `README.md` states that `bb plugin remove` leaves `data.db` behind, and how mk resets it.

**Test cases:**
- Filing the same `request_id` twice gives one row; two connections to one file racing the
  same insert give one row, and both callers get its id.
- Two connections racing `recordPick` on one decision with different pick ids: exactly one
  succeeds; the other sees `changes == 0` `[C-3]`.
- `recordPick` refuses a superseded decision, a withdrawn one and a stale revision, writes
  nothing, and returns `superseded`, `withdrawn` or `stale` respectively, never
  `already-ruled` `[D-8]`.
- A second decision with the same `supersedes` is refused by the unique index `[C-5]`.
- **Replacement vs pick, two connections** `[D-8]`: connection 1 opens `insertReplacement`
  for A, connection 2 picks A first; the replacement is refused `already-ruled` and no
  replacement row exists. In the other order the pick returns `superseded`.
- A filing and a mention with the same `request_id` race on two connections: one `requests`
  row; the loser gets the winner's result `[D-7]`.
- `transition` with a stale `attempt` or state changes nothing; `done` never goes back to
  `pending`; a `voided_at` row never goes to `pending` or `sending` `[E-2]`; setting
  `voided_at` on a `done` row leaves it `done` `[G-2]`.
- `dueMessages` withholds a row until its `after_id` row is finished, and never returns a
  row with `voided_at` `[E-2]`.
- **Attempts and the permit** `[F-2]` `[G-1]`: an obligation with attempt 1 `uncertain`
  and attempt 2 `queued` keeps both; a `delivered` attempt is never changed by a later
  event; a `pending` row with an `uncertain` attempt is due only while `resend_permit = 1`,
  and reconciling that attempt leaves the row `pending` `[H-1]`; `claim` clears the permit with
  the attempt insert, and a `claim` after `voided_at` is set changes nothing `[H-2]`.
- **Recipient set** `[F-6]`: a `steps-done` completion with two mentioners inserts three
  rows with ops `steps-done:<decision>:<thread>` in one transaction; a retry inserts none; a
  SIGKILL mid-transaction leaves none or all three.
- **Override with mentions** `[E-3]`: A has mentions from threads B and C (requests R1, R2);
  an override-mode replacement copies both mention rows; retries of R1 and R2 then return
  their original result (a mention of A), and the replacement lists B and C as recipients.
- **Crash:** a child process runs `recordPick` and is SIGKILLed at a hook between statements
  and after commit, 50 times; the parent opens the file and finds either no pick and no
  obligations, or both, never one without the other.
- A database at an older schema version migrates; one whose `min_reader_version` exceeds
  the code refuses and stays not-ready.
- **Failed activation after migration** `[D-10]`: version N's store is open; a version N+1
  candidate migrates the same file, then its factory throws; version N's store still files,
  picks and reconciles on that file.
- **Locked at start:** the factory returns with the store not-ready; after the lock is
  released, the retry opens it without a reload.
- **Export boundaries** `[D-13]`: a crash before the segment rename, after the rename before
  the cursor update, and after the cursor update each end, after restart and one more run,
  with every `(store_id, seq)` exported exactly once; a reinstall over the retained file
  keeps the `store_id`; a reset starts a new directory.

Commit: `bb-plugin-autarch: store with conditional picks, staged migrations and export`.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm test -- __tests__/store.test.ts __tests__/store-crash.test.ts __tests__/migrations.test.ts __tests__/export.test.ts`
  expect: exit 0
</verify>

### Task 1.3: ruling files and the safe writer `[A-19]` `[C-14]`

Depends on Task 1.2.

**Files:** create `ruling.ts`; test `__tests__/ruling.test.ts`.

**Ruling format** `[D16]` (unsigned in v1): as in rev 3 (ratification block with `ruled_by`,
`ruled_at`, `ruling`, `transcribed_by: autarch-home`, `session_id`, `source`, `supersedes`;
a `home` block with decision id, `pick_id`, revision, asking thread, `options_shown`,
`picked`, and the instruction for instruction picks). Rev 4 adds `ruled_by: vizier` with
`delegated_reason` for delegated rulings, and `mentions` for mentioned threads.

**Rules:**
- `rulingPath(d)`: `docs/decisions/YYYY-MM-DD-<slug>-<id>.md` under the project root; estate
  rulings under `$AUTARCH_UQBAR_DIR/rulings/`. Estate filing is refused at `file` time when
  the Uqbar is unset (`estate-wide decisions need an Uqbar (see G-1)`).
- **Trusted root.** The root is the one `serve` resolved at filing, saved with its `dev` and
  `ino`. At write time the writer:
  1. `lstat`s the saved root path; it must be a directory, not a symlink, with the saved
     `dev` and `ino`. A replaced or retargeted root is refused with `project root changed
     since filing`.
  2. walks `docs/decisions` one component at a time with `lstat`, refusing any symlink, and
     creates missing directories with `mkdir` (mode 0755);
  3. opens a temp file with `O_CREAT|O_EXCL|O_WRONLY|O_NOFOLLOW`, writes, `fsync`s, and
     renames it into place;
  4. re-checks the root's `dev` and `ino` after the rename.
- Node has no `openat`, so a swap between steps is possible for a process running as mk. That
  residual is the same-user risk accepted in `[D21]`, and is stated in the README.
- Writing is idempotent: same path, same bytes.

**Test cases:**
- Round trip of the frontmatter, including `options_shown`, the instruction and
  `ruled_by: vizier`.
- Writing twice gives one file with the same bytes.
- A `docs` or `docs/decisions` symlink pointing outside the root is refused; nothing is
  written outside.
- **Root replacement:** rename the root away and create a new directory at the same path; the
  write is refused.
- **Symlink retargeting:** the configured root path is a symlink retargeted after filing; the
  write is refused.
- An instruction containing `---` and YAML-looking lines round-trips as a string.

Commit: `bb-plugin-autarch: ruling files through a root-pinned writer`.

#### Accepted residual: ruling-file writer (same uid)

A process running as the same user can move directories the writer holds. Two residuals remain
and are accepted:

- **Linux (fd-anchored).** The writer works through held directory descriptors and re-reads
  `/proc/self/fd/N` before the temp write, before the rename, after it, and at the end (the
  identical-existing-file path runs the same checks). A move landing between the last check and
  the rename, or after the final check, can leave one file outside the project until it is
  detected; removal is attempted, but the window is not closed.
- **macOS (path-based).** There is no `/proc`, so the writer falls back to paths and a swap
  between steps is possible.

Status: accepted by the vizier 2026-09-30 in mk's place, conditional on a clean final Sol review.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm test -- __tests__/ruling.test.ts`
  expect: exit 0
</verify>

### Task 1.4: the service — file, pick, reconcile, ack, feed and stats `[C-12]`

Rev 3's Tasks 1.4 and 1.5 are merged here, because the service tests need the feed.
Depends on Tasks 1.2 and 1.3.

**Files:** create `service.ts`, `feed.ts`; test `__tests__/service.test.ts`,
`__tests__/feed.test.ts`.

**Readiness** `[C-7]`: the service is ready once the store is open and migrated. Nothing else
gates writes. A failing obligation keeps its own `last_error` and `next_try_at` (backoff 30 s,
doubling to 1 h) and is shown on Home; other decisions are unaffected.

**`file(req, ctx)`** `[A-10]` `[C-6]`:
1. Validate (`supersedes` with `mention_of` is exit 2 `[E-7]`).
2. **Known requests first** `[E-10]`: if `request_id` is in `requests`, the same identity
   returns the recorded result, decision or mention, with its canonical identity (200); a
   different identity returns 409 `request id reused for a different decision` `[D-7]`.
   This needs no `serve`.
3. Only for a new insertion, resolve the project through `serve`'s `GET /api/projects`;
   `project_root` must equal the resolved root; save `dev` and `ino`. If `serve` cannot
   answer, return `not-filed: project resolution unavailable` (exit 3).
4. If `supersedes` is set, this is the only branch `[E-7]`: `insertReplacement` (Task 1.2)
   checks and inserts in one transaction, with no mention matching. `already-ruled` returns
   409 `already ruled: "<label>"`; `not-replaceable` returns 409 `only decide asks can be
   superseded` `[I-2]`; `already-superseded` returns 409 `already superseded by
   <id>`; `withdrawn` returns 409 `withdrawn` `[D-8]`.
5. If `mention_of` names an open ask from a different thread, or an open ask from a
   different thread in the same project is semantically equal (P-11), insert a mention and
   return that ask's id with `mentioned: true` `[D-7]` `[E-1]`. A `mention_of` naming a
   picked, withdrawn or unknown ask returns 409 with its state. A matching `ask_key` alone
   files a new decision and records `related: <id>`.
6. Otherwise insert the decision.
   **Supersession writes no close and creates no obligation** `[C-5]`: the predecessor stops
   being owed because a replacement references it, and `recordPick` refuses it. Automatic
   closing of superseded decisions elsewhere (bd, the hub copy) is deferred.

**`pick(id, optionID, revision, pickID, by, surface)`** `[B-1]` `[C-3]` `[C-6]`:
1. Read the decision. Recompute its revision from the stored body; a mismatch with the stored
   revision is a store fault (500, logged); a mismatch with the caller's is 409
   `revision differs` and the surface re-reads.
2. If a pick exists: the same `pick_id` and the same `params_hash` (option, revision, by)
   return it; the same `pick_id` with other parameters is 409 `pick id reused`; another
   `pick_id` is 409 `already ruled`.
3. `recordPick` with obligations: `ruling-file`, plus `wake` for instruction and needs-context
   picks and for every `by: vizier` pick. Only an `already-ruled` result goes back to step 2;
   `superseded`, `withdrawn` and `stale` return 409 with that reason and never loop `[D-8]`.
4. After commit, `reconcile(id)` and nudge the wake loop. Return the pick.

**`reconcile(id)`** carries out `ruling-file` obligations: write the file, then
`transition(pending → done)`. A failure sets `last_error` and `next_try_at` and leaves it
pending. `reconcileAll()` runs at start and every 30 s over obligations with
`state = 'pending' AND kind = 'ruling-file' AND next_try_at <= now`.

**Wake rows.** `recordPick` inserts each `wake` with its snapshot (P-5): recipient (the
decision's `thread`), `op` `wake:<decision>:<pick_id>`, and the payload rendered at pick time
(question, picked label, kind, `by`, and the exact instruction for instruction picks). A
pick on a decision with an `overriddenRuling` A starts its payload with "This supersedes the
vizier's ruling on <A>." (P-10) `[H-3]`. **`wakes()`** returns `dueMessages()`, one row
per message, never grouped `[E-4]`.

**`dismiss(decision, obligation)`**: `undeliverable → dismissed`. Terminal.

**Feed** `[A-20]` `[B-12]` `[C-9]`:
- `feed(project, thread)` returns `{own:[line…], project:[line…]}`. Each line is structured:
  `{decision, label, picked_at, by, project, status}`.
- **Only effective rulings are instructions** `[D-4]`: a pick whose decision has a
  replacement is historical. `project` lines exclude it. `own` shows it once as
  `status: "void"` (rendered `void <id>: overridden, awaiting mk (<tip>)`, where the tip
  is the end of its replacement chain) until the tip is picked or withdrawn `[I-3]`.
- A replacement's ruling is published as soon as it is picked, with no barrier (P-10); with
  an `overriddenRuling` A its line adds `, supersedes <A>` `[H-3]`.
- `own`: effective picks on decisions this thread filed or mentioned, following the
  replacement chain (mentions are copied to a replacement), any project, picked within 30
  days, newest first, up to 10.
- `project`: effective picks in this project within 14 days, newest first, up to 10,
  excluding those already in `own`.
- Ordering within the same timestamp is by decision id, so same-day ordering is stable.
- Rendered text: `ruled 2026-09-26T14:03Z "<label>" (<id>)`, plus ` [vizier]` for delegated
  rulings. The label is quoted, stripped of newlines and control characters, and clipped to
  80 UTF-16 code units without splitting a surrogate pair.
- **Budget:** the configured text is at most 4096 UTF-16 code units
  (`PLUGIN_AGENT_DYNAMIC_INSTRUCTIONS_MAX_CHARS`, `host-policy.ts:173`, which truncates by
  `slice`). The header and filing note are reserved first, then own lines, then project
  lines; lines are dropped whole, oldest project lines first. Own lines are dropped only
  after every project line, oldest first.
- A line never contains the question, the instruction or any other field `[D21]`.
- `recent(project, limit)`, `owed()`, `undeliverable()` and `failures()` read the store.

**Stats** (the success measure): `stats(since)` returns asks filed per ISO week; picks by
`by` and `surface`; median and p90 of `picked_at − filed_at` for picks by mk; delegated
rulings; overrides; override rate (overrides ÷ delegated rulings); and median time from a
delegated ruling to its override.

**Test cases** (each asserts the rows, the ruling file, and what `owed`, `wakes` and `feed`
return):
- Filing twice with one `request_id` gives one row and one event.
- A reused `request_id` with a different thread or project is refused 409 `[C-6]`.
- Picking an instruction option: ruling file written, one wake with the exact instruction.
  Ruling-only by mk: no wake; the feed carries it.
- Retry with the same `pick_id`: one pick. Same `pick_id`, other option: 409 `pick id reused`.
- Two surfaces race a pick: one wins, the other gets 409 `already ruled`.
- **Replacement before pick** `[C-4]` `[C-5]`: file A, file B superseding A, pick A → 409
  `superseded`; A is not owed; B is.
- **Two replacements:** B and C both supersede A; C gets 409 `already superseded by B`.
- **Machine ask not replaceable** `[I-2]`: filing B superseding machine ask A (or a `steps`
  ask) → 409 `only decide asks can be superseded`; nothing inserted; A's owner notice
  unchanged.
- **Dismiss then restart** `[C-4]`: an undeliverable wake is dismissed; a new service over the
  same file returns no wake and no undeliverable item for it.
- **Ruling-file failure is local** `[C-7]`: project P's root is unwritable; a pick in P leaves
  its `ruling-file` pending with `last_error`; a pick in project Q succeeds and its file is
  written; after a restart both states hold and the service is ready.
- **Mention** `[D-7]`: a second thread files a semantically equal ask; one entry with one
  mention; after the pick both threads' `own` feed lines include it. Same `ask_key` with a
  different kind, instruction or approval SHA files a separate decision marked related.
- **Unrelated asks** `[E-1]`: "Remove feature A?" and "Remove feature B?" with identical
  yes/no needs-context options, and two blockers of one machine class with different detail,
  file separate decisions; the second thread never receives the first ruling. Two equal
  asks without `subject` file separately; `mention_of` still joins them.
- **Replacement is exclusive** `[E-7]`: B supersedes A and is semantically equal to thread
  C's open ask; B is filed as A's replacement, not a mention of C, and picking A returns
  `superseded`. `supersedes` plus `mention_of` exits 2.
- **Retry while `serve` is down** `[E-10]`: file succeeds, the response is lost, `serve`
  stops; the retry returns the same result with status 200, not exit 3.
- **Mention retry** `[D-7]`: thread B files R as a mention of A; A is picked; B retries R and
  gets the same mention result, and no new decision exists.
- **Supersede outcomes** `[D-8]`: picking a withdrawn decision returns 409 `withdrawn`, not a
  retry of the existing-pick branch; a replacement for a picked predecessor returns 409
  `already ruled`.
- **Effective rulings** `[D-4]` (replacement made with override-mode `insertReplacement`):
  after an override, the project feed omits the old label;
  the asker's `own` shows it void; after mk picks the replacement, with the old wake still
  `queued`, only the new label appears, at once and carrying `supersedes <old id>`, and a
  thread that had mentioned the original sees it too. The replacement's wake payload starts
  with the supersedes line.
- **Feed:** own answers survive when 10 project lines of 80 non-BMP characters fill the budget;
  no surrogate is split; same-timestamp lines order by id; a thread never sees another
  thread's own lines; neither question nor instruction appears.
- Stats over fixed timestamps.

Commit: `bb-plugin-autarch: file, pick once, reconcile, feed and stats`.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm test -- __tests__/service.test.ts __tests__/feed.test.ts`
  expect: exit 0
</verify>

### Task 1.5: structured asks — mentions, the machine lane, the runbook `[D25]`

Depends on Task 1.4.

**Files:** create `asks.ts`; test `__tests__/asks.test.ts`.

**Design:**
- `kind: steps`: filing stores the steps. Home shows one **runbook** merging every thread's
  open steps, grouped by thread, in filing order. mk marks a thread's steps done with one
  click (a `pick_id`, the same conditional pick with `option_id: "done"`), which creates,
  in the pick's transaction, one row per recipient (the asker and every mentioning thread):
  "mk finished your steps: <step list>" `[F-6]`.
- `kind: machine`: the owner is `owner_thread` if given, else the `machineOwners` setting
  (class → thread id). With an owner, the ask goes to the **machine lane** (a collapsed
  section on Home, not the Asks panel) and the owner thread gets a wake. With no owner, it
  appears in Asks as "unowned machine blocker". An owned blocker whose `updated_at` is older
  than 24 h is promoted to Asks as "stalled".
- **Blocker lifecycle** `[D-11]`: `bb home progress <id> --note <text>` (owner thread) sets
  `updated_at` and appends an event; `bb home resolve <id> [--note]` (owner or asker) sets
  `resolved_at`, removes it from the lane and from Asks even if promoted, and wakes the
  asker and every mentioning thread; `bb home withdraw <id>` (asker) sets `withdrawn_at`
  and wakes the owner. The asker, owner and mentioning threads are stored ids, so recipients
  survive restarts and a later `machineOwners` change. Who may call each is checked against
  `ctx.threadId`, which is advisory until G-15 (P-10). Each is idempotent per `request_id`.
- **Every notice is an outbox row** (P-5) `[E-8]`, inserted in the same transaction as the
  lifecycle change, with its own recipient, payload and `op`: `owner:<decision>`,
  `steps-done:<decision>:<thread>`, `resolve:<decision>:<thread>`,
  `withdraw:<decision>:<thread>`; every `op` names its recipient, and a fan-out inserts the
  whole recipient set in one transaction `[F-6]`. A resolve or withdraw sets `voided_at` on
  the owner notice; if that notice has no attempt, the owner gets nothing more, otherwise
  the owner also gets the resolve or withdraw notice with `after_id` = the owner notice.
  An `uncertain` owner notice counts as finished, so the resolve can arrive first `[H-5]`:
  the owner notice therefore says "confirm with `bb home get --id <id>` that this blocker is
  still open before acting", and `get --id` returns its `resolved_at` and `withdrawn_at`.
  A machine ask cannot be superseded; only resolve or withdraw ends it `[I-2]`.
- Mentions work for every kind (P-11 rules).
- The schema reserves `by: "rule:<id>"`; nothing in v1 writes it.

**Test cases:** steps from two threads form one runbook; marking one thread's steps done wakes
only that thread, once across a retry; a machine blocker with an owner is not in Asks and
wakes the owner; without an owner it is in Asks; a stalled owned blocker is promoted; a
mention on a `steps` ask wakes both threads when done. Blockers `[D-11]`: `progress` resets
the 24 h clock; `resolve` before promotion and after promotion both remove it and wake the
asker once across a retry; `resolve` or `progress` from an unrelated thread is refused;
`withdraw` wakes the owner; recipients survive a restart and a `machineOwners` change.
Outbox `[E-8]`: file a blocker, then resolve it before the drain runs: the owner notice gets
`voided_at` and is never sent, and the asker and mentioners each get one resolve notice with its own
payload; the same with `withdraw`; after a restart between the two steps the rows and
payloads are unchanged; an owner notice already `done` is followed by the resolve notice.
**Late owner delivery** `[H-5]`: owner notice `uncertain` → resolve (its notice is sent) →
the owner notice is delivered late; its text asks for confirmation, and `get --id` reports
the blocker resolved. Fan-out `[F-6]`: a `steps` ask with two mentioners, marked done, inserts three
`steps-done` rows in one transaction; a retry of the click inserts none; after a restart
each thread gets exactly one message.

Commit: `bb-plugin-autarch: runbook, machine lane and mentions`.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm test -- __tests__/asks.test.ts`
  expect: exit 0
</verify>

### Task 1.6: delegated rulings and overrides (P-10) `[V4]` `[V7]` `[V8]`

Depends on Task 1.4. **Foundational: cross-lab review items 1–3.** Here the override is
tested against wakes set to each state directly; the live send interleavings are in Task
1.7's wake tests.

**Files:** create `delegation.ts`; test `__tests__/delegation.test.ts`.

**Design:**
- `rule(decision, option, reason, ctx)`: refused unless `ctx.threadId == vizierThreadId`
  (advisory, P-10), the decision's asker is a thread, the decision is `delegable`, the option
  is `reversible`, and the bounded policy allows it: project in `delegation.projects`, under
  `delegation.dailyCap`, no `approval` spec, not an override replacement, and delegation not
  suspended by an unseen settings change `[D-2]` `[D-1]`. It calls
  `pick(…, by: "vizier", surface: "cli")` with the reason (1–500 characters, required, no
  approval token).
- The wake text for a delegated ruling starts: "The vizier ruled on your decision <id> in mk's
  place (you marked this option reversible). mk may override it. This is not merge, deploy,
  release or publish authorization." Then the rev-3 answer body.
- `setDelegation(settings)`: panel RPC only (no CLI verb). It records a
  `delegation-settings-changed` event, pinned in catch-up, and suspends `rule` until mk's
  account has seen that event `[D-1]`.
- `override(decision, ctx)`: only from Home (panel RPC), only on a `by: vizier` pick.
  In one transaction it files the superseding decision with override-mode
  `insertReplacement` (request id `override:<id>`, same question, options, thread and
  mentions, `delegable = 0`), records an `overridden` event, invalidates the feed caches,
  sets `voided_at` on the old wake W, asks the wake loop to remove W's queued attempt where
  the SDK allows, and inserts one `void-notice` to the asker (`op` `void:<decision>`, so a
  second override inserts none) (P-10) `[D-3]` `[E-2]` `[G-2]`. Nothing waits on W, whose
  state is still reconciled; the asker may act on the old ruling before the notice arrives.
- Catch-up pins every delegated ruling until mk's account has a `seen` row for it.

**Test cases:**
- **Advisory claimed-caller checks** `[E-13]` `[F-8]`, three separate assertions:
  - `rule` with a claimed `ctx.threadId` other than `vizierThreadId` is refused. A forged
    `BB_THREAD_ID` equal to `vizierThreadId` from another thread **is accepted** and
    recorded `by: vizier`, and is still held to every delegation limit (refused for an
    irreversible option, a project not enabled, past the cap), woken with the
    non-authorization line, and pinned with Override.
  - A direct `pick` RPC from a non-mk caller, with delegation disabled, **succeeds** on an
    irreversible ruling-only option and is recorded `by: mk` with no wake: this documents
    the admitted attribution bypass. Nothing in the test or the design applies delegation
    limits to human picks.
  - Authenticated human/vizier separation is not asserted; it is G-15.
- `rule` on an irreversible option, a Mycroft decision or an override decision is refused;
  with no reason is refused.
- **Bounded policy** `[D-2]`: `rule` is refused for a project not enabled, past the daily
  cap, and while a settings change is unseen; after mk sees it, `rule` works again. A
  reversible option whose instruction says to push and merge, ruled by the vizier, produces
  only a wake whose text carries the non-authorization line and no string matching
  `APPROVED-(MERGE|DEPLOY|RELEASE)`; a reason carrying such a token is refused.
- `vizierThreadId` and `delegation.*` cannot be changed through any `bb home` CLI verb.
- A delegated ruling wakes the asker, says "vizier", and is pinned in catch-up.
- **Override by wake state** `[D-3]` `[G-2]`: W with no attempt, or with an attempt in
  `sending`, `queued`, `uncertain` or `delivered`, gets `voided_at` and exactly one
  `void-notice` to the asker, with no `after_id`; W keeps its state (a `done` W stays
  `done`, an in-flight attempt still settles), and `dueMessages` never returns W again
  `[E-2]`. The new decision is owed to mk and not delegable.
- **Replacement answer** (ruling B): mk picks the replacement while W is still `queued`;
  its wake is due at once and its payload and feed line say it supersedes the vizier's
  ruling on W's decision. A restart between the override and the pick creates no second
  notice.
- **Supersession chain** `[G-4]` `[H-3]`: A has mentioner M; override creates B; the asker
  files C superseding B. C is not delegable (`rule` refused) and has M as a mention; mk
  picks C: C's wake and M's own feed line say it supersedes the vizier's ruling on A
  (C is `decide`, so machine and steps fan-out stays in Task 1.5) `[J-1]`. Before the pick, A's void line in the
  asker's and M's feeds says `awaiting mk (C)`; after it, the line is gone from both `[I-3]`.
- **Notice after the answer** `[H-4]`: mk picks B and B's wake is delivered before the void
  notice; the notice names B and says an answer already received stands. A duplicate
  delivery of the notice (Resend) carries the same text, and a second override adds none.
- A second override of the same ruling returns the first override's decision (unique
  `supersedes`).
- The old pick row is unchanged after override.
- A `by: vizier` pick on an option with an `approval` spec is impossible (refused at filing).

Commit: `bb-plugin-autarch: delegated rulings and superseding overrides`.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm test -- __tests__/delegation.test.ts`
  expect: exit 0
</verify>

### Task 1.7: plugin surfaces — RPC, wakes, feed injection and the `bb home` CLI

Depends on Tasks 1.4–1.6.

**Files:** create `contract.ts`, `wakes.ts`, `cli.ts`, `catchup.ts`; modify `server.ts`,
`feed.ts`; test `__tests__/wakes.test.ts`, `__tests__/configure.test.ts`,
`__tests__/cli.test.ts`, `__tests__/catchup.test.ts`.

**Step 0:** check the installed SDK for (a) removing a queued message by row id, (b)
reading a queued row's state and text by id, (c) listing a thread's queued rows, (d)
reading a thread's accepted `client/turn/requested` events with their input and id, and
(e) which `threads.send` errors are proven pre-acceptance (the request never reached the
server). Record all five in the commit body. Without (a), an overridden wake already
queued may still arrive (best effort, P-10); without (b)–(d), unresolved attempts stay
`uncertain`; without (e), every send failure is an unknown outcome `[E-5]` `[F-1]` `[F-4]`.

**RPC** (`defineRpcContract`): `listAsks`, `listRecent`, `pick`, `dismiss`, `override`,
`setDelegation`, `resend`, `markSeen`, `markAllSeen`, `catchup`, `stats`, `health`. RPC
carries no attested caller, so `pick` records `by: mk` as advisory attribution (P-10, G-15).
`health` includes `source_sha256`, recomputed at load over the plugin's own root (Task
1.10), the `build` reported by the `serve` it is connected to, and the project roots from
an authenticated `GET /api/projects` (or its error) `[F-7]` `[G-7]` `[H-6]`.

**`markAllSeen(snapshot)`** `[D-12]`: the panel sends the immutable item ids it rendered
expanded and visible, captured when mk clicked, while the document was visible and focused.
The server marks exactly those ids. Items inserted after the snapshot, collapsed items and
offscreen items stay unseen, and a delegated ruling is never marked by it unless it was in
the snapshot.

**Serve supervisor** (background): if `GET <serveAddr>/health` fails (`serveAddr` setting,
default `127.0.0.1:8110`), spawn `autarch serve --addr <serveAddr> --token-file
<serveTokenFile> [--project-dir <d>…]` (`AUTARCH_BIN`, `serveTokenFile` default
`~/.autarch/serve.token`, `serveProjectDirs` default Bigend's), at most 3 restarts per 10
minutes, never while `/health` answers. The plugin reads the token from `serveTokenFile`,
server-side only `[H-6]`. Only `file` needs `serve`.

**Wake loop** `[A-15]` `[B-4]` `[C-8]` `[F-1]` `[F-4]` `[G-1]`:
- Drains are serialized: one drain runs at a time; a nudge during a drain schedules one more.
- Each drain takes `dueMessages()` (Task 1.2) in obligation-id order. For each row it calls
  `claim` (Task 1.2), which re-checks eligibility and `voided_at` in the claiming
  transaction; a failed claim skips the row `[D-3]` `[H-2]`. Only then it sends **exactly
  one message per row** `[E-4]`: `bb.sdk.threads.send({threadId: recipient, input: "[home-msg <op> #<n>] " +
  payload, mode: "queue-if-active"})`.
- Attempt outcomes, each a conditional update of that attempt:
  - `delivery: "sent"` → `delivered` (evidence: the response's message id).
  - `delivery: "queued"` → `queued`, storing the row id as `handle`, then applying any
    `queue_events` row already recorded for it `[D-9]`.
  - A rejection because the thread is archived or deleted → `not-delivered`; the row is
    `undeliverable`.
  - A proven pre-acceptance rejection (step 0 (e)) → `not-delivered`; the row returns to
    `pending` with `last_error` and `next_try_at`, unless it has `voided_at`.
  - **Anything else is an unknown outcome** `[F-1]`: the attempt is `uncertain` and is
    reconciled at once (below). It never returns the row to `pending`.
- **The row's state follows its attempts:** any `delivered` → `done`; else any `sending` or
  `queued` → that state; else any `uncertain` → `uncertain`; else (all `not-delivered`) →
  `undeliverable` if it was removed or its thread is gone, `pending` after a proven
  pre-acceptance rejection; but a row holding `resend_permit` stays `pending` `[H-1]`. A
  row with `voided_at` never goes to `pending` or `sending`; Home lists it as voided.
- Events (`backend-contract.ts:295-340`): `message.dispatched` or `message.cancelled` are
  first recorded in `queue_events` by row id, then applied to the attempt holding that
  `handle`, so an event that arrives before the send response is not lost `[D-9]`.
  Dispatched → `delivered`. Cancelled → `not-delivered`. `thread.archived` or
  `thread.deleted` → that thread's `queued` attempts → `not-delivered`, and its rows →
  `undeliverable`.
- **Reconciliation** `[D-9]` `[E-5]` `[F-4]`: at start, after each response, after each
  unknown outcome, and every 60 s, every `sending`, `queued` or `uncertain` attempt is
  checked. With a handle: read the row by id (step 0 (b)): still queued → stays;
  dispatched → `delivered` (text mk edited counts, P-5); cancelled → `not-delivered`.
  Without a handle (a crash or lost response before binding): search the
  recipient's queued rows and accepted `client/turn/requested` events (step 0 (c), (d)) for
  input equal to the framed marker plus snapshotted payload. Exactly one queued row → bind
  it; exactly one accepted event → `delivered`, storing its id; none, a partial match (a
  quoted marker, edited text) or several → `uncertain`. Home never resends, or treats as
  cancelled, on marker absence alone.
- **Resend** (mk, from Home; refused if `voided_at` is set) moves an `uncertain` row to
  `pending` and sets its one-shot `resend_permit` in one transaction `[G-1]`, through
  `resend(row, attempt, click_id)`: the RPC carries the attempt count Home displayed and a
  `click_id` made once per click and reused on retries, so a replay never re-arms `[I-1]`. The
  drain's claim consumes the permit as it inserts the new attempt, so one Resend makes exactly
  one attempt, across restarts. Earlier uncertain attempts keep being reconciled, and a later
  `delivered` on any completes the row. **Dismiss** (`uncertain → dismissed`) stops listing
  it.
- A late failure never overrides a success: `delivered` and `done` are terminal and every
  update is conditional on state.

**Feed injection** `[D-17]`: a background refresher keeps two caches from the store, rebuilt
every 30 s and after each pick or override: a **shared per-project** cache (project lines)
and a per-thread cache (own lines). `bb.agents.configure` (synchronous) combines them for
the thread and its project: header `Recent rulings (label only):`, own lines (if any),
project lines, and a filing note: "When you need mk, end your turn with an ask: run
`bb home ask --request-stdin` with one line of JSON. Options default to needs-context. Give
an option an `instruction` when you know what you'd do if mk picks it, and mark it
`reversible` only if undoing it is cheap and local; never for a push, merge, deploy or
release." A thread with no own lines still gets its project's lines; only when both caches
are empty for that project are no instructions returned.

**CLI** (`defineCli`, `bb home …`) `[B-14]`:
- `ask` (`--request-stdin` named option declared `stdin: true`, or `--request '<json>'`,
  16 KiB max) `[D-16]`: the recipient is the request's `thread`, defaulting to
  `ctx.threadId`; if both are present and differ, it exits 2 `thread conflicts with caller
  context`. Outside a thread only `asker: mycroft` is accepted. Prints
  `{"id":…,"request_id":…,"mentioned":bool}`. Exits 2 validation, 3 not filed, 5 already
  ruled or superseded.
- `get --request-id <id>` (the `requests` registry, decision or mention; used by the Go
  recovery read) or `get --id <decision>` (with lifecycle state `[H-5]`),
  `list [--asker] --json`, `stats [--since 14d] --json`, `feed [--project] [--thread] --json`.
- `progress`, `resolve`, `withdraw` (Task 1.5).
- `rule <id> <option> --reason <text>` (P-10, vizier only). No verb changes settings.
- `note --cites <id,…> <text>` (vizier only): stores a catch-up note; refused unless every
  cited id is an existing fact.

**Catch-up** `[V2]` `[V9]` `[V10]`: facts from `events` since each item's `seen` state: rulings
made (by mk and by the vizier), asks opened and closed, undeliverable wakes, and runs failed
(the plugin records `turn.failed` for threads that have filed asks). Ranking: failures, then
owed asks, then delegated rulings; routine items collapse per project; delegated rulings stay
pinned until seen. Vizier notes follow the facts, marked "vizier's note", each linking its
cited facts. Run completions are listed only if Task 1.8's step 0 finds a completion event or
Bigend session end via `serve`; otherwise catch-up ships without them and the gap is reported.

**Test cases** (fake SDK):
- Wakes: one message per row with the exact snapshotted payload and its `op` marker; only
  the sent row is completed; a wake added during a send stays pending.
- **No batching** `[E-4]`: wakes A and B for one thread are two queued rows; overriding A
  removes only A's row; B dispatches and ends `done`, including across a restart and a
  dispatch racing the removal.
- **Crash before binding** `[E-5]`: the send queues row Q, the service is killed before Q's
  id is stored, mk cancels Q, the service restarts: the row is `uncertain` (or
  `undeliverable` when (c) finds Q cancelled), never resent until mk picks Resend; with Q
  still queued, (c) binds it and it ends `done` on dispatch.
- **Ambiguous failure, no crash** `[F-1]`: the fake queues Q, then throws a network error
  instead of responding; mk cancels Q; the reconciliation finds Q cancelled → attempt
  `not-delivered`, row `undeliverable`, never resent. Variant with (c) and (d) unavailable:
  the row is `uncertain` and nothing is sent again until mk picks Resend. A proven
  pre-acceptance rejection (connection refused) returns the row to `pending` and the retry
  sends once.
- **Delivery evidence** `[F-4]`: a diagnostic message in the recipient thread quoting
  `[home-msg <op> #1]` does not complete the attempt; an unbound queued row whose text was
  edited but kept the marker leaves it `uncertain`; two accepted events with the exact framed input
  leave it `uncertain`; exactly one completes it and stores that event's id.
- **Resend through the real drain** `[G-1]` `[F-2]`: Resend on an `uncertain` row → the
  drain sends attempt 2 exactly once; a restart after Resend but before the drain, and one
  after the claim but before the send, still give exactly one attempt 2 (the latter ends
  `uncertain`, never a third attempt); a repeated Resend while the row is `pending` is
  refused; attempt 1 later reconciles `delivered` → the row is `done` and attempt 2, if
  still queued, is removed where the SDK allows. `[H-1]`: startup reconciliation before
  the first drain, and a reconciliation while another row's send blocks the drain, both
  leave the Resent row `pending` with its permit, and attempt 2 is sent once. `[I-1]`:
  the Resend response is lost, attempt 2 becomes `uncertain`, and the RPC is retried with
  the same `click_id` (also after a restart): no permit, no attempt 3; a new click showing
  attempt 1 is refused `stale`.
- **Stale selection** `[H-2]`: the drain selects `[X, W]` and X's send blocks; an override
  voids W (variant: a resolve voids owner notice O); X completes; W's (O's) claim fails and
  nothing is sent for it.
- `queued` then `message.dispatched` → done. Queue then archive → undeliverable. Queue then
  `message.cancelled` → undeliverable. Restart while queued, then the row dispatches → done.
- **Event before response** `[D-9]`: the fake emits `message.dispatched` for the row before
  `threads.send` resolves `queued`; the obligation ends `done`, not stranded.
- **Cancelled during downtime** `[D-9]`: queue, stop the service, cancel the row, restart:
  with the row read available the wake is `undeliverable` and never resent; without it and
  with no marker it is `uncertain`, listed, and not resent until mk picks Resend.
- Periodic reconciliation moves a still-queued row to `done` once the fake dispatches it
  without emitting an event.
- **Override interleavings, live** `[D-3]` `[G-2]`: (a) `threads.send` blocks, override
  lands, the send succeeds: W's attempt is `delivered`, W is `done` with `voided_at`, and
  one void notice is sent; (b) W is queued, override lands: W's queued attempt is removed
  where the SDK allows and W is never sent again; one notice either way; (c) the asker
  already acted (W `done`): one notice, and a restart sends no second one; (d) a crash just
  before and just after the removal call: after restart the removal is re-read by handle
  and still one notice. No order between W, the notice and the replacement is asserted.
- Overlapping nudges produce serialized drains and no double send within one attempt. A late
  failure after `done` changes nothing. Resend on a voided row is refused.
- `configure` `[D-17]`: two threads in one project each get the project lines and only their
  own lines; a thread starting before any refresh of its own cache still gets its project's
  existing rulings; the text stays within 4096 UTF-16 units with non-BMP labels; an
  overridden ruling disappears from the project lines after the override without waiting
  30 s; nothing is returned only when both caches are empty.
- CLI: `ask` outside a thread without `asker: mycroft` is refused; inside one uses
  `ctx.threadId`; a request `thread` differing from `ctx.threadId` exits 2 `[D-16]`; `rule`
  from a non-vizier thread is refused; `note` with an unknown cited id is refused; `get`
  returns a mention's result; exit codes 2, 3, 5.
- Catch-up ranking, per-project collapse, pinned delegated rulings, and `markAllSeen`
  `[D-12]`: with 30 failures pushing two delegated rulings offscreen, a snapshot of the
  visible items leaves both rulings pinned and unseen; a ruling inserted after the snapshot
  stays unseen; ids not in the snapshot are never marked.

Commit: `bb-plugin-autarch: wakes, feed injection, bb home CLI and catch-up`.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm test -- __tests__/wakes.test.ts __tests__/configure.test.ts __tests__/cli.test.ts __tests__/catchup.test.ts __tests__/delegation.test.ts`
  expect: exit 0
</verify>

### Task 1.8: the Home tab — map placeholder, panel stack, vizier `[V1]` `[V11]`

Depends on Task 1.7.

**Files:** create `app.tsx`, `ui/stack.tsx`, `ui/asks.tsx`, `ui/catchup.tsx`,
`ui/vizier.tsx`, `ui/settings.tsx`, `ui/map-placeholder.tsx`; test `__tests__/ui.test.tsx`.

**Layout** (from the prototype):
- Home is a **full-page tab**: a `navPanel` that owns its whole route (`app-contract.ts:70`),
  with an `experimental_sidebarAccessory` badge (owed count, `!` when not ready or when an
  unowned machine blocker exists).
- **Left: the map.** In v1 a placeholder with the prototype's pace-layer × ecosystem frame and
  lens tabs (attention, allocation, dependencies, neglect), labelled "placeholder". The real
  map comes later, via Lattice. The prototype says it "arrives with build steps 3–4 (Lattice,
  then the map)"; this plan reads that as build-order steps 3 and 4 of `[D7]`. **mk confirmed
  on 2026-09-29 (G-14): Lattice first, then the map.**
- **Right: sliding panels**, Paradox/Matuschak style. Each panel is 1/3 of the width (`[`
  and `]` switch to 1/4 or 1/2). The newest panel is full; older ones collapse to 34 px
  spines. Esc closes the top panel.
- The base panel is **Asks**, in priority order: stalled (undeliverable wakes, unowned or
  stalled machine blockers, obligation failures), then `decide` asks, then the runbook, then
  waiting. Each decision shows the question, a link to the asking thread, mentions ("also
  mentioned in N threads"), and each option with label, kind, a reversible mark, the full
  instruction with "sent to <thread> as written; the agent acts on it under its own
  permissions", and the recommendation.
- Opening an item pushes a panel: the decision, the asking thread (`ThreadChat`,
  `variant: "compact"`), a ruling file, or catch-up.
- **The vizier panel** embeds `ThreadChat` for `vizierThreadId` (`variant: "full"`,
  `permissionPolicy: "inherit"`), per the spike. It opens from a pinned spine.
- **Catch-up panel** (Task 1.7 data), with Override on delegated rulings and "mark all seen".
- **Settings panel** ("⚙ messages"): routing per ask kind (e.g. fyi to the log), the
  `machineOwners` map, `vizierThreadId` and the delegation policy (`delegation.projects`,
  `delegation.dailyCap`), each showing the rule that applies. Delegation changes go through
  `setDelegation` and show "delegation suspended until you see this change" (P-10).
- Keyboard: `j`/`k`, Enter, Esc, `1`–`4` lenses, `[` `]` width.
- Picking sends `{option_id, revision, pick_id}` from what was rendered; `pick_id` is made
  once per click and reused on retry; a 409 re-reads.
- **Read marker (OQ4 resolved):** per account, in the plugin database, which every bb client
  of that server shares. The account key is the RPC caller's bb account id if the RPC context
  exposes one, else `local` (one account per bb server). An item is marked seen only when it
  has been expanded in the viewport for 1 s while the document is visible and focused;
  timers pause when the tab is hidden `[D-12]`. "Mark all seen" sends the rendered visible
  snapshot (Task 1.7). The overlay (Task 1.12) counts only for decision items it rendered;
  count-strip numbers never mark anything.

**Steps:**
0. Check the installed SDK for a run-completion event and record it in the commit body.
   Unit tests render the vizier panel against the SDK stub only; whether the real host
   `ThreadChat` renders and sends is verified in the real-bb scenario `vizier-chat`
   (Task 1.11) `[E-12]`.
1. Write `__tests__/ui.test.tsx`: Asks ordering; the reversible mark; instruction shown
   exactly; pick sends the rendered revision and reuses `pick_id` on retry; the stack
   collapses older panels to spines and keeps each at 1/3 by default; the seen marker
   advances only for items rendered for 1 s; a delegated ruling stays pinned until seen;
   with the document hidden no item is marked; "mark all seen" sends only the ids of
   expanded, visible items `[D-12]`.
2. Implement until they pass; `npm run typecheck`.
3. Commit: `bb-plugin-autarch: Home tab with asks, stack, catch-up and the vizier`.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm run typecheck && npm test -- __tests__/ui.test.tsx`
  expect: exit 0
</verify>

### Task 1.9: Go filers — `homeask`, `autarch decide`, MCP, Mycroft `[A-13]` `[B-15]` `[C-10]` `[C-13]`

Depends on Tasks 1.1 and 1.7.

**Files:**
- Create: `internal/homeask/filer.go`, `cmd/autarch/decide.go`, `cmd/autarch/exit.go`,
  `cmd/autarch/version.go` (`autarch version --json`: `vcs.revision` and `vcs.modified` from
  `debug.ReadBuildInfo`, and the sha256 of `os.Executable()`, the helper `serve`'s
  `/health` also reports; used by Task 1.10 `[F-7]` `[G-7]`).
- Modify: `cmd/autarch/main.go` (`root.AddCommand(decideCmd())`; the `os.Exit(1)` at `:91`
  becomes `os.Exit(exitCode(err))`) `[B-14]`.
- Modify: `pkg/mcp/server.go` (add `autarch_file_decision`, write scope checked as at `:317`).
- Modify: `internal/mycroft/escalate/escalate.go:55-120`,
  `internal/mycroft/scheduler/orchestrator.go` (`suggest()` at `:97-110`, the T2
  out-of-allowlist branch at `:123-130`, and the ready-bead filter),
  `cmd/mycroft/main.go:98`, `internal/tui/views/mycroft.go`.
- Test: `internal/homeask/filer_test.go`, `cmd/autarch/decide_test.go`,
  `internal/mycroft/escalate/escalate_test.go`,
  `internal/mycroft/scheduler/escalate_wiring_test.go`,
  `internal/mycroft/scheduler/exclude_test.go`, `cmd/mycroft/wiring_test.go`.
- The manifest (`.exec.yaml`) must list every file above for this task `[C-10]`.

**`ExecFiler`:**
- Runs `bb home ask --request-stdin` with a 10 s timeout, the request fixed with
  `request_id = Identity(ask)` when absent.
- **Thread across the proxy** `[D-16]`: the recipient thread is the request's `thread`. The
  child's environment sets `BB_THREAD_ID` to it (the CLI proxy derives `ctx.threadId` only
  from that variable), and unsets it for `asker: mycroft`. So the plugin sees the same
  thread in the request and the caller context.
- Exit 0 → the id. Exit 2, 3, 5 → typed errors.
- `bb` missing or the bb server refusing the connection → `ErrHomeDown` (nothing filed).
- A timeout or a lost connection after the request was written → **recovery read**:
  `bb home get --request-id <id> --json`, which returns `{result, decision_id, identity,
  thread, project}` `[E-6]`. Found with `identity == Identity(ask)` and the same thread and
  project → success, returned at once `[C-13]`. Found with a different identity or scope →
  the conflict error (exit 2), never success. Not found or unreadable → `ErrOutcomeUnknown`.

**`autarch decide`:**
- `file` reads the ask JSON on stdin; thread from `--thread` or `$BB_THREAD_ID` (required
  unless `--asker mycroft`); `--thread X` with a different non-empty `$BB_THREAD_ID` exits 2
  `thread flag conflicts with BB_THREAD_ID` `[D-16]`; `project_root` from `--project-root`
  (default: git root). Exit 2 validation, 3 Home down or not filed, 4 outcome unknown (re-run
  the same command), 5 already ruled.
- `list` and `stats` run `bb home list|stats --json`.
- There is **no pick in the Go CLI** `[D14]`.

**Mycroft:**
- `DecisionQueue` gets a `homeask.Filer`. `Add(p)` files asker `mycroft`, project from the
  bead's `project:<name>` label (BeadView has no project field; no label → `estate`), request
  id `mycroft:<project>:<bead>:<agent>`, question `Mycroft suggests <agent> on <bead>:
  <title>. Should it?`, two ruling-only, non-reversible options. The card notes "records your
  ruling; Mycroft does not dispatch from it until step 5". `Reasoning` goes in the question's
  detail only, never the feed.
- **Scheduling exclusion** `[C-10]`: the orchestrator drops ready beads whose type is
  `decision` or that carry any `home:*` label before `suggest()` and dispatch. This keeps the
  exclusion if the optional hub copy is ever turned on.
- `Len`, `All`, `HighestSeverity` read `bb home list --asker mycroft --json`, cached 10 s. The
  TUI badge shows `?` when Home is down.

**Test cases** (subprocess tests use a fake `bb` script on PATH):
- `ExecFiler`: a fake that commits and then hangs → recovery read finds it → success, exit 0
  from `autarch decide file` on the first run `[C-13]`. A fake that hangs and whose `get`
  also hangs → exit 4; the retry returns the same id. A conflicting request (same id,
  different ask) whose response is lost → the recovery read returns the other identity →
  exit 2, not 0 `[E-6]`.
- Exit 3 with `bb` absent; exit 2 and 5 passed through.
- Thread `[D-16]`: `--thread X` with no `BB_THREAD_ID` → the fake `bb` sees
  `BB_THREAD_ID=X` and `thread: X`; with `BB_THREAD_ID=Y` → exit 2, nothing run; Mycroft
  filings run with `BB_THREAD_ID` unset. The real proxy case is scenario `ask-cli-proxy`
  (Task 1.11).
- `autarch version --json` reports the build's revision and the sha256 of its own
  executable `[F-7]`.
- MCP `tools/list` includes `autarch_file_decision`; a call without the write scope is refused.
- Mycroft: `Add` makes one filing with the right request id, project from the label, and two
  ruling-only options; a second cycle files nothing new; the production `runCmd` path sets
  the queue; a ready bead of type `decision` or labelled `home:decision` is never suggested
  or dispatched; `Len` returns the last known count and `Stale()` when Home is down.

Commit: `homeask: Go filers for autarch decide, MCP and Mycroft`.

<verify>
- run: `go test -race ./internal/homeask/ ./cmd/autarch/ -run 'Filer|Decide|Version'`
  expect: exit 0
- run: `go test -race ./pkg/mcp/ ./internal/mycroft/... ./cmd/mycroft/ ./internal/tui/views/`
  expect: exit 0
</verify>

### Task 1.10: the scenario harness `[A-18]` `[B-18]` `[C-11]` `[C-13]`

Depends on Tasks 1.2–1.9.

**Files:** create `e2e/harness.ts`, `e2e/scenarios/*.ts`, `scripts/check-e2e.mjs`,
`scripts/build-identity.mjs`, `__tests__/check-e2e.test.ts`, `__tests__/build-identity.test.ts`, `__tests__/fixtures/e2e/{accept,reject}/*.jsonl`,
`e2e/README.md`.

**Design:**
- `npm run e2e -- --mode fake --run-id <id> --out <path>` runs the plugin's server code over a real
  better-sqlite3 file with the fake SDK, real ruling files in a scratch git project, a stub
  `serve` for `/api/projects`, and child processes for crash scenarios. The Go filer
  scenarios build `autarch` and point it at a fake `bb` that forwards to the harness.
- Faults: `kill-after=<statement>` (SIGKILL the store's child at a hook),
  `drop-response-after-commit`, `hang-response` (the filing and the recovery read both hang),
  `deny-write=<path>`, `send-queued`, `archive-thread`.
- `npm run e2e` requires `--run-id <id>`, generated by the verifier before the run (never by
  the harness), and `--out <path>` **outside the git worktree**; a path inside it is refused,
  so evidence never dirties or changes the commit it verifies `[D-15]`.
- **Install mode** `[G-7]`: Aleph's path install loads the manifest's source entry and
  builds the app bundle from source (`plugin-runtime.ts:1045`), so the artifact under test
  is a source tree, not a `bb plugin build` bundle. `scripts/build-identity.mjs --out
  <file>` exports HEAD with `git archive` into a scratch directory, writes `identity.json`
  there with `source_sha256` (sha256 over the sorted paths and contents of the export,
  excluding `identity.json`, `dist/` and `node_modules/`), runs `npm ci`, and builds
  `autarch` from HEAD. The build file records `{commit, plugin_dir, source_sha256,
  autarch_path, autarch_sha256}`.
- **Loaded identity** `[F-7]` `[G-7]`: in real-bb mode, given `--build <file> --install`,
  the harness first installs `plugin_dir` by path (`bb plugin install <dir>`) and reloads
  it; starts its **own** `autarch serve` from `autarch_path` on a free loopback port with a
  scratch token file and `--project-dir` set to one scratch discovery parent whose children
  are the scratch projects (Bigend discovery lists a root's children, not the root) `[I-4]`,
  and sets the plugin's `serveAddr`, `serveTokenFile`, `serveProjectDirs` and `AUTARCH_BIN` to
  match, so no pre-existing daemon or default token serves the run `[H-6]`; then, before any
  scenario, reads (a) the plugin's `health` RPC: `source_sha256` recomputed at load over its root,
  the connected `serve`'s `build.exe_sha256`, and the authenticated project roots, which
  must include each scratch project with the (dev, ino) the harness stats; (b) the Home tab
  opened with Playwright, whose root element carries `data-home-source` from the
  `identity.json` the app bundle was built with; (c) `autarch version --json` of
  `autarch_path`. Each must equal the build file (the serve hash equals `autarch_sha256`);
  any mismatch aborts the run with no scenario lines. Every line records
  `loaded: {plugin_source, app_source, serve_sha256, autarch_sha256}`.
- The output has one JSON object per scenario:
  `{scenario, mode, run_id, commit, tree, dirty, pass, loaded, evidence}`. `commit` is
  `git rev-parse HEAD` (the **product commit**), `tree` is `git rev-parse HEAD^{tree}`, and
  `dirty` is whether `git status --porcelain` is non-empty.
- `evidence` has a **typed schema per scenario** `[D-15]` (in `scripts/check-e2e.mjs`), for
  example `answer-instruction`: `decision` and `pick_id` matching their id formats,
  `vizier-chat` (real-bb only): `threads`, `message_id` and a 64-hex `screenshot_sha256`,
  `wake_state` equal to `"done"`, `wake_count` equal to 1, `ruling_sha256` 64 hex, and
  `feed_line` equal to the quoted label format; `two-writers`: `picks` equal to 1 and
  `conflicts` equal to 1. The harness fills them from the store and the disk, not from its
  own expectations.
- `scripts/check-e2e.mjs <file> --mode <m> --run-id <id> --product-commit <sha>
  --scenarios <list> [--build <file>]` accepts the file only if (with `--build`, which
  real-bb acceptance always passes, every `loaded` field must equal the build file and
  its `commit` the product commit `[F-7]`): every line parses; the scenario set is
  exactly the list with no duplicates; every `pass` is true; every `run_id` equals the
  expected one; `commit` equals `--product-commit` and `tree` equals that commit's tree;
  `dirty` is false; `mode` matches; every evidence object validates against its scenario's
  schema.
- `__tests__/check-e2e.test.ts` runs the checker on the fixtures: one accepted file, and
  rejected files for empty, partial, duplicated, stale commit, stale tree, dirty, mixed
  `run_id`, wrong mode, missing evidence, **a complete passing run at the same commit with a
  different run id** (stale run), and **malformed evidence** (wrong types, `wake_state:
  "pending"`, a 63-character hash, `wake_count: 2`) `[D-15]`, and complete real-bb runs at
  HEAD with an older `loaded.plugin_source`, an older `loaded.app_source` (stale frontend)
  or an older `loaded.serve_sha256` (stale daemon) `[F-7]` `[G-7]`.
  `__tests__/build-identity.test.ts` runs the harness preflight against a fake plugin and
  app but the real `autarch serve` built from HEAD `[I-4]`:
  it aborts before any scenario when the plugin's `source_sha256`, the app's
  `data-home-source` or the connected serve's hash is older than the build file; with an
  older daemon already answering on `127.0.0.1:8110` and a different token in the default
  token file, the plugin is pointed at the harness's own `serve` and scratch token, and the
  preflight passes only on that daemon's identity and an authenticated resolution of the
  scratch project; a 401 or a missing scratch root aborts it `[H-6]`.

**Scenarios** (fake mode; names are exact):
1. `answer-instruction`: file with an instruction option, pick it; ruling file, one wake with
   the exact instruction, `sent` → done, feed shows the label only.
2. `pick-retry`: `drop-response-after-commit` on pick; retry with the same `pick_id`; one
   pick, one wake.
3. `crash-after-pick`: `kill-after=pick-commit`; restart; within 35 s the ruling file exists
   and exactly one wake is pending.
4. `two-writers`: two store connections race picks with different ids; one pick, one 409.
5. `file-retry`: `drop-response-after-commit` on `autarch decide file`; the recovery read
   finds it; exit 0 on the first run; one decision `[C-13]`.
6. `file-unknown`: `hang-response`; exit 4; the re-run returns the same id; one decision.
7. `not-ready-at-start`: the database file is locked by another process at start; the
   factory returns and Home shows not-ready; `ask` exits 3; release the lock; within 35 s,
   without a plugin reload, filing and picking work `[D-10]`.
8. `supersede`: file A, B superseding A, pick A → 409; a second replacement → 409; B owed.
9. `ruling-file-blocked`: `deny-write` on project P; a pick in P stays pending with an error;
   a pick in Q completes `[C-7]`.
10. `queued-then-archived`: `send-queued`, then `archive-thread`; the wake is undeliverable
    and listed `[C-8]`.
11. `delegated-override`: the vizier rules on a reversible option in an enabled project; the
    asker is woken; mk overrides; one void notice to the asker; the project feed drops the
    old label; mk picks the new decision; its wake and feed line say it supersedes the
    vizier's ruling on the old decision (P-10).

**Steps:** write the harness and checker, run in fake mode, fix what it finds, commit
`bb-plugin-autarch: scenario harness and evidence checker`.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm test -- __tests__/check-e2e.test.ts __tests__/build-identity.test.ts`
  expect: exit 0
- run: `cd integrations/bb-plugin-autarch && rid=$(node -e 'console.log(crypto.randomUUID())') && out=$(mktemp -d)/e2e.jsonl && npm run e2e -- --mode fake --run-id "$rid" --out "$out" && node scripts/check-e2e.mjs "$out" --mode fake --run-id "$rid" --product-commit "$(git rev-parse HEAD)" --scenarios answer-instruction,crash-after-pick,delegated-override,file-retry,file-unknown,not-ready-at-start,pick-retry,queued-then-archived,ruling-file-blocked,supersede,two-writers`
  expect: exit 0
</verify>

### Task 1.11: end-to-end on a real bb, and follow-ups

No new product code. Rev 3 ran this against the hub tracker; decisions no longer live there,
so the real system under test is bb itself.

**Precondition** `[D-14]`: `bb plugin dev` only works on an **installed** plugin
(`apps/cli/src/commands/plugin.ts:1483` looks the plugin up in the server's list). Steps 1–3
therefore run only with mk's permission for an isolated install (a separate bb server with
its own scratch data directory and port, not mk's), or after G-3 on mk's server. Without
either, they are recorded as not run and criterion 14 stays unmet; that is not completed
verification.

**Steps:**
1. On devhost, against the permitted isolated bb server (its address in `HOME_E2E_BB`), build
   with `scripts/build-identity.mjs`, generate
   `rid=$(node -e 'console.log(crypto.randomUUID())')`, and run
   `npm run e2e -- --mode real-bb --build <file> --install --run-id "$rid" --out ~/.autarch/home-e2e/<commit>/<rid>.jsonl`
   (the harness installs by path, reloads, starts its own `serve` and verifies the loaded
   plugin, app, daemon and `autarch` identities first `[F-7]` `[G-7]`)
   (outside the checkout `[D-15]`). Scenarios `answer-instruction`, `queued-then-archived`,
   `ask-cli-proxy` (a real `bb home ask --request-stdin` from a scratch thread;
   `autarch decide file --thread` through `ExecFiler` with `BB_THREAD_ID` absent and with
   a conflicting value `[D-16]`) and `vizier-chat` `[E-12]` (Playwright against the
   isolated server opens Home; the vizier panel renders the host `ThreadChat` for the scratch
   vizier thread; a composer send adds a message; evidence: thread id, message id and the
   screenshot's sha256) run against the real bb server, real threads and the real CLI proxy.
2. Every scratch thread the run creates is tagged with the `run_id` in its title. Cleanup
   archives exactly the threads whose ids the run recorded in `evidence.threads`, and deletes
   the scratch data directory. The cleanup record lists those ids.
3. Write `docs/research/2026-09-29-home-e2e-real-bb.json`, a manifest
   `{product_commit, run_id, evidence_path, evidence_sha256}` (`evidence_path` absolute), and
   `docs/research/2026-09-29-home-e2e-check.md`: the command, product commit, a table from
   the JSONL, and any failure and fix. Both are **historical records, not acceptance**
   `[E-11]`: criterion 14 runs the harness afresh with a run id it generates itself and never
   reads them.
4. File child beads of `mk-okek` (none blocks this plan), and record their ids in the check
   document:
   - **Clavain Stop hook and filing helper** `[D15]` `[D25]` (G-12): the turn-end ask block
     and the wrapper that calls `bb home ask`.
   - **Signals clients reach the consolidated broker** `[B-19]`.
   - **bd upstream: `create --id` overwrites an existing bead** (C-1 evidence).
   - **Optional hub copy**: a one-way, never-read export of decisions to hub beads with
     `home:*` labels derived from the store.
   - **Approvals in Aleph core** (ruled at G-11, built after G-16), naming
     mk-qap9 rev 7 and mk-7l2o (release v8 §5.1) as prior art.
   - **Host-attested caller for plugin CLI and RPC** (G-15), for the Aleph coordinator.
   - **Core approval authentication and audit contract** (G-16).
5. If the precondition is not met, steps 1–3 move into G-3/G-6; criterion 14 stays unmet
   until they run.

<verify>
- run: the criterion 14 check.
  expect: exit 0
</verify>

### Task 1.12: the summoned overlay `[V1]` `[V3]` `[V12]` — gated on G-10

Not started until G-10 clears. It is a separate task so it never blocks Tasks 1.1–1.11.

**Core dependency (OQ2):** Aleph desktop has no `globalShortcut` and no `alwaysOnTop`, and
plugins cannot reach Electron's main process. The overlay needs a core desktop change: a
global shortcut, a frameless always-on-top `BrowserWindow`, and a preload bridge that loads a
plugin panel route. It belongs to the Aleph coordinator (thr_39wwcmwi84). The web build gets
an in-page overlay only.

**Plugin side** (after G-10): an `overlay` route showing owed decisions (pick in place), one
input line to the vizier (`ThreadChat`, `variant: "compact"`), a count strip (undeliverable
wakes, delegated rulings not yet seen) and the vizier's last reply. No map, no history. Only
rendered decision items mark seen. Tests: `__tests__/overlay.test.tsx` for those rules.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm test -- __tests__/overlay.test.tsx`
  expect: exit 0
</verify>

### Task 1.13: approvals, interim and non-authorizing (P-12) — gated on G-11 `[D-5]` `[D-6]`

**Foundational: cross-lab review item 4.** mk ruled at G-11 (2026-09-29) that approvals live
in Aleph core. The authoritative record and check API are built there under the follow-up
bead, after G-16 and G-15; Home then files options with an `approval` spec and calls core's
API. Core is not ready, so this task builds only the interim below, as a stopgap to be
replaced by that API. The interim is **non-authorizing**: nothing may consume it, and no path
promotes, imports or converts it into an authoritative approval.

**Record** (`approvals` table, and `approval_events`, append-only):
`approval_id`, `decision_id`, `option_id`, `pick_id`, `account`, `kind` (`merge`, `deploy`,
`release`), `target` (`owner/repo#N`, a site, a unit), `identity` (head SHA, image digest,
tag SHA), `minted_at`, `expires_at` (default 24 h, at most 7 days), `revoked_at`, and
`authorizing` (always `false` in the interim). Single-use only; there is no `single_use`
field.

**Rules:**
- An option may carry `approval: {kind, target, identity, ttl?}` at filing. Such an option is
  never reversible and never delegable.
- Before the pick, the option renders the exact tuple (kind, target, pinned SHA or digest) and
  the expiry, so mk sees what the pick would approve.
- Only a pick with `by: "mk"` mints, in the pick's transaction. `by: vizier` never mints. The
  `by` is advisory (G-15), which is one reason the record cannot authorize.
- `bb home approval check --kind --target --identity` reads (recorded, expired, revoked,
  absent) and always prints `authorizing: false`. There is **no consume command**.
- Home can revoke a record. Every mint, check and revoke appends an event.
- **Enforcement:** the SylvesteOps hook (reported to scan transcripts for
  `APPROVED-MERGE owner/repo#N sha`; its source was not read for this revision) stays the
  enforcing check.
- **What G-16 must provide before anything consumes an approval** `[D-6]`: authenticated,
  account-scoped consume before dispatch, matching the approval id and the exact tuple; an
  operation-side precondition on the pinned SHA or digest, so a moved head fails; expiry
  checked against trusted time; a defined consume/revoke race; retry of an uncertain consume
  outcome without double use; and consume refused when its audit write fails, in one
  transaction. It also preserves release v8 §5.1 (presence-key signature, transitions bound
  to prior and new semantic hashes, `expires_at` and `batch_valid_until`). The racing-consume
  test moves to that contract; it is not dropped.
- **No approval tokens, unconditionally** `[E-9]`: nothing the interim writes (wakes, ruling
  files, Home text, `check` output) carries an `APPROVED-*` token, so the existing hook can
  never accept an interim record. Rev 4's G-11b question is withdrawn. Any authorizing path
  needs G-16 and a revision of this approval boundary first.

**Test cases:** mint only on mk's pick; none on a vizier pick; expired and revoked read as
such; an approval option marked reversible is refused at filing; the events table records
each step; no consume verb or RPC exists; `check` output always says `authorizing: false`;
a minted record's wake, ruling file and `check` output contain no `APPROVED-` string;
the tuple and expiry render before the pick; a `single_use` field at filing is refused.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm test -- __tests__/approvals.test.ts`
  expect: exit 0
</verify>

---

## Success measure (OQ6, resolved by the baseline)

Baseline (thr_dcuzkvjb39, 2026-09-29 11:58–15:16): 40 turns in 3h18m, **0 of 40 answered
from a rail**, median gap 1.6 min, longest 15.1 min. The gap is a poor proxy for time-to-pick
and is one sample, so it is used only for the first measure.
- **Rail share:** picks with `surface` `home` or `overlay` rise from 0.
- **Time-to-pick:** median and p90 of `picked_at − filed_at` for mk's picks, from the pick
  record after the build (`bb home stats`). No log reconstruction.
- **Scroll check:** after two weeks on Home, ask mk once whether they still scroll the vizier
  chat to find what is owed.
- **Failure signals:** override rate (overrides ÷ delegated rulings) and median time from a
  delegated ruling to its override. Both are zero today because delegated rulings are not
  listed. A rising rate means delegation is too wide; mk decides whether to narrow it.

## Explicitly out of scope

- Command picks, the continuation runner and revert commands (cut 2026-09-26).
- Signing ruling files (step 3, Lattice).
- The brief continuation `[D22]`; a "QA this" kind (open question for mk).
- Badges beyond the Home entry (step 2).
- Lattice (step 3), the real map (step 4, WI-5), Mycroft's proposals and dispatch (step 5), and
  the companion (step 6).
- Automatic closing of superseded decisions outside the store, and the hub copy (follow-up).
- The Clavain `UserPromptSubmit` feed hook (P-7); the Clavain Stop hook and filing helper
  (G-12, Clavain-owned).
- Standing rulings (after the trial, Clavain-owned; `by: rule:<id>` reserved).
- Overriding mk's own picks.
- Successor-thread resolution (G-9).
- `bb.ui.requestInput` in-thread cards.
- A standalone vizier app, and live-work watching outside the map `[V6]`.
- Removing the standalone servers; mounting Pollard.
- A host-attested caller (G-15, Aleph core) and trusted, consumable approvals (G-16, Aleph
  core). Home builds neither; it records advisory attribution and non-authorizing intent.

## Execution preconditions and mk gates

These are **not agent-completable**. Each is a checklist item for mk; none is a DONE WHEN.

- **G-0:** mk approved `mk-okek` (2026-09-26). The round-11 review-astra pass returned PASS
  with no new findings (rounds 4 to 10 returned D-1 to D-17 through J-1, folded in here).
  Checked 2026-09-29: the immutable GitHub repository ID is 1140086114
  (`mistakeknot/Autarch`, `gh api repos/mistakeknot/autarch --jq .id`), and `devhost-ci status
  --repo mistakeknot/Autarch --json` on devhost shows it registered (campaign `mk-ag2s`,
  disposition `pending-inventory`, inventory `requires-workflow-review`, evidence
  `ci/fleet/evidence/2026-09-10-autarch-visit-registration.json`). Migration is **not
  complete**: the migration task is `mk-ag2s.18`, still in progress. The lowercase name
  `mistakeknot/autarch` returns "repository not registered"; the registered name is
  `mistakeknot/Autarch`. `gh` reports the repository as public (`private: false`), against the
  private-by-default rule, which is for mk to review. The sidecars (`.criteria.md`, `.criteria.md.seal`, `.exec.yaml`) were
  re-extracted, resealed and regenerated for revision 4 (19 tasks) on 2026-09-29.
- **G-1 (ruled 2026-09-29 by the vizier):** no Uqbar repo yet. Estate-wide decisions stay
  refused until a real one exists; creating the private repo and setting `AUTARCH_UQBAR_DIR`
  is then mk's step.
- **G-3:** install the plugin: `bb plugin install path:integrations/bb-plugin-autarch`. Removing
  it later leaves `data.db`; mk deletes it by hand to reset. Task 1.11 may instead run on an
  isolated bb server (its own data directory and port) if mk permits that install `[D-14]`.
- **G-4 (ruled 2026-09-29 by the vizier):** one republish of `autarch-plugin` (`plugin.json`
  changes in Task 0.5) after the Home build is accepted. The publish itself still needs mk's
  approval then.
- **G-5 (ruled 2026-09-26):** P-7 and P-5 departures accepted.
- **G-6 (ruled 2026-09-29 by the vizier):** start after Task 1.11 passes on the isolated
  server. The trial: start by walking autarch-07 on the installed Home tab with a real decision,
  including one delegated ruling and one override, and revise autarch-07 to match.
- **G-9 (ruled 2026-09-26):** undeliverable in v1; successor resolution deferred.
- **G-10 (ruled 2026-09-29 by mk, via the vizier):** approved, kept small: the Aleph core
  desktop change for the overlay (global shortcut, frameless always-on-top window, and the
  plugin-panel preload bridge only), owned by thr_39wwcmwi84. Unblocks Task 1.12, which is
  built last. Normal review applies, and its release needs mk.
- **G-11 (ruled 2026-09-29): approvals live in Aleph core.** The interim in Home is
  non-authorizing and nothing consumes an approval before G-16. G-11b is withdrawn: the interim never emits `APPROVED-*` tokens `[E-9]`. Gates
  Task 1.13 now builds the interim only.
- **G-12:** the Clavain Stop hook that writes structured turn endings (decision 25). Home works
  without it, but the trial of decision 25 does not start until it ships.
- **G-13 (OQ5, ruled 2026-09-29 by the vizier):** A first; this revision comes before the
  interim pop-out window and fixed-format catch-up, and B is decided after Home ships.
- **G-14 (ruled 2026-09-29):** "build steps 3 and 4" means Lattice, then the map.
- **G-15 (D-1):** an Aleph core host-attested caller for plugin CLI and RPC. Today the CLI
  context takes the thread from `BB_THREAD_ID` (`apps/cli/src/lib/context-env.ts:48`) and the
  plugin RPC route accepts a client-supplied `threadId`
  (`apps/server/src/routes/plugins.ts:438`), so `by: vizier` and `by: mk` are forgeable. mk
  asks the Aleph coordinator (thr_39wwcmwi84). Until it ships, attribution is advisory and
  delegation stays bounded, listed and overridable (P-10).
  **Resolution path (Aleph coordinator, 2026-09-29; nothing is built):** option A, a
  per-thread capability token minted by the host at spawn (hash stored, mapped to
  `{threadId, role: agent}`, revoked on thread end or respawn), injected as
  `BB_THREAD_CREDENTIAL` and sent by the CLI as a distinct header. The server resolves the
  caller from the token alone; `body.threadId` and `?threadId=` are hints, and a hint that
  conflicts with the token is rejected. Human means the authenticated UI session channel with
  no thread credential. Rejected: SO_PEERCRED (Linux-only, breaks remote and tunnels; possible
  later hardening) and per-thread request signing (same trust root, more machinery).
  Caller API: a host-populated, read-only `caller {threadId|null, role human|agent|unattested,
  via ui-session|thread-credential|none}` on CLI and RPC contexts. The legacy `threadId`
  stays but is deprecated for authorization, and plugins can never construct or override
  `caller`. Old CLIs, scripts and mk's terminal outside a thread are `unattested`; counting
  mk's terminal as human needs a separate proof, such as an mk-held token, and is a question
  for mk. The token does not reuse `x-bb-plugin-token` and never appears in logs,
  diagnostics, `bb thread show` or plugin-visible data. It changes the plugin-sdk contract
  and the CLI-server wire, so it stays on its own branch and ships after the Aleph public
  release, not in 0.5.x (protocol 219). Required tests: a spoofed `BB_THREAD_ID` with another
  thread's token; a forged `body.threadId` or `?threadId=`; no token gives `unattested`; a
  revoked token; a UI-session request carrying a thread credential does not become human.
  Estimate: about 3-4 executor-days plus 2-3 security review rounds, about 1-1.5 weeks after
  commissioning, and +~1 day if daemon or remote-host CLI forwarding needs work.
  **Residual risk:** every agent runs as uid mk, so a same-uid process can read
  `/proc/<pid>/environ` or ptrace a sibling and steal its token. Option A closes forgery
  through the old channels, not a deliberately malicious agent with host code execution;
  closing that needs per-thread OS isolation, a separate and larger decision. Commissioning
  is a later gate that goes to the vizier after the public release. Home attribution stays
  advisory until then.
  Tweaks for the commissioning brief (vizier research input, 2026-09-29, not yet ruled on):
  the token travels in a header only, never a tool argument or URL; only its hash is stored;
  it has an expiry, a thread binding and revocation; human-tier approvals accept only the
  authenticated UI session, with no CLI or environment path. Worth testing: deliver the token
  by inherited file descriptor or socket instead of `environ`, which narrows the
  `/proc/<pid>/environ` leak.
- **G-16 (D-5, D-6):** a reviewed Aleph core approval authentication and audit contract, with
  the requirements listed in Task 1.13, before any consumer trusts an approval.
  Per G-15, the `human` role (UI session, no thread credential) is the strong signal; agent
  identity is strong against spoofing, not against same-uid compromise. Commissioned after
  G-11 and G-15.
  Ideas for the contract (vizier research input; ideas only, no code copied: the source
  repositories carry a licence rider that grants nothing to Anthropic, so any use is
  clean-room and awaits mk's view): each approval binds a hash of the exact action, expires,
  is single-use, and the first decision wins atomically; approver equal to requester is
  rejected. Tiers: merge, deploy and release are human-only, and the vizier rules only on
  reversible choices. Configuration may add gates but never remove them. The audit is
  hash-linked and append-only, with the head hash stored where agents cannot write.
- G-2, G-7 and G-8 were superseded on 2026-09-26.

## Verification

- Whole Go tree: `go build ./cmd/... && go test -race ./...`.
- Bind and read-only: `go test -race -run 'Bind|ReadOnly' ./internal/serve/`.
- Plugin: `npm run typecheck`, `npm test` and `bb plugin build` in
  `integrations/bb-plugin-autarch`.
- Harness: fake mode (Task 1.10) and real-bb mode (Task 1.11), each run fresh at acceptance
  by its criterion, which generates the run id and checks the exact scenario set and HEAD;
  evidence is written outside the checkout. Committed reports are history, not evidence
  `[E-11]`. Real-bb acceptance first installs a clean export of HEAD by path, starts its
  own `serve`, and checks the loaded plugin source, the rendered app, the daemon and
  `autarch` against the build `[F-7]` `[G-7]`.
- Gated tasks (1.12, 1.13) are verified by their own blocks when their gates clear; they are
  not part of this plan's acceptance.
- Real acceptance: G-6 (mk). No harness or fixture replaces it.

## Acceptance Criteria

1. The whole Go tree builds and every test passes with the race detector.
   ```check
   go build ./cmd/... && go test -race ./...
   ```
2. `autarch serve` refuses a non-loopback address.
   ```check
   go test -race -run 'Bind' ./internal/serve/ ./cmd/autarch/
   ```
3. The service mounts Bigend, Gurgeh and Signals under prefixes behind the token, Host and
   Origin checks, with no query-string token. `/health` is open. Projects resolve without
   ambiguity or symlink escape and report root identity. Gurgeh's signals reach `/signals/ws`
   through one broker. `serve` has no decision route or writer and imports no decisions code; a failing
   `go list` fails the check `[D-15]`.
   ```check
   go test -race ./internal/serve/ ./internal/gurgeh/server/ && go test -race -run 'ReadOnly' ./internal/serve/ && deps=$(go list -deps ./internal/serve/) && ! printf '%s\n' "$deps" | grep -qE 'internal/(decisions|homeask)'
   ```
4. `autarch mcp` serves the existing MCP tools plus `autarch_file_decision`, and `autarch-mcp`
   still works.
   ```check
   go test -race ./cmd/autarch/ -run MCP && go test -race ./pkg/mcp/
   ```
5. Options have ids, one of three kinds, bounded instructions and a reversible mark with its
   refusals. Identity, revision and semantic key cover the scope, approval tokens are refused
   in any text, and Go and TS agree on every vector.
   ```check
   go test -race ./internal/homeask/ -run 'Validate|Revision|Identity|Semantic|Vectors' && cd integrations/bb-plugin-autarch && npm test -- __tests__/model.test.ts
   ```
6. The store files once per request id without upsert, records a pick only by conditional
   update, refuses superseded or stale picks, keeps obligations fenced by state and attempt,
   never tears a pick from its obligations under SIGKILL, and exports events append-only in
   immutable per-store segments outside the checkout, idempotent across a crash `[D-13]`.
   Requests are one registry; mentions are keyed by decision and thread; obligations are one
   outbox row per message with a snapshotted recipient, payload, unique per-recipient `op`
   and `after_id`, a fan-out inserts its whole recipient set in one transaction, each send
   is its own attempt row, a Resend permit is consumed with the attempt it creates, and
   `voided_at` suppresses sending without changing a row's delivery state `[E-2]` `[E-3]`
   `[E-8]` `[F-2]` `[F-6]` `[G-1]` `[G-2]`. Migrations are staged (expand-only, `min_reader_version`), a
   failed activation after a migration leaves the old instance working, and a locked database
   initializes degraded and recovers without a reload `[D-7]` `[D-10]`.
   ```check
   cd integrations/bb-plugin-autarch && npm test -- __tests__/store.test.ts __tests__/store-crash.test.ts __tests__/export.test.ts __tests__/migrations.test.ts
   ```
7. Ruling files carry the ratification block and the Home fields, are idempotent, and are
   written only under the root identity saved at filing, refusing symlinks, a replaced root and
   a retargeted root.
   ```check
   cd integrations/bb-plugin-autarch && npm test -- __tests__/ruling.test.ts
   ```
8. Filing is idempotent and scoped; a pick is recorded once per decision; a reused pick id with
   other parameters is refused; a replacement blocks its predecessor's pick; one obligation's
   failure never blocks another decision; dismissal survives restart; mentions attach once,
   only by `mention_of` or semantic equality over a structured `subject` and every answer
   semantic (`ask_key` alone marks "possibly related"; unrelated asks never merge); a
   replacement never mentions, and `supersedes` with `mention_of` is refused; a retried filing
   returns its canonical identity while serve is down `[E-1]` `[E-7]` `[E-10]`; every
   supersede interleaving resolves in one transaction with an explicit outcome
   (`already-ruled`, `withdrawn`, `already-superseded`, `superseded`, `stale`) `[D-4]` `[D-8]`.
   ```check
   cd integrations/bb-plugin-autarch && npm test -- __tests__/service.test.ts
   ```
9. The feed carries only the quoted, clipped label from pick records, per thread, with own
   answers reserved within 4096 UTF-16 units; only effective rulings count, and a superseded
   ruling shows as void; a thread new to a project gets the shared project feed, and the
   caches are invalidated on override `[D-4]` `[D-17]`; a replacement ruling appears at once
   with `supersedes <old id>`; undeliverable wakes are listed regardless of age;
   stats report rail share, time-to-pick, override rate and ruling-to-override time.
   ```check
   cd integrations/bb-plugin-autarch && npm test -- __tests__/feed.test.ts __tests__/service.test.ts
   ```
10. Wakes complete only on `sent`, a dispatched queued row, or exactly one accepted event
    matching recipient, framed marker and snapshotted payload (a quoted marker, an edited
    unbound row or several matches do not count), become undeliverable on archive, delete or queue
    removal, survive restart while queued, drain serially, and never regress from done.
    Each obligation is its own message, never batched, and each send is its own attempt.
    Only a proven pre-acceptance rejection is retried automatically; any other failure,
    with or without a crash, including a crash before the row id is stored, is reconciled
    and otherwise `uncertain` with Resend and Dismiss; one Resend click makes exactly one new
    attempt through the real drain, across restarts and replays `[I-1]`, while earlier ones
    keep being reconciled. A voided wake is never sent again, and an override sends one void
    notice whatever the wake's state `[D-3]` `[D-9]` `[E-4]` `[E-5]` `[F-1]` `[F-2]` `[F-4]`
    `[G-1]` `[G-2]`. The
    `bb home` CLI and `autarch decide` return exit codes 2, 3, 4 and 5 as real processes, a
    thread conflict between `--thread` and `BB_THREAD_ID` exits 2 `[D-16]`, a found recovery
    read with the same identity and scope exits 0, and one with another identity exits 2
    `[E-6]`.
    ```check
    cd integrations/bb-plugin-autarch && npm test -- __tests__/wakes.test.ts __tests__/configure.test.ts __tests__/cli.test.ts __tests__/delegation.test.ts && cd ../.. && go test -race ./internal/homeask/ ./cmd/autarch/ -run 'Filer|Decide|Version'
    ```
11. Mycroft's suggestions and out-of-allowlist dispatches file ruling-only decisions once per
    suggestion through the production constructor, Mycroft never schedules a decision bead,
    and `DecisionQueue` keeps no private list.
    ```check
    go test -race ./internal/mycroft/... ./cmd/mycroft/ && ! grep -nE '^[[:space:]]+decisions[[:space:]]+\[\]PendingDecision' internal/mycroft/escalate/escalate.go
    ```
12. The bb plugin typechecks, all its fake-SDK tests pass, and it builds.
    ```check
    cd integrations/bb-plugin-autarch && npm run typecheck && npm test && bb plugin build
    ```
13. The fake-mode harness passes exactly its eleven scenarios in one run at this clean commit
    and tree, under an externally generated run id, with evidence that validates against each
    scenario's typed schema and is written outside the checkout; the checker rejects every
    rejection fixture, including a stale same-commit run and malformed evidence `[D-15]`.
    ```check
    cd integrations/bb-plugin-autarch && npm test -- __tests__/check-e2e.test.ts && rid=$(node -e 'console.log(crypto.randomUUID())') && out=$(mktemp -d)/e2e.jsonl && npm run e2e -- --mode fake --run-id "$rid" --out "$out" && node scripts/check-e2e.mjs "$out" --mode fake --run-id "$rid" --product-commit "$(git rev-parse HEAD)" --scenarios answer-instruction,crash-after-pick,delegated-override,file-retry,file-unknown,not-ready-at-start,pick-retry,queued-then-archived,ruling-file-blocked,supersede,two-writers
    ```
14. A fresh real-bb run, started by this check with a run id it generates itself, against
    the permitted isolated bb server named by `HOME_E2E_BB` (Task 1.11, or mk's server after
    G-3), exports HEAD and builds `autarch`, installs the export by path and reloads it,
    starts its own `autarch serve` from that build with a scratch token and a scratch
    discovery parent and points the plugin at both, resolves the scratch project through an
    authenticated request, verifies the loaded plugin source, the rendered app's source, the
    connected daemon and the `autarch` binary against the build, passes exactly its four
    scenarios at HEAD with clean evidence written outside the checkout, and archives every
    scratch thread it recorded. The checker rejects a stale plugin, stale frontend and stale
    daemon. The committed manifest and report are historical and not read. Without
    `HOME_E2E_BB` the check fails: unmet, not skipped `[D-14]` `[D-15]` `[E-11]` `[E-12]`
    `[F-7]` `[G-7]` `[H-6]` `[I-4]`.
    ```check
    test -n "$HOME_E2E_BB" && cd integrations/bb-plugin-autarch && npm test -- __tests__/build-identity.test.ts && rid=$(node -e 'console.log(crypto.randomUUID())') && d=$(mktemp -d) && b="$d/build.json" && e="$d/e2e.jsonl" && node scripts/build-identity.mjs --out "$b" && npm run e2e -- --mode real-bb --build "$b" --install --run-id "$rid" --out "$e" && node scripts/check-e2e.mjs "$e" --mode real-bb --run-id "$rid" --product-commit "$(git rev-parse HEAD)" --build "$b" --scenarios answer-instruction,ask-cli-proxy,queued-then-archived,vizier-chat && jq -se '[.[].evidence.threads[]] as $t | ($t|length) > 0 and all(.[]; .evidence.cleanup.archived == .evidence.threads)' "$e"
    ```
15. Structured asks, delegation and the Home tab hold: runbook, machine lane and mentions;
    machine blockers progress, resolve and withdraw, each notice an outbox row that a later
    resolve or withdraw voids or follows, an owner notice asks the owner to confirm the
    blocker is open (a resolve may overtake an uncertain one), and a completion wakes every
    recipient once `[H-5]`; `rule` from a caller claiming the vizier thread, including a
    forged matching `BB_THREAD_ID`, is held to every delegation limit (reversible, listed
    project, daily cap, no approval spec), woken with the non-authorization line and pinned;
    a direct `pick` RPC from any caller is recorded `by: mk` without delegation limits,
    which the test records as the admitted attribution bypass; authenticated human/vizier
    separation is G-15; delegation settings change only through the panel, pinned, and suspend
    delegation until seen; an override voids the old wake, sends one void notice to the
    asker in every wake state, keeps a delivered wake `done`, and makes the replacement and
    its successors non-delegable with their mentions kept, the void notice names the
    replacement and never overrides an answer already received, and every later answer
    names the overridden vizier ruling `[H-3]` `[H-4]`; catch-up ranking and pinning;
    the panel stack, `markAllSeen` over a rendered snapshot, and a read marker that needs the
    item expanded, visible and focused `[D-1]` `[D-2]` `[D-11]` `[D-12]` `[E-2]` `[E-13]`
    `[F-6]` `[F-8]` `[G-2]` `[G-4]`.
    ```check
    cd integrations/bb-plugin-autarch && npm test -- __tests__/asks.test.ts __tests__/delegation.test.ts __tests__/catchup.test.ts __tests__/ui.test.tsx
    ```
16. mk walks autarch-07 on the installed Home tab, including a delegated ruling and an
    override (G-6). This is recorded by mk, and no command can substitute for it.
