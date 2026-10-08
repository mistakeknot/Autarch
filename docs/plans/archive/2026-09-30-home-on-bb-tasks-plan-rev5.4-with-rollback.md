# Home S1, revision 5: Home on bb tasks

**Bead:** mk-okek (S1). Re-scopes mk-okek.8 and mk-okek.16.
**Replaces:** the S1 queue design in `docs/plans/2026-09-26-home-serve-and-decisions-plan.md`
(rev 4). Rev 4 is not edited and is still the reference for everything this plan marks
SURVIVES.
**Revision:** rev 5.4 (2026-10-01).
- Rev 5.1 answered the first Astra review (Reject, 12 findings).
- Rev 5.2 answered the second Astra review (Reject, 11 findings) and added a self-pass on the
  card state machine (§8.3).
- Rev 5.3 answers the third Astra review (Reject, 9 findings) and adds an enumerated
  self-pass covering triggers, ExecFiler sites and generation hashes (§8.5).
- Rev 5.4 answers the fourth Astra review (Reject, 6 findings, §8.6) and moves the rollback
  model to mk/vizier (§1.3.9, Q7); rollback-only items are marked [ROLLBACK-ONLY].
- The dispositions are in §8.

**Inputs:**
- `docs/research/2026-09-30-tasks-rpc-spike.md`
- `docs/research/2026-09-30-aleph-runner-interface.md`
- tasks@0.1.2: `~/bb-picker-switch/plugins/tasks/{shared/contract.ts,api/index.ts,cli/index.ts}`
- plugin SDK: `packages/plugin-sdk/src/backend-contract.ts`
- the committed plugin at `integrations/bb-plugin-autarch/` (a9853e2)

**mk ruling 2026-09-30:**
- The "what am I blocking" panel and Home's decision queue are built on bb tasks.
- There is one queue for new asks: tasks cards labelled `needs-mk` with a `Blocks:` line.
- The panel is a cross-project view, sorted by the derived blocks count and then by age.
  It opens beside a thread.
- Home keeps only what tasks lacks, as a layer keyed by card id.
- Beads stay the agents' tracker.

## Decision context

```json
{"reasons": ["broad-consequences", "difficult-verification"],
 "rationale": "Moves the S1 source of truth from Home's own decisions table to another plugin's cards that same-uid agents can edit; every pick, wake, ruling and delegation path changes its input, existing open asks must survive cutover, and the rollback and isolation properties can only be shown by functional tests."}
```

**Escalation.** One frontier author wrote this plan. The other-frontier reviewer (Astra) has
now rejected it four times, though the findings are narrowing (12, 11, 9, then 6). Under
the operating contract, two rejects already amount to a request for escalation, and the
decision belongs to mk or the coordinator.

**Gate.** Rev 5.4 has not been reviewed. It does not go to execution until a review passes,
or until mk rules otherwise.

---

## 1. Architecture on tasks

### 1.1 What lives where

| Concern | Owner | Notes |
|---|---|---|
| The ask as filed (title, question, options, Blocks refs, Request, root-run tuple) | **tasks card**, label `needs-mk` | **Untrusted and mutable.** Any same-uid agent can edit any card, so Home never treats the card as the ruling's content. |
| Who asked | **The earliest `kind:"agent"` comment on the card** | The tasks server sets `threadId` from the CLI context (`cli/index.ts:1856`). One selector, `askingThread(comments)`, is used everywhere: ingest, filer replay and the root-run hand-off (§1.3). `authorName` is never read: RPC comments always say "You" (`api/index.ts:828`). |
| Agents' own work | **beads** | Unchanged. |
| Immutable ask generations, picks, revision gate, ruling files, delegation, overrides, catch-up, wakes, card write-backs, interim approvals | **Home layer** (`bb.storage.database()`) | Keyed by `task.id` (ULID) and a generation number. The card key (e.g. `PROJ-2`) is display only. |
| Request → canonical card | **Home `card_requests` registry** (insert-once) | It is separate from card observations, which are not unique (§1.3.3). |
| tasks project → Home project | **Home `project_bindings`** (mk-confirmed) | Delegation scope and ruling scope are one key, the Home project name (§1.3.6). |
| Pre-v3 asks | **Home legacy lane** | Drained in place, without migrating them to cards (§1.3.7). |
| Live refresh | **Home server poller → Home realtime channel** | Proven on real bb in Task 2.12. |
| Root runs | **Aleph** | Home shows a hash check and a paste command only. |
| Root approval | **Aleph slice B passkey page only** | Home is not designed against slice B. |

### 1.2 Card convention (v1, description text)

The rules:
- **Strict parse.** A malformed block makes a card display-only. A display-only card shows
  no pick buttons and no root-run command, and it shows the reason it is display-only.
- **Edits after materialization** follow §1.3.4.

````text
<question prose>

Blocks: bead:mk-okek.8 thread:thr_abc123 project:autarch
Request: 6f1c…-uuid sha256:<16 hex>

```home-ask
{"schema":"home-ask/v2","project":"autarch","project_root":"/abs/root",
 "question":"…","options":[{"id":"a","label":"…","kind":"instruction","instruction":"…","reversible":true}],
 "ask_key":"…"}
```

```root-run
script: /abs/path/to/script.sh
sha256: <64 hex>
timeout: 900
set: <SET_ID>
```
````

**`Blocks:`**
- Refs take the forms `bead:`, `thread:thr_` and `project:`, with ids matching
  `[A-Za-z0-9:_.-]{1,128}`.
- Duplicates are removed.
- Unknown prefixes are shown but not counted.

**`Request:`**
- The first field is the filer's idempotency key. For agents it is a UUID. For Mycroft it is
  a UUIDv5 of the legacy key (§1.3.1).
- The second field is the **payload identity** (hash H1 in §1.3.4). It is the first 16 hex
  characters of the sha256 of the canonical JSON of
  `{project, title, blocks sorted, ask normalized, root_run}`.
  - **Only the filer** compares H1, against the token stored in `card_requests`, to tell a
    replay from a reused key.
  - Ingest never recomputes H1 from the card's content. A content edit is a T3 edit, not an
    identity mismatch.
  - Ingest treats the whole `Request:` line (key plus identity token) as immutable once the
    card is registered. Changing it is an invalidating edit (T4).

**`home-ask/v2`** is wire-only. Ingest converts it into a stored `home-ask/v1` ask, and every
row a rev-4 build reads is v1. Compared with v1:
- `kind` is always `decide`;
- `thread`, `asker`, `request_id` and `supersedes` are removed;
- `"pull":"mycroft"` is optional.

Option kinds, instruction bounds, `reversible`, the refusal of approval tokens, and the
identity/revision/semantic-key functions are all kept from v1.

**`root-run`**
- `set` must match `^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`.
- `script` must be an absolute path with no `..`, no control characters and no newline, and
  at most 4096 bytes.

### 1.3 Data flow and the card state machine

#### 1.3.1 File (agent side, in the asking thread): `autarch needs-mk file`

The filer works under an exclusive `flock` on
`${XDG_RUNTIME_DIR:-/tmp}/autarch-needsmk-$UID/<request>.lock`. It takes these steps in
order:

1. **Ensure the label.**
   - `bb tasks label list --project P --json`.
   - If the label is missing, `bb tasks label create --project P --name needs-mk`. "Already
     in use" counts as success.
2. **Look up the Request.** The label is never used as a filter. Home's registry is the
   **authority**, and the filer never creates a card without an answer from it (finding r3-5).
   1. **Registry.** Run `bb home get --request <key> --json`. It reads Home's
      `card_requests`, and the rev-4 `decisions.request_id` for legacy keys. It returns one of:
      - `registered` with the card id and identity token;
      - `legacy` with the decision id;
      - `absent`.

      If Home is unreachable, not ready, or returns anything else, the filer **exits 3 and
      creates nothing**. There is no Home-down creation path.
   2. **`registered`.** That card is canonical. Go to step 3. If the card is deleted, or its
      `Request:` line no longer matches the registry, exit 2: "Request <key> is registered to
      card <id>, which was deleted or changed; use a new Request". This is the case the
      review constructed, and it never creates a second card.
   3. **`legacy`.** Exit 2: "Request <key> belongs to a pre-cards ask <id>".
   4. **`absent`.** Search for cards filed but not yet materialized. Run
      `bb tasks list --project P --status backlog,todo,in_progress,in_review,done,canceled --search "Request: <key>" --limit 500 --json`.
      - Follow `nextCursor` with `--cursor` until it is null. Every page must succeed, and if
        any page fails the filer exits 3 **without creating anything** (finding r2-9).
      - For each hit, run `bb tasks show <id> --json` and parse it with the Task 2.1 parser.
        Keep every hit whose `Request:` **key** matches exactly, **whatever its identity
        token** (finding r4-2). The identity token is not a filter here; it is compared only
        after the canonical card is chosen (step 3).
      - If there are several key matches, the canonical card is the earliest by
        `(createdAt, id)`. This is the rule Home uses before registration (§1.3.3).
      - A hit whose description cannot be parsed far enough to read its `Request:` line is
        not a key match. If its raw text still contains the exact `Request: <key>` string,
        the filer exits 2 ("Request <key> appears on unparseable card <id>") and creates
        nothing.
      - Only if no card carries the key is the card created (step 4). A card deleted before it was
        ever materialized is not in the registry, so a new card is correct there: nothing
        could have been ruled on the deleted one.
3. **If a canonical card exists:**
   - **Identity token differs** from this payload's H1 (compared with the registry token, or
     with the canonical card's `Request:` token before registration) → exit 2, "Request
     reused for a different card". This check runs **before** any create, so a key collision
     with a different payload never produces a second card (finding r4-2).
   - **Routing.** `askingThread(comments)` is the threadId of the **earliest** agent comment
     (finding r2-8):
     - it equals `BB_THREAD_ID` → exit 0 (replay);
     - there is no agent comment → post one (step 5), then exit 0. The filer never succeeds
       before the routing comment exists;
     - it is any other thread → exit 2, "Request filed from thread <T>".
4. **Create.**
   `bb tasks create --project P --title … --description-file … --label needs-mk --json`.
5. **Comment.**
   `bb tasks comment <task.id> --body "Asked from this thread (Request <key>)." --json`.
   This is run from the thread, which makes it a `kind:"agent"` comment.

**Exit codes:**

| Code | Meaning |
|---|---|
| 0 | ok |
| 2 | usage error or refused |
| 3 | unavailable, including incomplete pagination |
| 4 | the card was created but the comment failed (a rerun repairs it) |
| 5 | not yet confirmed |

**Threadless exception.** This applies only to Mycroft and is mk question 4.
- The Go API `CardFiler.FileForPull` is called only from `cmd/mycroft`. There is no CLI
  flag for it.
- It writes `"pull":"mycroft"` and posts no comment.
- **Legacy check first.** It looks up the rev-4 key `mycroft:<project>:<bead>:<agent>`
  through `bb home get --request`. If a legacy ask is open, nothing is filed.

#### 1.3.2 Poll (Home server)

- **Interval.** Every `POLL_MS` (default 5 s). The poller never spawns the `bb` CLI.
- **Main read:**
  1. `callRpc({pluginId:"tasks"})` → `listProjects`.
  2. Per project, `listLabels`, cached for 60 s. The cache maps the name `needs-mk` to **all**
     label ids that currently carry that name.
  3. `listTasks{labelIds, statuses: open four, limit 500}`, following `nextCursor`. A
     partial read counts as unavailable, and nothing is withdrawn.
  4. Known cards that are missing from the list are re-read with `getTask`.
  5. **Label-loss confirmation (finding r3-6).** `getTask` returns label ids only, so it
     cannot tell a removed label from a renamed or recreated one. Before committing any
     withdrawal for "label missing", the poller:
     - bypasses the cache and re-reads `listLabels` for that project;
     - re-runs step 3 with the fresh ids;
     - decides the label is lost only if the card's `labelIds` contain none of the fresh
       `needs-mk` ids.

     If the label metadata read fails, nothing is withdrawn and the snapshot is kept (T12).
     A fresh label set that differs from the cache also replaces the cache at once, so a
     deleted, recreated and reapplied `needs-mk` label is picked up on the same poll.
- **Digest.** The poller keeps a digest over `(id, updatedAt, status, labelIds)` and
  ingests the cards that changed. The digest is committed only after the whole read
  succeeds.
- **Unresolved-retry set.** This runs independently of the digest. Each poll re-checks up to
  20 due cards whose state is `observed` (§1.3.4):
  - `listComments` for the asking thread;
  - the serve check of the root;
  - materialization.

  Backoff runs from 5 s to 5 min and keeps going while the card is open. The display text:
  - "waiting for asking thread" for the first 60 s, then "unroutable" (A8);
  - "root unverified: <reason>" when serve is down.
- **Routing revalidation set (finding r4-4).** This also runs independently of the digest,
  because the digest excludes comments and a deleted comment changes no `updatedAt`. It
  covers every **materialized** card in state `open` whose latest generation is not an
  override generation (T11). Each poll re-reads `listComments` for up to 20 of them, the
  ones whose `routing_check_at` is oldest, so each is re-checked about every 30 s with up to
  120 open cards (more cards stretch the interval, and Settings shows the current
  worst-case age).
  - If the earliest agent comment's thread differs from `cards.routed_thread`, or the
    routing mode changed, that is T4 and g n is withdrawn.
  - If the comment read fails, nothing changes and the snapshot is kept (T12).
  - The wake is always addressed to the generation's stored `thread`, which equals the
    frozen `routed_thread`, so the window before revalidation can delay a withdrawal but
    can never send a wake to a different thread.
- **Publish.** Any change publishes `home-queue-changed`.

#### 1.3.3 Request registry and observations (finding r2-7)

**`cards`** is one row per observed card. Its `request_key` is **not** unique.

**`card_requests(request_key PK, task_id, identity, registered_at)`** is insert-once: a
trigger aborts any UPDATE or DELETE.
- A row is written in the transaction that materializes a card's first generation.
- Before registration, the canonical card for a key is the earliest `(createdAt, id)` among
  the observed cards that carry it, so cards arriving in reverse order resolve the same way.
- After registration the registry wins. A card observed later with the same key is shown
  as "duplicate Request of KEY" and is never materialized, even if its `createdAt` is
  earlier.
- **Request edited on a materialized card.** If the card's parsed key differs from its
  registry row, that is an invalidating edit (§1.3.4). The registry is never rewritten.

#### 1.3.4 Card state machine (ingest, pick, override)

**Card states** (`cards.state`):

| State | Meaning |
|---|---|
| `observed` | Seen, but not yet materialized: waiting on parse, routing or root. |
| `display` | Display-only. The reason is in `display_reason`. |
| `open` | The latest generation is unpicked and not withdrawn. |
| `ruled` | Some generation has a pick and no newer generation is open. |
| `closed` | Withdrawn with no pick, through invalidation, gone or unlabel. |

**Generations** are rows in `decisions` with `source='card'`, `task_id`, and
`generation = n`.
- They are immutable: a trigger aborts updates of `body_json`, `revision`, `identity`,
  `subject`, `semantic_key` and `card_fp`.
- The card-link columns change only as the `decisions_card_link` trigger allows (finding
  r3-1):
  - `task_id`, `generation` and `tasks_project_id` may change only from NULL to a value;
  - `source` may change only from `'home'` to `'card'`, and only for a **verified
    adoption**. In one UPDATE, `task_id` goes from NULL to a value, and the row's
    `supersedes` names a `source='card'` row with the same `task_id` and generation
    `NEW.generation - 1`. The row's `request_id` must also be `'override:' || supersedes`;
  - `'card'` → `'home'` is never allowed.

  The full trigger is in Task 2.3.

**Hashes used for generations** (each one has a single job, finding r3-3):

| Hash | Computed over | Job |
|---|---|---|
| H1 Request identity token | `{project, title, blocks, ask, root_run}` at filing | Filer replay vs reuse only (§1.2). Frozen in `card_requests` |
| H2 `identity` (`model.ts` `identity()`) | The v1 ask except `request_id` | rev-4 `requests` registry row of each generation |
| H3 `revision` (`model.ts:463`) | scope, kind, question, options, recommendation, supersedes, steps, machine. **Not** Blocks, title or root-run | Pick gate (`recordPick` revision check) |
| H4 `semantic_key` | subject-based P-11 key | Stored for display only. `ingestCard` does **not** run the cross-thread merge at `service.ts:221-231` |
| H5 **`card_fp`** (new) | Canonical `{title, blocks sorted and deduplicated, ask as v1 without request_id/supersedes, root_run tuple, request line, asking_thread, tasks project}` | **Deciding whether a valid edit is a new generation.** Stored per generation, immutable |

Of these, H5 alone drives T3. A Blocks-only, title-only or root-run-only edit changes H5
but not H3. Before a pick it therefore produces g n+1, with a fresh `decision_blocks`
snapshot, and the new H3 differs anyway because `supersedes` differs. After a pick it falls
under T8.
- The id is `card-<task.id>-g<n>` (it matches `SAFE_ID`) and the request_id is
  `card:<task.id>:g<n>`. Adopted rows keep their minted `dec_…` id, which is also
  `SAFE_ID`-valid.
- The **current generation** is `max(generation)` for the card. It is derived on read and
  never stored as a counter, so it cannot drift (self-pass S-1).
- The block refs of each generation are snapshotted into `decision_blocks(decision_id, ref)`
  when it is materialized. Notices and the feed read the snapshot, never the live card
  (self-pass S-2).

**Transitions.** Each one runs in a single transaction.

| # | From | Event | Effect |
|---|---|---|---|
| T1 | (none) | First valid observation, routable, root verified | Insert g1 (`supersedes` NULL), `card_requests` row, `decision_blocks`. State → `open`. |
| T2 | `observed` | Not routable, root unverified or invalid | State stays `observed`, or goes to `display` if invalid. Stays in the retry set. |
| T3 | `open` | Valid edit (**H5 `card_fp` changed**, whatever happened to H3), **the scope `{asker, thread, project, project_root}` is unchanged** (so `store.ts:416-422` accepts it), **no generation of this card has a pick** | `insertReplacement(…,"replace")` inserts g n+1, which supersedes g n, with a new `decision_blocks` snapshot. The rev-4 mention copy (`store.ts:445-450`) is a no-op, because card rows have no mentions. **If `insertReplacement` refuses for any reason, the same transaction applies T4 instead**, so a refused replacement can never leave g n pickable (finding r4-3). |
| T4 | `open` | **Invalidating edit**: parse error, the `Request:` line (key or identity token) differs from `card_requests`, a v1-only field, a project mismatch (§1.3.6), a `project_root` change, or a **routing change** (finding r4-3): the routing mode (`thread` or `pull:mycroft`) differs from the mode frozen on the card at g1, or, in `thread` mode, the asking thread (earliest agent comment) differs from `cards.routed_thread` | Withdraw g n (`withdrawn_at`). State → `closed`, `display_reason` set. Stale tabs get rev-4 `withdrawn` (`store.ts:500,518`), in v2 and v3 builds alike (finding r2-2). |
| T5 | `closed` (no pick) | Valid again, or relabelled | Insert g n+1 with `supersedes` NULL (T4 withdrew g n, so a replace is not allowed). Lineage is the generation order. State → `open`. |
| T6 | `open` | `done`, `canceled`, deleted, or label missing (confirmed with fresh label metadata, §1.3.2 step 5), **and no generation of this card has a pick** | Withdraw g n. State → `closed`. |
| T7 | `open`/`ruled` | Same events, **some generation has a pick** | No withdrawal. Shown as "card closed in tasks" or "unlabelled". |
| T8 | `ruled` | Any edit, valid or not | No new generation. `changed_after_ruling` set. Never reopened. |
| T9 | `open` | Pick (mk or vizier) | `recordPick` (rev-4 gates). Same transaction inserts the wake, notices from `decision_blocks`, and `card_writes` (comment, unlabel). State → `ruled`. |
| T10 | `ruled` (vizier pick on g n) | mk override | `delegation.override` inserts g n+1 (`override:<g n id>`, body copied from g n, `delegable` 0). Old wake voided, void notice sent. State → `open`. |
| T11 | override generation open | T6 events or invalidating edits | **No withdrawal.** Override generations are mk's pending decision and are withdrawn only by mk's pick (self-pass S-3). |
| T12 | any | tasks unreachable, partial read, or label metadata read failed | Nothing. |
| T13 **[ROLLBACK-ONLY]** | any | Latest card generation superseded by a non-adopted legacy row (rollback replace, §1.3.8) | State → `display`, "replaced during rollback by <id>". No new generations. The legacy row is operable in the legacy lane. |

**Routing is frozen per card (finding r4-3).** At T1 Home stores `cards.routing_mode`
(`thread` or `pull`) and, in `thread` mode, `cards.routed_thread`. Neither column ever
changes afterwards (they are in the `cards_routing_frozen` trigger, Task 2.3). The asker
and thread are part of the rev-4 scope (`model.ts:421`), and `insertReplacement` refuses a
replacement whose scope differs (`store.ts:416-422`). So a routing change cannot be a T3
edit: adding `"pull":"mycroft"`, removing it, or a different earliest agent comment is
always T4, which withdraws g n. The card can only reopen (T5) once its routing matches the
frozen values again; a card that wants different routing needs a new Request.

**Why label absence can never hide an override (finding r2-3).** The rule in T6/T7 depends
on whether a pick exists, and the pick is committed in the same transaction that enqueues
the unlabel. So:
- if the unlabel succeeds and Home crashes before acknowledging it, the pick already exists,
  and T7 applies, not T6;
- an override generation is also protected by T11 on its own.

`home_unlabelled_at` is now informational only.

**Pick gate.** Rev-4 `recordPick` (`store.ts:494-503`) already refuses a pick on a row that
is withdrawn, resolved, superseded, already picked, or at the wrong revision. With T3, T4
and T6, a card that is no longer eligible always has its stale generation either superseded
or withdrawn. No new pick predicate is needed, and the gate holds in a rolled-back v2 build.

#### 1.3.5 Pick write-back

| Item | Store | What it does |
|---|---|---|
| (a) wake | `obligations` | `threads.send` to the asking thread (rev-4 marker). Not sent for `pull:mycroft` cards. |
| (b) notices | `obligations` | One per distinct non-asker `thread:` ref in `decision_blocks`. |
| (c) comment | `card_writes` (never `obligations`, finding r1-6) | `createComment{notify:false}`: "Ruled: <label> (Home, advisory; ruled by: <pick.by>)". |
| (d) unlabel | `card_writes` | Re-reads the card first, so it is idempotent. Sets `home_unlabelled_at` (informational). |

The ruling file carries `card_id`, `card_key`, `generation` and `supersedes`.

#### 1.3.6 Project binding and delegation scope (finding r2-6)

**Ruling scope** is the ask's Home project. That is `ask.project`, and serve verifies
`project_root` for it at materialization (G-1 refuses `estate`). Rev-4 delegation already
keys on it (`delegation.ts:129`, `d.project`). **That key is kept**, and the rev-5.1 switch
to `task.projectId` is withdrawn.

**`project_bindings(tasks_project_id PK, home_project, state, confirmed_at)`**
- `state` is one of `suggested`, `confirmed` or `rejected`.
- When the poller first sees a tasks project, it writes `suggested` if the project's name,
  lowercased, equals a serve project name. Otherwise it writes no row.
- Only mk can confirm or reject a binding, in Home Settings. Same-uid agents can rename
  tasks projects, so no binding confirms itself.

**At materialization:**
- `ask.project` must equal the binding's `home_project`, and the binding must be
  `suggested` or `confirmed`. Otherwise the card is display-only: "project mismatch:
  card in <tasks project>, ask targets <home project>". This rejects a card in tasks
  project A that targets a ruling in B.
- The binding id is stored on the generation (`decisions.tasks_project_id`).

**Delegation (`rule` by vizier) requires all of the following:**
- `d.project ∈ delegation.projects`, unchanged;
- `binding(d.tasks_project_id).home_project == d.project`;
- `binding.state == 'confirmed'`.

A `suggested` binding allows mk picks but refuses vizier, with "binding unconfirmed".

**Migration of existing name-based settings.** At v3 open:
- `delegation.projects` stays as Home project names. These are the ruling-scope keys, so no
  rewrite is needed.
- Each name is re-validated against serve's current project list. A name serve does not
  know is moved to `delegation.projects_inactive`, with a `delegation-settings-migrated`
  event, and shown in Settings. It is never silently matched.
- If serve is unreachable at open, the check is retried. Until it succeeds, delegation is
  refused, which fails closed.

#### 1.3.7 Legacy lane: cutover of pre-v3 asks (finding r2-5)

**Legacy decisions** are rows with `task_id IS NULL` that are not adopted (§1.3.8). They
are **not** migrated to cards:
- they have no card;
- machine and steps asks have no card form;
- inventing a card would forge agent authorship.

They drain in place:
- **Queue.** `queue` returns them in a "Legacy asks (pre-cards)" group, with their rev-4
  rendering:
  - decide asks get pick buttons;
  - steps asks get the runbook view;
  - machine asks get the lane rows.

  They sort after cards with an equal blocks count. Their count is their mention count, as
  in rev 4.
- **Commands.** The CLI lifecycle commands `progress`, `resolve` and `withdraw` keep working
  **only for legacy ids**:
  - for a card generation they exit 2, "card asks close through tasks";
  - `bb home ask` exits 2, "moved: use `autarch needs-mk file`", so no new legacy rows are
    created.
- **Code.** `asks.ts` and its mention logic, plus the runbook and lane UI components, stay
  until the drain completes.
- **Drain status.** Home Settings shows the count of open legacy asks.
- **Retiring the legacy code** is a follow-up bead under mk-okek. Its trigger is "0 open
  legacy asks for 14 days", so this plan does not delete that code.
- **Rule application.** Wakes, catch-up, feed, delegation and override apply to legacy rows
  exactly as in rev 4.

#### 1.3.8 Rollback and re-upgrade (findings r1-6, r2-1) [ROLLBACK-ONLY, see §1.3.9]

**What does not change:**
- `minReaderVersion` stays at 2.
- Stored asks are v1.
- Card writes live in `card_writes`, which the v2 worker's `DUE` filter never reads
  (`store.ts:183`).
- In v3, `DUE` becomes an allowlist.

**Adoption at v3 open.** A rolled-back v2 build can write rows that v3 must re-link:

| v2 action | Resulting row | v3 handling |
|---|---|---|
| Override of a card generation | `source='home', task_id NULL, generation NULL`, `request_id override:<card-…-gn>`, minted `dec_` id | Adopted, see below |
| Pick on a card generation | Pick with no `card_writes` | Back-filled |
| Rule on a card generation | (vizier pick) | Back-filled |
| `bb home ask` replace of a card generation (an agent in the asking thread, `supersedes: card-…`) | `source='home'`, `request_id` from the agent | **Not adopted**: its content came from the agent, not the card. It stays a legacy ask (§1.3.7). The card shows display-only "replaced during rollback by <id>" and never opens a new generation |

**Adoption (findings r3-1, r4-1). [ROLLBACK-ONLY, see §1.3.9]** Adoption covers **only**
genuine override rows. The `override:` prefix alone proves nothing: v2 accepts an ordinary
agent replacement whose `request_id` is `override:<predecessor>` (the `REQUEST_ID` grammar
allows `:`, `model.ts:260`). A row is adopted only when **all** of these hold:
- `source='home'` and `task_id IS NULL`;
- `request_id = 'override:' || supersedes`;
- `supersedes` names a row `p` with `source='card'`;
- **`p` has a vizier pick** (`picks.by = 'vizier'`). This alone excludes the forged case:
  rev-4 replace mode requires an *unpicked* predecessor (`store.ts:414`), and a superseded
  row can never be picked afterwards;
- **the override event exists**: an `events` row with `type = 'overridden'`,
  `decision_id` = this row, and `detail.supersedes = p.id`. Rev-4 writes it only in
  override mode (`store.ts:451`); replace mode writes `replaced`;
- **the content is the copied ask**: `delegable = 0`; `asker`, `thread`, `project` and
  `project_root` equal `p`'s; and the body equals `p`'s body apart from `request_id` and
  `supersedes` (`json_remove(body_json, '$.request_id', '$.supersedes')` compared on both
  rows; rev-4 `delegation.ts:154` builds the override body from `p.body_json`, and both are
  `normalizedJson` canonical output).

The predecessor must itself be a card row, so v3 adopts in supersedes order: a chain
g1 → dec_X → dec_Y adopts dec_X as g2 first, then dec_Y as g3.

Each adoption is one UPDATE:

```sql
UPDATE decisions
   SET source='card', task_id=@task, generation=@pred_gen + 1, tasks_project_id=@tp
 WHERE id=@id
```

The `decisions_card_link` trigger (Task 2.3) allows exactly this `'home'` → `'card'`
transition and nothing else. It rechecks every condition above inside the database, so a
buggy caller cannot adopt an unrelated row.

After adoption, v3:
- recomputes the card state from §1.3.4;
- back-fills `card_writes` for every picked generation that lacks them;
- writes `decision_blocks` and `card_fp` for adopted rows by copying from the predecessor
  (the override body equals the predecessor's).

Adoption is idempotent and runs in one transaction.

**The v3 triggers run under a rolled-back v2.** They live in the DB file. The v2 build
writes only:
- INSERTs to `decisions`;
- UPDATEs of `updated_at` and the resolution columns (`asks.ts:138,173`).

Neither touches a guarded column, so no v3 trigger aborts a v2 write. The rollback test
(Task 2.8a) asserts this.

#### 1.3.9 Rollback decision (pending mk/vizier)

Finding r4-1 is the fourth round in which the compatible-rollback design (§1.3.8) produced a
P1. Rev 5.4 fixes r4-1 cheaply (verified adoption above) and adds **no** further rollback
machinery. Whether to keep that machinery at all is a decision for mk or the vizier (Q7).

**Option (a): forward-only v3 with a pre-migration backup.**
- **Backup.** When v3 opens a DB at `user_version = 2`, before any v3 DDL, it runs SQLite
  `VACUUM INTO '<data>/home-v2-backup-<utc>.db'`. This is a SQL statement inside the plugin's
  own SQLite connection: no exec, no script, no new fs API. It writes one new file in the
  plugin's existing data directory (file mode 0600; it holds the same data as the live DB).
  v3 then opens the backup read-only and checks `PRAGMA integrity_check = 'ok'` and
  `user_version = 2`. If the backup or the check fails, the migration aborts with no DDL
  applied and the plugin reports the error; the DB stays v2.
- **Fence.** The v3 migration sets `min_reader_version = 3`. The a9853e2 build then refuses
  to start on the v3 DB ("database requires reader version 3 … upgrade the plugin",
  `migrations.ts:221`), so no v2 code ever writes a v3 DB.
- **Rollback procedure** (mk-run; a script per the hands-on rule): stop serve; move the v3
  DB aside (kept, not deleted); copy the backup into place; install the a9853e2 build.
- **Cost.** Every decision, pick, ruling, override and observation made after the
  migration is absent from the restored Home. Ruling files already written and the tasks
  cards themselves remain. A card picked under v3 can reappear as open after a later
  re-upgrade (Home lost the pick), so mk may be asked again; with Q2 unlabel on, a picked
  card has no `needs-mk` label and is not re-materialized.
- **Tests under (a)** (they replace the rollback-only tests): Task 2.3 test 8, the backup
  file exists, passes `integrity_check`, has `user_version = 2` and the pre-migration row
  counts, and `min_reader_version = 3` is set; a forced `VACUUM INTO` failure leaves the DB
  at v2 with no v3 table. Task 2.8a test 1 becomes: the a9853e2 build refuses to start on
  the v3 DB, and the documented restore brings it back up on the backup.

**Option (b): compatible rollback (the current design).** `minReaderVersion` stays 2, the
v2 worker is fenced, v3 adopts verified v2 overrides (§1.3.8) and shows non-adopted
replacements as T13. Its residuals: S-9 (a vizier rule under v2 on an unconfirmed
binding), and same-uid direct writes to the SQLite file, which no trigger can stop.

**Recommendation.** None forced. (a) removes the class of finding that has recurred in every
round (r1-6, r2-1, r3-1, r4-1) at the price of losing post-migration Home state on
rollback. (b) keeps that state and keeps the machinery.

**Everything marked [ROLLBACK-ONLY]** exists only for (b). If (a) is ruled, delete it, and
add the two (a) tests above:

| Site | Item |
|---|---|
| §1.3.4 T13 row | Card superseded by a non-adopted legacy row |
| §1.3.8 | The whole section (the `minReaderVersion` 2 rule, worker fencing, adoption table, adoption) |
| §2.1 row 1.7, §2.2 row 10 | DUE allowlist (harmless to keep; its reason disappears) |
| Task 2.3 | `minReaderVersion` stays 2; `adoptOverrides()`; the `'home'` → `'card'` branch of `decisions_card_link` (the `'card'` NULL → value rule and the no-link-on-`'home'` clause stay); test 4 adoption rows; test 6; test 7; Worker bullet |
| Task 2.4 | T13 test |
| Task 2.8a | Test 1 (rollback). Test 2 (cutover, A11) stays |
| §3 A10 | The whole criterion |
| §8.2 r2-1, §8.3 S-9, §8.5 S-11 | Dispositions that exist only for (b) |

`card_writes` as a separate table also began as worker fencing (r1-6). Under (a) it can
stay as is; deleting it is optional and not required.

---

### 1.4 The blocks panel

- **Registration.** `app.slots.threadPanelAction({ id: "home-blocks", title: "Blocking", component })`,
  opened with `openThreadPanel(threadId, "home-blocks")`.
- **Content.** Open card generations across all tasks projects, followed by the legacy
  group.
- **Sort order:**
  1. the derived blocks count, highest first;
  2. `createdAt`, oldest first;
  3. id.
- **Pinning.** Rows whose `decision_blocks` include `thread:T`, or whose asking thread is
  T, are pinned under "this thread".
- **Row contents.** Each row shows:
  - the key, project, title, refs and age;
  - pick buttons, or a link to open the card in tasks or the thread;
  - the display reason (parse error, identity mismatch, duplicate Request, project
    mismatch, unroutable, root unverified);
  - "overrides vizier ruling g<n>" on override generations;
  - "changed after ruling" when that flag is set;
  - the root-run section.

### 1.5 Root runs (display only)

**`rootRun(task_id)`** works in three steps:
1. Strict parse.
2. Open the script with `O_RDONLY|O_NOFOLLOW` after `lstat`. It must be a regular file of
   at most 256 KiB.
3. Hash it and return `match`, `mismatch`, `unreadable` or `invalid`.

**When the paste command renders.** Only when all of these hold:
- the state is `match`;
- `set` is valid;
- `owner_thread`, which is `askingThread(comments)`, matches `^thr_[A-Za-z0-9]{1,64}$`;
- `task.id` matches `^[0-9A-HJKMNP-TV-Z]{26}$`.

Otherwise the panel names the field that failed and renders no command.

**Quoting.** `pasteCommand()` single-quotes every argument with `shQuote()`, which refuses
`'`, control characters and non-ASCII. The rendered command is:

`todo-add --set '<SET>' --from-card '<task.id>' --expect-sha256 '<hex>'`

**Item JSON.** `JSON.stringify` of
`{run_as:"operator", script, script_sha256, owner_thread, run_timeout_s, label}`.

**Status.** The panel shows the read-only `todo-run --status '<set>' '<item>'` command for
mk to paste. Reading it automatically is question D-1.

**Hard limits** (Tasks 2.7 and 2.8):
- no process execution in plugin server code;
- no route writes Aleph state;
- no "approved" wording;
- the approvals table is never touched;
- the badge reads "run by paste, not authenticated".

### 1.6 Hand-off to Aleph (thr_39wwcmwi84), relayed by mk

The adapter:
- reads `task.id` (never `task.key`), `task.projectId` and the `root-run` block, validated
  with the §1.2 grammar;
- takes `owner_thread` from the **earliest** `kind:"agent"` comment, the same selector as
  §1.3.1;
- uses the item label `bbtask:<task.id>:<attempt>` as the two-way link;
- reads status with `todo-run --status --json`;
- must re-pin the script bytes and refuse a sha256 mismatch.

The card text can be written by any same-uid agent.

---

## 2. Rev-4 S1 tasks and acceptance criteria

**Committed on `main` (7507fc6…a9853e2):** Step 0, Tasks 1.1–1.11, and the interim part of
1.13. Task 1.12 is not built.

**Marks:**
- SURVIVES: unchanged.
- CHANGES: adapted by a Step 2 task.
- LEGACY-DRAIN: kept unchanged for pre-v3 rows only, and retired by a follow-up bead after
  the drain.

Tables are never dropped.

### 2.1 Tasks

| Rev-4 task | Mark | Reason | Code |
|---|---|---|---|
| 0.1–0.4, 0.6 | SURVIVES | Independent of the queue. Serve still resolves roots | Kept |
| 0.5 `autarch mcp` + `autarch_file_decision` | CHANGES | Files through CardFiler | `cmd/autarch/mcp.go` (Task 2.10) |
| 1.1 model, vectors | CHANGES | v2 wire form added (converts to v1), plus card vectors | `model.ts`, `model.go`; new `cards.ts`, `card.go` (Task 2.1) |
| 1.2 store, migrations, export | CHANGES | v3: `cards`, `card_requests`, `card_writes`, `decision_blocks`, `project_bindings`, card columns, triggers, adoption. `requests` stays live | Task 2.3 |
| 1.3 ruling writer | CHANGES (small) | Adds `card_id`, `card_key`, `generation`, `supersedes` | `ruling.ts` (Task 2.5) |
| 1.4 service | CHANGES | `ingestCard()` and the state machine; the `file()` route is retired | `service.ts` (Tasks 2.4, 2.5) |
| 1.5 mentions, machine lane, runbook | **LEGACY-DRAIN** | New asks use cards. Pre-v3 asks keep these paths (§1.3.7) | `asks.ts`, `ui/asks.tsx` lane/runbook kept, legacy ids only (Task 2.7) |
| 1.6 delegation, override | CHANGES | Override generations; scope check adds the confirmed binding; names migrated (§1.3.6) | `delegation.ts` (Task 2.5) |
| 1.7 RPC, wakes, CLI, catch-up, supervisor, feed | CHANGES | `queue`/`rootRun` RPCs; DUE allowlist **[ROLLBACK-ONLY]**; CLI `ask` retired, lifecycle commands legacy-only, `get --card/--request`, `list --pull`; supervisor spawn retired; feed text and recipients | `contract.ts`, `cli.ts`, `server.ts`, `serve.ts`, `feed.ts`, `wakes.ts` |
| 1.8 Home tab UI | CHANGES | Reads `queue`; blocks panel; legacy group; binding settings | `app.tsx`, `ui/asks.tsx`, new `ui/blocks.tsx`, `ui/settings.tsx` |
| 1.9 Go filers, `autarch decide`, Mycroft | CHANGES | CardFiler added in 2.9; callers and `cmd/mycroft/main.go:94` move in 2.10, where ExecFiler is deleted | Tasks 2.9, 2.10 |
| 1.10 scenario harness | CHANGES | Fake tasks plugin, `rigexec.ts` | Task 2.11 |
| 1.11 real-bb e2e | CHANGES | Harness-owned server (A7) | Task 2.12 |
| 1.12 overlay (G-10) | SURVIVES | Renders `queue` | Not built |
| 1.13 interim approvals | SURVIVES | Root-run never reads or writes approvals | Kept |
| (docs) skill, README, PLUGIN_OVERVIEW, AGENTS.md | CHANGES | They teach retired commands | Task 2.7 |

### 2.2 Acceptance criteria (rev-4 numbering)

| # | Mark | New form |
|---|---|---|
| 1, 2, 3, 12 | SURVIVES | — |
| 4 mcp | CHANGES | Asserts the CardFiler call sequence |
| 5 model | CHANGES | Card vectors in Go and TS; v2 → v1 conversion |
| 6 store | CHANGES | v3 migration, triggers and adoption at store level (Task 2.3); rollback (A10) and populated-v2 cutover (A11) as integration acceptance (Task 2.8a) |
| 7 ruling | CHANGES | A real write with a `card-…-g<n>` id |
| 8 service | CHANGES | State-machine transitions T1–T13 |
| 9 feed/stats | CHANGES | New instruction text; recipients from `decision_blocks` plus the asking thread; legacy mentions kept for legacy rows |
| 10 wakes/CLI | CHANGES | DUE allowlist **[ROLLBACK-ONLY]**; filer exit codes; legacy-only lifecycle commands |
| 11 Mycroft | CHANGES | CardFiler wiring; legacy de-duplication; no private list |
| 13 fake harness | CHANGES | Task 2.11 scenario list; every rev-4 scenario kept or mapped (table in Task 2.11, finding r3-8) |
| 14 real-bb | CHANGES | Full replacement command in Task 2.12 (findings r2-11, r3-4); real list keeps `queued-then-archived` and `vizier-chat`, maps `ask-cli-proxy` → `filer-from-thread` |
| 15 asks/delegation/catch-up/UI | CHANGES | Machine lane and runbook kept for legacy rows only; adds blocks sort, panel, binding settings, root-run limits |
| 16 mk walk (G-6) | CHANGES | On cards: file → pick → wake → ruling, delegated ruling + override, plus one legacy ask resolved |

---

## 3. New tasks (Step 2)

**Order:** 2.0 → 2.1 → 2.2 → 2.3 → 2.4 → 2.5 → 2.6 → 2.7 → 2.8 → **2.8a** → 2.9 → 2.10 →
2.11 → 2.12. Each task's verify uses only behaviour built by that task or an earlier one
(finding r3-7).
`P = integrations/bb-plugin-autarch`.

Every task is TDD: write the failing test, make it pass, then verify. Nothing is pushed.

### Task 2.0: baseline and prerequisites

- **Baseline.** Run rev-4 criteria 1, 12 and 13 at HEAD and record the results outside the
  checkout. Confirm tasks is version 0.1.2.
- **CI prerequisite (finding r3-9).** The historical evidence is rev-4 G-0, checked
  2026-09-29:
  - immutable GitHub repository id `1140086114`;
  - canonical registry name `mistakeknot/Autarch`;
  - campaign `mk-ag2s`, migration task `mk-ag2s.18`, still in progress.

  The lowercase name `mistakeknot/autarch` returns "repository not registered" and is
  misleading. Rev 5.2 wrongly reported that lowercase result as non-registration, and that
  claim is withdrawn.

  **Fresh canonical lookup, re-run at the start of this task.** I already ran it on
  2026-10-01:
  - `gh api repos/mistakeknot/Autarch --jq '.id,.full_name,.private'` →
    `1140086114`, `mistakeknot/Autarch`, `false`;
  - `<host>-ci status --repo mistakeknot/Autarch --json` lists id 1140086114 with
    `campaign: mk-ag2s`, `disposition: pending-inventory`,
    `inventory_disposition: requires-workflow-review`, `enabled: true`.

  The migration is not complete. Claiming `mk-ag2s.18` would need the hub tracker, which
  this plan does not touch, so that stays with mk or the session that owns `mk-ag2s`.
  `private: false` is still mk's to review (rev-4 G-0).
- **Verify:** `go build ./cmd/... && go test -race ./... && (cd P && npm run typecheck && npm test)`.

### Task 2.1: card convention parser (TS + Go, shared vectors)

**Files:**
- new: `P/cards.ts`, `P/__tests__/cards.test.ts`, `P/__tests__/fixtures/cards/*.json`;
- new: `internal/homeask/card.go`, `card_test.go`;
- adapted: `P/model.ts` and `model.go` (v2 parse, `toV1()`);
- new: `askingThread(comments)` in TS and Go, shared by every caller.

**Vectors:**
- `Blocks`: one or several lines, de-duplication, the id grammar, unknown prefixes.
- `Request` key and identity: a match, and a mismatch after an edit.
- `home-ask/v2`:
  - valid, and refused when it carries a v1-only field;
  - `pull` set to `mycroft` is accepted; any other value is refused;
  - `toV1()` output passes the **unchanged** rev-4 `parseAsk`, for both thread and mycroft
    askers.
- Approval tokens are refused.
- `root-run` hostile vectors:
  - a relative path, `..`, a newline or control character in the path;
  - bad hex;
  - a timeout of 0 or 3601;
  - a duplicate block;
  - `set` values `x; rm -rf ~`, `$(id)`, `` `id` ``, `a'b`, `a b`, the empty string, 65
    characters, and a lookalike character.
- Two `home-ask` blocks.
- `askingThread`:
  - user, agent A, agent B → A;
  - no agent comments → none;
  - comments out of order → sorted by `(createdAt, id)` first.

**Verify:** `cd P && npm test -- __tests__/cards.test.ts __tests__/model.test.ts && cd ../.. && go test -race ./internal/homeask/ -run 'Card|Vectors|Validate|AskingThread'`.

### Task 2.2: tasks client (server)

**Files:** `P/tasks.ts`, `P/__tests__/tasks-fake.ts`, `P/__tests__/tasks.test.ts`.

**Tests:**
- the label cache;
- full pagination, where a page failure surfaces as an error;
- comment ordering;
- `getTask` tells deleted apart from unlabelled;
- zod validation of responses;
- every call goes through `callRpc({pluginId:"tasks"})`.

**Verify:** `cd P && npm test -- __tests__/tasks.test.ts`.

### Task 2.3: migration v3, triggers, adoption (store level)

This task covers schema and store behaviour only. The end-to-end rollback and cutover
acceptance needs the queue, write-back and CLI from Tasks 2.4–2.7, so it moves to Task 2.8a
(finding r3-7).

**Files:**
- `P/migrations.ts`: `CODE_VERSION = 3`; `minReaderVersion` stays 2 **[ROLLBACK-ONLY]**;
- `P/store.ts` (`adoptOverrides()` **[ROLLBACK-ONLY]**, `backfillCardWrites()`), `P/wakes.ts`;
- tests: `migrations.test.ts`, `store.test.ts`, new `adoption.test.ts`.

**Schema:**
- **`cards`** (one row per observed card):
  - columns `task_id PK`, `project_id`, `card_key`, `title`, `request_key` (not unique),
    `request_identity`, `asking_thread` (last observed);
  - `routing_mode` (`thread` or `pull`) and `routed_thread`, frozen at T1 (finding r4-3);
    the `cards_routing_frozen` trigger allows each only NULL → value;
  - `routing_check_at` for the routing revalidation set (finding r4-4);
  - `state` is one of `observed`, `display`, `open`, `ruled` or `closed`;
  - also `display_reason`, `root_state`, `root_reason`, `changed_after_ruling`,
    `home_unlabelled_at`, `next_check_at`, `check_attempts`;
  - also `created_at`, `updated_at`, `status`, `labelled`, `blocks_json`, `first_seen_at`,
    `last_seen_at`, `deleted_at`;
  - there is **no generation counter**.
- **`card_requests(request_key PK, task_id, identity, registered_at)`**, with a trigger that
  aborts UPDATE and DELETE.
- **`decision_blocks(decision_id, ref, PK(decision_id, ref))`**, insert-only, enforced by
  trigger.
- **`card_writes(id PK, task_id, decision_id, kind, payload, state, attempt, last_error, next_try_at, updated_at, UNIQUE(decision_id, kind))`**:
  - `kind` is one of `comment`, `unlabel` or `relabel`.
- **`project_bindings(tasks_project_id PK, home_project, state, suggested_at, confirmed_at)`**.
- **`decisions`** gains:
  - `source TEXT NOT NULL DEFAULT 'home' CHECK (source IN ('home','card'))`;
  - `task_id`, `generation`, `tasks_project_id`;
  - `card_fp` (H5).

  A unique index covers `(task_id, generation)` where `task_id` is not null.
- **Triggers on `decisions`:**
  - `decisions_ask_immutable` aborts any UPDATE of `body_json`, `revision`, `identity`,
    `subject`, `semantic_key` or `card_fp`. The one exception is a `card_fp` going from
    NULL to a value, which is how adoption copies it.
  - `decisions_card_link` (finding r3-1):

    ```sql
    CREATE TRIGGER decisions_card_link BEFORE UPDATE OF source, task_id, generation, tasks_project_id
    ON decisions
    WHEN NOT (
      (OLD.task_id IS NEW.task_id OR OLD.task_id IS NULL)
      AND (OLD.generation IS NEW.generation OR OLD.generation IS NULL)
      AND (OLD.tasks_project_id IS NEW.tasks_project_id OR OLD.tasks_project_id IS NULL)
      AND (
        OLD.source = NEW.source
        OR (OLD.source = 'home' AND NEW.source = 'card'
            AND OLD.task_id IS NULL AND NEW.task_id IS NOT NULL
            -- [ROLLBACK-ONLY] the whole 'home' -> 'card' adoption branch below (§1.3.9)
            AND OLD.request_id = 'override:' || OLD.supersedes
            AND OLD.delegable = 0
            AND EXISTS (SELECT 1 FROM decisions p
                         WHERE p.id = OLD.supersedes AND p.source = 'card'
                           AND p.task_id = NEW.task_id
                           AND NEW.generation = p.generation + 1
                           AND p.asker = OLD.asker AND p.thread IS OLD.thread
                           AND p.project = OLD.project
                           AND p.project_root IS OLD.project_root
                           AND json_remove(p.body_json, '$.request_id', '$.supersedes')
                             = json_remove(OLD.body_json, '$.request_id', '$.supersedes')
                           AND EXISTS (SELECT 1 FROM picks k
                                        WHERE k.decision_id = p.id AND k."by" = 'vizier'))
            AND EXISTS (SELECT 1 FROM events e
                         WHERE e.type = 'overridden' AND e.decision_id = OLD.id
                           AND json_extract(e.detail_json, '$.supersedes') = OLD.supersedes))
      )
      AND NOT (NEW.source = 'home'
               AND (NEW.task_id IS NOT NULL OR NEW.generation IS NOT NULL
                    OR NEW.tasks_project_id IS NOT NULL))
    )
    BEGIN SELECT RAISE(ABORT, 'decisions card link: transition not allowed'); END;
    ```

    The last clause means a `'home'` row never carries any link column, so a legacy row can
    gain a link only by becoming a card through verified adoption (self-pass, rev 5.3). A
    companion `decisions_home_insert` trigger (`BEFORE INSERT … WHEN NEW.source = 'home' AND
    (NEW.task_id IS NOT NULL OR NEW.generation IS NOT NULL OR NEW.tasks_project_id IS NOT
    NULL)`) applies the same rule to inserts. v2 inserts never name these columns, so they
    stay NULL and pass.
- **Worker** **[ROLLBACK-ONLY]**: the v3 `DUE` filter becomes an allowlist of the rev-4 message kinds.

**Migration of delegation settings at open** (§1.3.6):
- Names serve does not know go to `delegation.projects_inactive`.
- Delegation is refused until the check succeeds.

**Tests** (store level only):
1. The v2 → v3 migration on a populated fixture, with every row and column preserved.
2. Uniqueness of `(task_id, generation)`.
3. The immutability trigger:
   - it rejects updates of `body_json` and of a non-NULL `card_fp`;
   - it allows `updated_at`, `resolved_at`, and `card_fp` from NULL.
4. **The `decisions_card_link` trigger, as a table of transitions:**
   - **allowed:**
     - NULL → value for each link column on a `'card'` row;
     - a verified override adoption (`'home'` → `'card'`) **[ROLLBACK-ONLY]**;
     - a chained adoption **[ROLLBACK-ONLY]** g1 → X → Y in supersedes order;
   - **refused:**
     - `'card'` → `'home'`;
     - **[ROLLBACK-ONLY]** every `'home'` → `'card'` row below, through "adopting Y before X" (under
       option (a) all `'home'` → `'card'` is simply refused, one row);
     - `'home'` → `'card'` without a card predecessor;
     - `'home'` → `'card'` where `request_id` is not `override:<supersedes>` (a rollback
       replace);
     - **forged override (finding r4-1):** an agent replacement written through the
       a9853e2 store in replace mode with `request_id = override:<g1>`; refused because g1
       has no vizier pick and the event is `replaced`;
     - `'home'` → `'card'` with a vizier pick but no `overridden` event;
     - `'home'` → `'card'` whose body differs from the predecessor's (one option label
       changed), or whose `delegable` is 1, or whose `thread` differs;
     - `'home'` → `'card'` with the wrong generation;
     - `'home'` → `'card'` with a different `task_id`;
     - value → other value for `task_id`, `generation` and `tasks_project_id`;
     - a `'home'` row gaining `task_id`, `generation` or `tasks_project_id` (each
       separately), by UPDATE or by INSERT;
     - adopting Y before X.
5. The `card_requests` triggers (UPDATE and DELETE abort), the `decision_blocks`
   triggers (UPDATE and DELETE abort), and `cards_routing_frozen` (finding r4-3).
6. **[ROLLBACK-ONLY]** `adoptOverrides()` on store fixtures **written directly through the a9853e2 `Store`**:
   g1 → v2-style override row. It adopts the override, back-fills `card_writes` and copies
   `decision_blocks` and `card_fp`. A rollback replace row is left alone. A second run
   changes nothing.
7. **[ROLLBACK-ONLY]** A v2-style write against the v3 schema succeeds (INSERT, plus updates of `updated_at` and
   `resolved_at`). The triggers never abort it.

**Verify:** `cd P && npm test -- __tests__/migrations.test.ts __tests__/store.test.ts __tests__/store-crash.test.ts __tests__/adoption.test.ts`.

### Task 2.4: poller, ingest and the state machine

**Files:**
- new: `P/queue.ts`, `P/__tests__/queue.test.ts`;
- adapted: `P/service.ts` (`ingestCard()`, which reuses `parseAsk`, identity/revision,
  `insertReplacement` and the serve root check);
- adapted: `P/store.ts`. `insertDecision` and `insertReplacement` gain the `source`,
  `task_id`, `generation`, `tasks_project_id` and `card_fp` columns in their INSERT lists,
  so card generations are created with their links and never need a later link UPDATE;
- new: `cardFingerprint()` (H5) in `P/cards.ts`;
- adapted: `P/server.ts`.

**Tests.** Each transition T1–T8 and T11–T13 has a named test here, driven only by the
fake tasks client (Task 2.2) and **store fixtures**: picks are written with the rev-4
`store.recordPick`, and override rows with the rev-4 `store.insertReplacement(…,
"override")`. Nothing here needs unlabel write-back, card-aware override, or the ruling
writer. T9 and T10, and every integration test that needs them, are in Task 2.5 (finding
r4-5).

**State-machine and pick-gate tests:**
- **T3:** an edit before a pick creates g2, and a pick on the old revision is refused as
  stale.
- **T3 on H5 only (finding r3-3).** Each of these produces g2 with a fresh
  `decision_blocks` snapshot and a different `card_fp`, while the ask's H3 content is
  unchanged:
  - a Blocks-only edit before any pick;
  - a title-only edit before any pick;
  - a root-run-only edit before any pick.

  A pick on g1 after the Blocks-only edit is refused as `superseded`. After a pick, a
  Blocks-only edit creates no generation, sets `changed_after_ruling`, and the notices go to
  the g1 snapshot's recipients only (asserted in Task 2.5 #1).
- **No-op re-observation.** A card whose H5 is unchanged (a status change or a comment)
  never creates a generation.
- **T8:** after a pick, the g1 row is byte-identical and `changed_after_ruling` is set.
- **T4 (finding r2-2):** an old tab picks g1 after the card was made invalid. Run once for
  each cause:
  - a broken JSON block;
  - a `Request:` identity token changed;
  - an edited Request key;
  - a project mismatch;
  - the earliest agent comment deleted, so the asking thread changes (rev 5.3).

  Each pick is refused as `withdrawn`, and no wake is addressed to the new thread.
- **T5:** a card fixed after T4 opens g2, and a pick on g2 succeeds.
- **T6/T7:** unlabel or `done` with no pick withdraws g n. With a fixture pick, nothing is
  withdrawn.
- **T11 (store fixture):** an override row inserted through the rev-4 store on a picked g1
  survives `done`, unlabel and an invalid edit.
- **Routing frozen (finding r4-3), both directions.** Each case is followed by an old-tab
  pick on g1, which is refused as `withdrawn`:
  - a `thread`-mode card gains a valid `"pull":"mycroft"` → T4;
  - a `pull`-mode card loses `"pull":"mycroft"` (and has an agent comment) → T4;
  - a `thread`-mode card's earliest agent comment is from a different thread → T4;
  - restoring the original routing reopens the card as g n+1 (T5); a third routing gets
    T4 again.
  - **Refused replacement falls back to T4:** a store hook forces `insertReplacement` to
    refuse during a T3; g n is withdrawn in the same transaction and stays unpickable.
  - `cards_routing_frozen` rejects any value → other update of `routing_mode` and
    `routed_thread`.
- **Routing revalidation (finding r4-4).** The card's task metadata (`updatedAt`, status,
  labels, description) is held unchanged, so the digest sees nothing:
  - the earliest agent comment is deleted: within the revalidation interval g1 is
    withdrawn (T4) and an old-tab pick is refused;
  - `listComments` fails on every revalidation: g1 stays `open`, the snapshot is unchanged,
    and nothing is withdrawn (T12);
  - 150 open cards: each is re-checked within the stated worst-case age, which Settings
    reports;
  - an open override generation is not revalidated (T11).

**Retry tests:**
- **Retry set.**
  - `comment-arrives-later`: the comment arrives with no `updatedAt` change.
  - `serve-recovers`.
  - "unroutable" shows after 60 s, while retries continue.
  - Backoff is capped at 5 min.

**Registry and scope tests:**
- **Registry (finding r2-7):**
  - duplicate cards arriving in order and in reverse order: same canonical card before
    registration, and the registry wins after it;
  - a later card with an earlier `createdAt` is still the duplicate once registration
    exists;
  - a Request edit on a registered card is T4.
- **Project mismatch** is display-only.
- **Bindings:** a `suggested` binding is written by an exact name match and never by a
  fuzzy one.

**Label tests (finding r3-6):**
- `needs-mk` is deleted, recreated with a new id and reapplied to an open card. The card
  stays `open`, and nothing is withdrawn on the poll that sees the new id.
- The label is genuinely removed: the card is withdrawn only after the fresh `listLabels`
  confirms it.
- `listLabels` fails during confirmation: nothing is withdrawn, and the snapshot is kept.
- Two labels named `needs-mk` exist: either one counts as labelled.

**Degraded-mode and invariant tests:**
- A partial page read changes nothing.
- **[ROLLBACK-ONLY]** **T13:** a card generation superseded by a non-adopted legacy row (store fixtures: a
  plain replace, and a forged `override:` replace per r4-1) goes
  to `display` "replaced during rollback" and opens no generation on later edits.
- When tasks is unavailable, health becomes `degraded: tasks` and the last snapshot is kept.
- The current generation is always `max(generation)` (property test over random event
  sequences).

**Verify:** `cd P && npm test -- __tests__/queue.test.ts __tests__/service.test.ts`.

### Task 2.5: pick write-back, delegation scope, override generations

**Files:**
- `P/service.ts`;
- new `P/cardwrites.ts`;
- `P/delegation.ts`, `P/ruling.ts`;
- tests: `service`, `cardwrites`, `delegation`, `ruling`.

**Tests:**
1. A pick inserts, in one transaction, the wake, the notices (from `decision_blocks`) and
   the `card_writes` rows. A `pull:mycroft` card gets no wake. Two cases for notice
   recipients (finding r3-3):
   - Blocks are edited before the pick: the notices go to the g2 snapshot's threads;
   - Blocks are edited after the pick: the notices go only to the picked generation's
     snapshot.
2. The comment renders `pick.by`, so a delegated ruling shows "ruled by: vizier".
3. The unlabel is idempotent. If it fails, the wake and the ruling are not blocked. A
   re-added label never re-queues the card.
4. **Delegation scope (finding r2-6).** A vizier `rule` is refused in each of these cases:
   - a card in tasks project A, whose binding is to `alpha` with delegation enabled, that
     targets `ask.project=beta`. The card is display-only, and the rule is refused;
   - the binding is only `suggested` ("binding unconfirmed");
   - the binding was rejected.

   It is allowed with a confirmed binding and an enabled name. The rev-4 limits still apply.
5. **Override:** g1 picked by vizier → mk overrides →
   - the new generation has id `card-<id>-g2` and request `override:card-<id>-g1`;
   - its body equals g1's and it is not delegable;
   - it is visible after the unlabel;
   - the old wake is voided and a notice sent;
   - the g2 ruling names `supersedes`;
   - a replay returns g2.
6. A real `writeRuling` with `card-01J…-g1` succeeds, and the `card:<id>` form is refused
   with `bad-name`.
7. Back-fill on start.

**Integration tests moved here from Task 2.4 (finding r4-5).** They need the unlabel
write-back, card-aware override and the ruling writer built in this task. They live in a
new `P/__tests__/card-integration.test.ts`:
8. **Crash window (finding r2-3).** g1 is picked by vizier, the unlabel succeeds on tasks,
   and the process is killed before `home_unlabelled_at` is written (`store` hook).
   - After restart and an mk override, g2 stays `open` across 10 polls.
   - Variants: the unlabel response is lost; the process restarts before the override; the
     process restarts after the override.
9. **Immutable snapshot.** pick → failed ruling write → card edit → restart → retry. The
   original text is written.
10. **T11 end to end.** A real mk override of a vizier pick (T10) survives `done`, unlabel
    and an invalid edit, and mk's pick on it succeeds.

**Verify:** `cd P && npm test -- __tests__/service.test.ts __tests__/cardwrites.test.ts __tests__/wakes.test.ts __tests__/delegation.test.ts __tests__/ruling.test.ts __tests__/card-integration.test.ts`.

### Task 2.6: `queue` RPC, panel, Home tab, binding settings

**Files:**
- `P/contract.ts` (`queue`; `listAsks` kept as an alias);
- `P/ui/blocks.tsx`, `P/app.tsx`, `P/ui/asks.tsx`;
- `P/ui/settings.tsx`: confirm or reject bindings, the inactive delegation names, and the
  legacy drain count.

**Tests:**
- the sort order;
- pinning;
- free-form cards;
- every display reason;
- the override marker;
- the legacy group renders a steps ask and a machine ask;
- refresh on the realtime event and on the fallback interval;
- confirming a binding is an RPC that mk's Home session can call. Vizier's `rule` path
  cannot reach it, which the test asserts by route.

**Verify:** `cd P && npm test -- __tests__/ui.test.tsx __tests__/queue.test.ts && npm run typecheck && bb plugin build`.

### Task 2.7: retire spawns, legacy-scope the lifecycle commands, migrate instructions

**`P/cli.ts`:**
- `ask` exits 2, "moved".
- `progress`, `resolve` and `withdraw` accept **legacy ids only**.
- `get` gains `--card` and `--request`. `--request` resolves through `card_requests`, then
  through the rev-4 `decisions.request_id`.
- `list` gains `--pull mycroft`, which includes legacy asker-mycroft rows.

**Server code:**
- **`P/serve.ts`:** remove the `ServeSupervisor` spawn and its import (`serve.ts:4`).
  `P/server.ts` is unwired to match.
- **`P/asks.ts`:** kept and guarded. Every entry point refuses a row with
  `task_id IS NOT NULL`.

**`P/feed.ts`:**
- The instruction at `:154` names `autarch needs-mk file`.
- Own-answer selection (`:118`) becomes:
  - `d.thread = @thread`;
  - OR an `EXISTS` match on `decision_blocks` for `'thread:' || @thread`;
  - OR, for legacy rows only, the existing `mentions` join.

**Docs:** `skills/home/SKILL.md`, `README.md`, `PLUGIN_OVERVIEW.md` and `AGENTS.md` teach
the card filer. Legacy lifecycle commands are documented only as "for asks filed before
cards".

**Static checks:**
- **`no-exec.test.ts`:**
  - It uses the TypeScript compiler API over the server sources, that is everything outside
    `ui/`, `__tests__/`, `e2e/` and `scripts/`.
  - It fails on a static or dynamic import or `require` of `child_process`,
    `node:child_process`, `execa`, `node:worker_threads` or `node:cluster`, and on any use
    of `Bun.spawn` or `process.binding`.
  - Regexp `.exec()` and SQLite `.exec()` calls do not trigger it.
  - A positive fixture proves the check works.
- **`write-scope.test.ts`:**
  - fs write APIs are allowed only in `ruling.ts` and `export.ts`;
  - `rootrun.ts` may only open files `O_RDONLY`.

**Tests:**
- the CLI messages and the legacy-only guard;
- `get --card` and `get --request`, including for a legacy key;
- the feed text, and a Blocks-ref thread receives the ruling;
- a grep finds no live teaching of `bb home ask` in the docs;
- the static checks.

**Verify:** `cd P && npm run typecheck && npm test -- __tests__/no-exec.test.ts __tests__/write-scope.test.ts __tests__/feed.test.ts __tests__/cli.test.ts && bb plugin build`,
then rev-4 criteria 1 and 12.

### Task 2.8: root-run display

**Files:** `P/rootrun.ts`, `P/ui/rootrun.tsx`, `P/__tests__/rootrun.test.ts`, and
`contract.ts` (`rootRun`).

**Tests:**
- the four states;
- the item JSON keys;
- `owner_thread` comes from `askingThread()`;
- every hostile vector renders no command and names the field;
- an incomplete tuple renders no command;
- a valid tuple has every argument quoted;
- `shQuote` refuses `'`, control characters and non-ASCII;
- no approval wording.

**Verify:** `cd P && npm test -- __tests__/rootrun.test.ts __tests__/no-exec.test.ts __tests__/write-scope.test.ts`.

### Task 2.8a: rollback and cutover acceptance (A10, A11)

This task runs after 2.4–2.8, because it asserts on `queue` (2.6), write-back (2.5) and the
CLI (2.7). That ordering is the fix for finding r3-7.

It uses a temporary `git worktree` under `/tmp` at a9853e2 (not a stash) to build and drive
the v2 plugin through its real service and CLI entry points.

**Files:** new `P/__tests__/rollback.test.ts`, new `P/__tests__/cutover.test.ts`.

**Tests:**
1. **[ROLLBACK-ONLY]** **Rollback (A10, findings r2-1, r3-1, r4-1).** Start from a v3 DB holding a card
   generation g1. Then:
   1. start the v2 build;
   2. v2 makes a vizier `rule` on g1;
   3. v2 runs `override` on g1, which creates a `dec_…` row with `source='home'` and
      `task_id` NULL;
   4. an agent's v2 `bb home ask` replaces a second card's g1 (the rollback-replace case);
   4a. **forged override (finding r4-1):** an agent's v2 `bb home ask` replaces a third
       card's g1 with `request_id = "override:<card-…-g1>"` and `supersedes` that g1,
       copying g1's body; v2 accepts it as an ordinary replace;
   5. drain the v2 worker once, using a recording SDK;
   6. reopen the DB under v3.

   Assert:
   - no v3 trigger aborted any v2 write;
   - the v2 drain sent only rev-4 kinds, and every card row parses under v2;
   - after reopen:
     - the `dec_…` override is adopted as g2 (`source='card'`, `task_id`, `generation=2`);
     - the first card is `open`, and g2 appears in `queue` as "overrides vizier ruling g1";
     - `card_writes` are back-filled for g1, and `decision_blocks` and `card_fp` are
       copied to g2;
   - the rollback replace is **not** adopted. It appears in the legacy group, and the second
     card is display-only "replaced during rollback" (T13);
   - the forged override is **not** adopted (no vizier pick on g1; its event is
     `replaced`): it stays `source='home'` in the legacy group, and the third card is T13;
   - a second open changes nothing.
2. **Cutover (A11, finding r2-5).** The fixture is a populated v2 DB, created only through
   the a9853e2 build's own entry points (finding r3-7: `asks.ts:133` allows `progress` only
   on `machine` asks). It holds:
   - an open thread decide ask with mentions;
   - an open mycroft decide ask;
   - an open steps ask (no progress);
   - an open machine ask **with progress**, recorded by its owner thread;
   - a picked ask whose ruling file is still pending;
   - a delegated, vizier-picked ask;
   - a withdrawn ask.

   Reopen it under v3. Every outstanding ask is in `queue` under the legacy group with its
   rev-4 rendering data, and the withdrawn ask is not. In addition:
   - the machine ask's progress event is shown;
   - `progress` (machine ask only), `resolve` and `withdraw` work on legacy ids and refuse
     card ids;
   - reconcile writes the pending ruling file;
   - the mentions feed still reaches the mentioned thread;
   - an override of the vizier pick works;
   - `list --pull mycroft` includes the legacy mycroft ask;
   - the drain counter equals the number of open legacy asks.

**Verify:** `cd P && npm test -- __tests__/rollback.test.ts __tests__/cutover.test.ts`.

### Task 2.9: CardFiler and `autarch needs-mk file` (mk-okek.8)

ExecFiler is **kept** in this task (finding r2-10).

**Files:**
- new `cmd/autarch/needsmk.go`, `needsmk_test.go`;
- `internal/homeask/filer.go`: adds `CardFiler`, `FileForPull` and `CardLister` next to
  `ExecFiler`.
- `P/cli.ts`: `get --request` returns exactly one of `registered`, `legacy` or `absent`,
  with exit 3 when not ready.

**Inputs:**
- `--project`, `--title`, `--blocks` (repeatable), `--ask-file`;
- `--root-run script=…,timeout=…,set=…`, where the filer hashes the named file and validates
  `set`;
- `--request`.

`BB_THREAD_ID` is required.

**Tests:**
- the exact argv sequence, with the lock held;
- **Pagination (finding r2-9).** An exact match is on page 3 behind 1000 substring matches,
  so no card is created. A failure on page 2 exits 3 with no create.
- **Replay (finding r2-8).**
  - Comments from A then B, replayed from B → exit 2, naming A. Replayed from A → exit 0.
  - With no comment, the filer posts one before exiting 0 (order asserted).
  - Replays after unlabel or `done` succeed.
  - A substring-only hit is not a match.
  - An identity mismatch exits 2. A deleted card exits 2.
- **Registry authority (finding r3-5).**
  - A registered card is deleted, then Home is made unavailable: the filer exits 3, and no
    card is created.
  - A registered card has its `Request:` line edited away, then Home is made unavailable:
    the filer exits 3 with no create.
  - The same two cases with Home available: exit 2, "registered to card <id>", with no
    create.
  - Home unavailable, with an exact match visible in tasks search: exit 3. The search is
    never used without a registry answer.
  - Registry `absent`, and a card deleted before it was materialized: creates a new card,
    exit 0.
  - Registry `legacy`: exit 2.
- **Key first, identity second (finding r4-2).** Registry `absent` in every case:
  - an unmaterialized card has the same Request key and a **different** identity token:
    exit 2, "Request reused for a different card", and the create call is never made
    (argv asserted);
  - two such cards, the earlier one with a different token and the later one with the
    matching token: the earlier is canonical, so exit 2 and no create;
  - the same key appears only on an unparseable card: exit 2, no create;
  - the key appears only as a substring of a longer key: not a match, so the card is
    created.
- **Concurrency:** two concurrent filers using real `flock` produce one card.
- **Errors:** exit 4 when the comment fails, and the rerun repairs it; exit 3 when
  unreachable.
- the description round-trips;
- `FileForPull` skips filing when the legacy key is open.

**Verify:** `go build ./... && go test -race ./internal/homeask/ ./cmd/autarch/ -run 'CardFiler|NeedsMk|FileForPull' && (cd P && npm test -- __tests__/cli.test.ts)`.
This compiles, because ExecFiler still exists.

### Task 2.10: move the Go callers, then delete ExecFiler

**Inventory (finding r3-2).** This was found with
`grep -rn ExecFiler --include='*.go' .` over the **whole repository** at a9853e2:

| Site | Kind | Action |
|---|---|---|
| `cmd/autarch/decide.go:22-23` (`newFiler`) | constructor | Returns `*homeask.CardFiler` |
| `cmd/mycroft/main.go:94` | constructor | `CardFiler` plus `CardLister` in `buildOrchestrator` |
| **`pkg/mcp/handlers.go:624`** | constructor (default when `s.filer == nil`) | Defaults to `&homeask.CardFiler{}`. Server field `pkg/mcp/server.go:22` keeps the `homeask.Filer` interface and its comment changes to "nil means the card filer" |
| `internal/homeask/filer.go:49-206` | definition and methods | Deleted last |
| `internal/homeask/filer_test.go:49-50` (`newFiler`) and its ExecFiler tests | tests | Recovery tests are deleted with the type. The CardFiler tests from 2.9 cover the replacements |

`cmd/autarch/mcp.go` has no ExecFiler reference of its own. It reaches the filer through
`pkg/mcp`.

**Files:**
- `cmd/autarch/decide.go`;
- `pkg/mcp/handlers.go`, `pkg/mcp/server.go`, and the tests in `pkg/mcp` that reach the
  default filer (asserting a CardFiler call sequence through a fake `bb`);
- `cmd/mycroft/main.go`;
- `internal/mycroft/escalate/escalate.go`;
- **then** delete `ExecFiler` from `internal/homeask/filer.go` and its tests from
  `filer_test.go`.

**Mycroft:**
- Files with `FileForPull`. The Request is the UUIDv5 of the legacy key `mycroft:<project>:<bead>:<agent>`.
- Reads outcomes with `bb home get --card` and `list --pull mycroft`, which also cover
  legacy rows.

**DecisionQueue:**
- Remove `decisions []PendingDecision` and `nextID` (`escalate.go:73-74`), and the
  local-only branch in `AddPending` (`:261-263`).
- `Get`, `All`, `Len` and `HighestSeverity` come from `CardLister`.
- `Remove` becomes a documented no-op.
- Tests use a `_test` `MemoryFiler`.

**Production-wiring test:** `buildOrchestrator`'s queue holds a CardFiler.

**Verify:** `go build ./... && go vet ./... && go test -race ./cmd/autarch/ ./cmd/mycroft/ ./pkg/mcp/... ./internal/mycroft/... ./internal/homeask/ && ! git grep -nE 'ExecFiler|decisions[[:space:]]+\[\]PendingDecision' -- '*.go'`.
The grep covers every tracked Go file in the repository, not only `cmd/` and `internal/`.

### Task 2.11: fake harness and the shared rig wrapper

**Files:**
- `P/e2e/fake-bb.mjs` (tasks fake);
- new `P/e2e/rigexec.ts` (`rigExec`, `rigRpc`, the `bb` PATH shim);
- `P/e2e/rig.ts` and `real.ts`, whose spawns at `rig.ts:44,213` and in `real.ts` move behind
  `rigExec`;
- `scenarios/index.ts`, `scripts/check-e2e.mjs`, tests.

**Wrapper (A7):**
- **Environment.** The wrapper builds the child environment from nothing:
  - `PATH` = the shim dir, then `/usr/bin:/bin`, then the directories of the resolved `bb`
    and `node`;
  - `HOME`, `BB_SERVER_URL`, `BB_DATA_DIR` and `BB_HOST_DAEMON_PORT` point at the rig;
  - `BB_THREAD_ID` is passed only when a scenario sets it.
- **RPC.** `rigRpc` is the only e2e RPC path, and it calls `assertOwned()` first.
- **Shim.** The shim execs the real `bb` only when the URL and the data-dir nonce match the
  rig. Otherwise it exits 97 and logs the call.
- **Refusal.** A target is refused when it:
  - is in the loopback class (`localhost`, `127/8`, `::1`, `::ffff:127.0.0.1`,
    `0.0.0.0`) and uses any ambient port. Ambient ports come from env and from every
    `bb-app-runtime.json` under `~/.bb` and `<bb-data-dirs>`;
  - is not loopback;
  - has no environment, because there is no default.

**Existing scenarios: every one is kept or mapped (finding r3-8).** The rev-4 set is in
`e2e/scenarios/index.ts` (fake) and `e2e/real.ts` (real), and `scripts/check-e2e.mjs:19-36`
holds a schema for each.

| Rev-4 scenario | Disposition | Behaviour kept | New evidence |
|---|---|---|---|
| `answer-instruction` | **Kept**, on a card g1 | pick → ruling file → one wake → feed line | Adds `card_id`, `comment_by` |
| `pick-retry` | **Kept** | Same `pick_id` replayed → one pick, one wake | Unchanged |
| `crash-after-pick` | **Kept** | Crash after the pick commit → ruling and wake recovered | Adds a pending `card_writes` count of 2 |
| **`two-writers`** | **Kept** | Two processes pick the same generation at once → statuses `[201, 409]`, one pick | Unchanged schema, decision is a `card-…-g1` |
| `file-retry` | **Mapped** to `card-file-retry` | The create response is dropped after commit; the filer recovers in the same run (lookup again under the lock) → exit 0, one card | `{card, exit_code: 0, cards: 1}` |
| `file-unknown` | **Mapped** to `card-file-unknown` | The agent-comment response hangs after the card is created → exit 4 (outcome unknown, the Task 2.9 error code); a rerun finds the registered card and repairs or confirms the comment → exit 0, one card, one agent comment | `{card, first_exit: 4, rerun_exit: 0, cards: 1, agent_comments: 1}` |
| **`not-ready-at-start`** | **Kept** (filer form) | Store locked at start: `bb home get --request` reports not ready, the filer **exits 3 and creates no card** (r3-5), and `queue` rejects "not ready". After the lock is released and ready: the filer succeeds, the card materializes, and the pick returns 201 | `{ask_exit_locked: 3, cards_while_locked: 0, not_ready: true, decision, pick_id, seconds}` |
| `supersede` | **Mapped** to `card-edited-before-pick` | A pick on the superseded generation → 409, and the replacement is owed. Rev-4 "second replacement of an already-replaced decision → 409" has no card form, because edits always chain from the latest generation. That refusal stays covered by the unchanged rev-4 `store.test.ts` `insertReplacement` test | `{g1, g2, pick_g1_status: 409, owed_g2: true}` |
| `ruling-file-blocked` | **Kept** | | Unchanged |
| `queued-then-archived` | **Kept**, fake **and real** | A wake to an archived thread is undeliverable and the card is still listed | Unchanged |
| `delegated-override` | **Kept**, on cards | Override generation, void notice, supersedes in wake and feed | Adds `new_generation: 2` |
| `ask-cli-proxy` (real) | **Mapped** to `filer-from-thread` (real) | Thread identity carried through the real CLI proxy: the agent comment's `threadId` is the scratch thread; with `BB_THREAD_ID` absent the filer exits 2; replay from another thread exits 2 | `{threads, card, comment_thread_matches: true, env_absent_exit: 2, other_thread_replay_exit: 2}` |
| **`vizier-chat`** (real) | **Kept** in the real list | Unchanged Playwright check | Unchanged |

**Scenarios (fake), in full:**
- card lifecycle: `card-ingest`, `card-edited-before-pick`, `card-blocks-only-edit`,
  `card-edited-after-pick`, `card-invalidated-old-tab-pick`, `card-unlabelled`,
  `label-recreated`;
- pick and ruling write: `answer-instruction`, `pick-retry`, `crash-after-pick`,
  `two-writers`, `edit-after-pick-failed-write-restart`, `ruling-file-blocked`,
  `blocks-notices`;
- filing: `card-file-retry`, `card-file-unknown`, `not-ready-at-start`,
  `filer-registry-authority`;
- retries and archive: `comment-arrives-later`, `serve-recovers`, `queued-then-archived`;
- override and delegation: `delegated-override`, `override-unlabel-crash`,
  `project-mismatch-delegation`;
- Request handling: `duplicate-request-reversed`;
- root run: `root-run-display`, `root-run-injection`.

Migration acceptance lives in Task 2.8a's unit-level tests, because it needs the a9853e2
worktree build and not a fake bb. `check-e2e.mjs` gains a schema for every new or mapped
name, and the four retired names are removed from `SCHEMAS`. A check-e2e test asserts that
each retired name is listed in this table.

**Tests:**
- every refusal happens with no socket opened (a `net.connect` spy);
- an indirect `autarch needs-mk file` against an ambient port is refused by the shim;
- an import-graph test using the Task 2.7 checker: under `e2e/`, only `rigexec.ts` may spawn
  processes or call `fetch`/`callRpc`.

**Verify:** rev-4 criterion 13's command with this `--scenarios` list, plus
`cd P && npm test -- __tests__/rigexec.test.ts __tests__/check-e2e.test.ts`.

### Task 2.12: real-bb run on a harness-owned server

#### Rig (A7, finding r2-4)

The harness launches the server itself. The only operator input is `HOME_E2E_BB_APP`.

1. **Setup.**
   - Create `mkdtemp` data, `HOME` and runtime directories.
   - Pick two ports with `freePort()`.
   - Write a nonce to `<data>/.home-e2e-nonce`.
2. **Launch.** Run `node $HOME_E2E_BB_APP/… start --data-dir <data>` through `rigExec`, with
   `BB_SERVER_PORT` and `BB_HOST_DAEMON_PORT` set. Keep the child pid `L`.
3. **Socket ownership, before any request to the port.**
   1. Wait until `/proc/net/tcp` and `/proc/net/tcp6` list a socket in state `0A` (LISTEN)
      on the server port, and record its inode `I`.
   2. Compute the **owned set `O = {L} ∪ descendants(L)`** by walking `/proc/*/stat` ppid
      fields (finding r3-4). `L` is included because the launcher may serve the port itself.
   3. Collect `S`, the inodes of every `socket:[N]` link under `/proc/<p>/fd/*` for each
      `p ∈ O`, and remember which pid holds each inode.
   4. **Require `I ∈ S`.** The pid holding `I` is recorded as `socket_owner_pid`. It may
      equal `L` or be any descendant; the two identities are kept separate.
   5. Require that no other LISTEN entry exists on that port.

   If any check fails, refuse and stop the launcher. `/proc/net/tcp` is only used to map
   port to inode. It says nothing about which process owns the socket, so ownership comes
   from the fd links.
4. **Runtime cross-check (launcher identity).** `<data>/bb-app-runtime.json` records the
   **launcher's own pid** (`claimBbAppRuntimeFile`, `packages/bb-app/src/launcher.ts:3715`,
   `pid: process.pid`). The harness requires that pid to equal `L` exactly, and requires the
   port to equal the chosen port. The runtime file is never used to identify the socket
   owner.
5. **Nonce round-trip.** Only after steps 3 and 4 pass does the harness use `rigRpc` to
   create a tasks project named `e2e-<nonce>`. It then opens
   **`<data>/plugins/tasks/data.db`** read-only (`file:…?mode=ro`). That is the per-plugin
   DB (`packages/plugin-sdk/src/backend-contract.ts:183`), and it was confirmed on the live
   machine at `<bb-data-dir>/plugins/tasks/data.db`. The harness requires
   a project row with that name. This proves that the URL and the data dir belong to one
   server.
6. **Teardown.**
   - `bb-app stop --data-dir <data>` through `rigExec`.
   - Every pid in `O` (recomputed just before the stop, so late children are included) is
     gone within 10 s; otherwise `SIGKILL` and record it.
   - The evidence record `evidence.rig` holds `launcher_pid` (`L`), `runtime_file_pid`,
     `socket_owner_pid`, `listen_inode`, `listen_inode_in_owned_set`, `owned_set`, and the
     stop result.

#### Scenarios (real)

- `filer-from-thread`: replaces rev-4 `ask-cli-proxy` (see the Task 2.11 table). It
  includes replay after unlabel, `env_absent_exit: 2` and `other_thread_replay_exit: 2`.
- `poller-refresh`: the panel updates without a reload.
- `comment-arrives-later-real`.
- `cross-project-panel`.
- `answer-instruction`: the label is removed, the ruling file is written through a
  `card-…-g1` id, and the comment says `ruled by: mk`.
- `queued-then-archived`: kept from the rev-4 real list.
- `vizier-chat`: kept from the rev-4 real list, unchanged.

The default list at `e2e/harness.ts:42` is changed to exactly these seven names, so a
plain `--mode real-bb` run cannot silently drop one.

#### Replacement for criterion 14 (finding r2-11)

The flags `--owned-server` (`e2e`) and `--require-ownership` (`check-e2e`) are added in this
task. `check-e2e` fails a record that has no `evidence.rig` block.

```bash
test -n "$HOME_E2E_BB_APP" && test -f "$HOME_E2E_BB_APP/package.json" \
 && cd integrations/bb-plugin-autarch \
 && npm test -- __tests__/build-identity.test.ts __tests__/rigexec.test.ts \
 && rid=$(node -e 'console.log(crypto.randomUUID())') && d=$(mktemp -d) \
 && b="$d/build.json" && e="$d/e2e.jsonl" \
 && node scripts/build-identity.mjs --out "$b" \
 && npm run e2e -- --mode real-bb --owned-server "$HOME_E2E_BB_APP" --build "$b" --install --run-id "$rid" --out "$e" \
 && node scripts/check-e2e.mjs "$e" --mode real-bb --require-ownership --run-id "$rid" \
      --product-commit "$(git rev-parse HEAD)" --build "$b" \
      --scenarios filer-from-thread,poller-refresh,comment-arrives-later-real,cross-project-panel,answer-instruction,queued-then-archived,vizier-chat \
 && npm test -- __tests__/rig-acceptance.test.ts \
 && jq -se -f scripts/rig-acceptance.jq "$e"
```

**`scripts/rig-acceptance.jq`** (finding r4-6). The expression lives in one file, so the
command and its test cannot drift. Each record binds its rig evidence to `$r` first, so no
path is evaluated against the wrong input:

```jq
length > 0
and all(.[];
      .evidence.rig as $r
      | ($r | type) == "object"
        and $r.listen_inode_in_owned_set == true
        and ($r.launcher_pid | type) == "number"
        and $r.launcher_pid == $r.runtime_file_pid
        and ($r.owned_set | type) == "array"
        and any($r.owned_set[]; . == $r.socket_owner_pid)
        and $r.nonce_in_tasks_db == true
        and $r.stopped == true)
and ([.[].evidence.threads[]] | length) > 0
and all(.[]; .evidence.cleanup.archived == .evidence.threads)
```

**`P/__tests__/rig-acceptance.test.ts`** runs the real `jq -se -f scripts/rig-acceptance.jq`
binary on fixture files and asserts the exit status:
- **valid evidence** (two records; socket owner equals the launcher in one and is a
  descendant in the other) → exit 0, with no jq error on stderr;
- **invalid evidence**, one fixture per case, each → non-zero exit:
  - `socket_owner_pid` not in `owned_set`;
  - `launcher_pid` ≠ `runtime_file_pid`;
  - `listen_inode_in_owned_set` false;
  - `evidence.rig` missing, or `owned_set` not an array;
  - `stopped` false; `nonce_in_tasks_db` false;
  - no threads; archived threads differ from created threads;
  - an empty file.

It also asserts that a jq **runtime error** (for example the rev-5.3 form, `Cannot index
array with string "evidence"`) is reported as a failure of the test, never as a pass.

#### Report

Write `docs/research/2026-10-xx-home-on-tasks-e2e.md` and file follow-up beads. Nothing is
pushed.

### New acceptance criteria (in addition to the CHANGES rows)

- **A1.** Parser vectors pass in Go and TS. This includes `toV1()`, the hostile `set`
  values and `askingThread` (Task 2.1).
- **A2.** The poller:
  - publishes only on change;
  - retries unresolved cards independently of the digest;
  - withdraws nothing when tasks is unavailable or a read is partial (Task 2.4).
- **A3.** The sort, the panel, the legacy group and the binding settings work (Task 2.6).
- **A4.** Root-run is display-only:
  - plugin server code has no process execution (import and call analysis);
  - the write scope is `ruling.ts` + `export.ts` + `bb.storage`;
  - there is no approval wording;
  - no command is rendered for an invalid or incomplete tuple (Tasks 2.7, 2.8).
- **A5.** The filer is idempotent by exact Request key, then identity (r4-2):
  - the key is matched first; the earliest card with that key is canonical; a differing
    identity or an unparseable card carrying the key exits 2 before any create;
  - Home's registry is authoritative: no card is created unless `bb home get --request`
    answers `absent`; unreachable or not ready → exit 3 with nothing created; a registered
    card that was deleted or edited away → exit 2 (r3-5);
  - across unlabel and status changes;
  - across fully paginated search;
  - under concurrent filers;
  - with the same earliest-agent-comment routing rule as ingest.

  It requires a thread. The only exception is `FileForPull` (Task 2.9).
- **A6.** On real bb, the poller refreshes an open panel without a reload (Task 2.12).
- **A7.** Probe isolation is enforced by the harness.
  - **Owned server identity.** The harness launches the server itself.
    - Before any request, the LISTEN socket's inode on the server port must be among the
      `/proc/<pid>/fd` socket links of the owned set `{L} ∪ descendants(L)`, and it must be
      the only LISTEN socket on that port. Its holder is recorded as `socket_owner_pid`.
    - `bb-app-runtime.json` must name exactly the launcher pid `L` (it records
      `process.pid`, `launcher.ts:3715`) and the chosen port.
    - Teardown confirms every pid in the owned set is gone.
    - A nonce tasks project created through the URL must then appear in
      `<data>/plugins/tasks/data.db`, opened read-only.
  - **One wrapper.** Every e2e spawn and RPC goes through `e2e/rigexec.ts`. It builds the
    environment from nothing, and the PATH shim refuses indirect `bb` calls that do not
    target the rig.
  - **Refusals.** These are tested with no socket opened:
    - the ambient URL;
    - each loopback alias on an ambient port;
    - an absent environment (there is no default);
    - an indirect autarch → bb call.
  - **Import-graph test.** Nothing under e2e spawns or calls RPC outside the wrapper. Plugin
    server code never spawns (A4).
- **A8.** Reply routing:
  - the filer never reports success until the asking thread's comment exists;
  - the asking thread is always the earliest agent comment;
  - a card without one is shown, retried and not materialized, as "unroutable";
  - the only exception is `pull:"mycroft"`, shown as "pull: mycroft (unverified)".
- **A9.** Ruling content and lineage are immutable.
  - A card edit after a pick never changes the ruling file, the feed or the catch-up.
  - An invalidating edit before a pick withdraws the stale generation, so old tabs cannot
    pick it.
  - Triggers enforce this (Tasks 2.3, 2.4).
- **A10.** **[ROLLBACK-ONLY]** Rollback is safe. The a9853e2 build runs on a v3 DB, including pick and override
  on a card. After re-upgrade, its override is adopted as the next generation and is
  visible (Task 2.8a rollback test; trigger and adoption unit tests in Task 2.3).
- **A11.** Cutover is lossless. A populated v2 DB keeps every outstanding ask visible and
  actionable in the legacy group, including steps and machine asks; progress appears only on
  the machine ask, as `asks.ts:133` allows (Task 2.8a cutover test).
- **A12.** Delegation scope equals ruling scope:
  - a vizier ruling requires a confirmed tasks → Home binding;
  - the card's `ask.project` must equal that binding;
  - existing name-based settings are re-validated, not reinterpreted (Tasks 2.3, 2.5).

---

## 4. Bead re-scoping

These descriptions are for mk or a later session to apply. The hub Dolt was not touched.

- **mk-okek.8** becomes "needs-mk card filer: `autarch needs-mk file`" (Tasks 2.9 and 2.10).
  The Stop hook stays with G-12.
- **mk-okek.16** becomes "Home on tasks: state machine, poller, queue, blocks panel,
  root-run display, legacy lane" (Tasks 2.3–2.8, 2.11 and 2.12).
- **New follow-up**, retire legacy asks code: remove `asks.ts`, the lane/runbook UI and the
  legacy-only CLI. Trigger: 0 open legacy asks for 14 days.
- **mk-okek.15** can be closed.
- **mk-okek.9–.14** are unchanged.

## 5. Gates

- **G-10** (overlay): last.
- **G-4** (republish): mk.
- **G-6** (walk): mk, after Task 2.12.
- **G-0, G-1, G-3, G-9, G-11–G-16:** as in rev 4.
- **G-15:** an asking thread taken from an agent comment is only as strong as
  `BB_THREAD_ID`. A forged `pull:"mycroft"` gives no authority.
- **CI prerequisite (corrected in rev 5.3, r3-9):** as in rev-4 G-0, the repository id is
  `1140086114`, the canonical registry name is `mistakeknot/Autarch`, and the migration task
  is `mk-ag2s.18`. A fresh canonical lookup on 2026-10-01 agrees: `gh api
  repos/mistakeknot/Autarch` returns id 1140086114, and `<host>-ci status` lists it with
  campaign mk-ag2s, disposition `pending-inventory`, inventory `requires-workflow-review`,
  enabled. The rev-5.2 "not registered" claim came from the lowercase lookup that rev 4
  already called misleading, and it is withdrawn. Claiming mk-ag2s.18 needs the hub, which
  this session may not touch, so it stays with mk. The migration remains outstanding.
- **Review gate:** two other-frontier rejects means escalation is requested. Rev 5.4 has
  not been reviewed; four rejects so far (12, 11, 9, 6 findings). The rollback model (Q7)
  is an open mk/vizier decision. Nothing is pushed or published.

## 6. Fork changes (Aleph fork first, non-blocking)

| # | Change | Removes |
|---|---|---|
| F-1 | Cross-plugin realtime subscription | The Home poller |
| F-2 | Structured `blocks`/`request` fields with exact-match filter | Description parsing, the search pre-filter, and pagination of substring hits |
| F-3 | Label-name filter and label names in output | The label cache and the `createLabel` step |
| F-4 | Notify to the attached thread; comment events bump a cursor | The mandatory routing comment and the comment retry set |
| F-5 | Attested comment author on RPC `createComment` | Mirror comments that look like mk's |
| F-6 (new) | Immutable `createdBy` (thread or plugin) on tasks cards | Earliest-comment routing as the asker proof |

## 7. Open questions for mk

1. **D-1, root-run status read.**
   - (a) Aleph writes a status file and Home reads it. This is recommended.
   - (b) A paste command only. This is the default until you rule.
   - (c) Home execs the status command. Excluded unless you lift the no-exec rule.
2. **Mirror writes to cards.** May Home write to cards: remove the label and post a comment
   after a pick? If so, may it also re-add the label on an override, so the tasks board
   shows the new generation?
3. **Blocks count.** Should it count transitive blocking, or direct refs only? The current
   design counts direct refs only.
4. **Mycroft threadless exception.** This is the forgeable, authority-free
   `pull:"mycroft"` marker. Do you accept it, or should Mycroft file from a dedicated thread
   it owns?
5. **(new) Project bindings.** Delegation needs you to confirm each tasks project → Home
   project binding once in Settings. Name-matched bindings are suggested only, and allow mk
   picks but not vizier rulings. Do you accept this one-time confirmation step?
6. **(new) Legacy asks.** Open asks from before cards drain in place, with no card. They
   are retired 14 days after the count reaches 0. Do you accept this, or should open legacy
   asks be force-closed at cutover?
7. **(new, rev 5.4) Rollback model (§1.3.9).** (a) Forward-only v3: a `VACUUM INTO` backup
   before migration, `min_reader_version = 3` fences the v2 build, and rollback restores the
   backup, losing post-migration Home state. (b) Compatible rollback with verified override
   adoption (current). Every (b)-only item is marked [ROLLBACK-ONLY] for clean deletion. No
   default is forced; the plan executes (b) unless you rule (a).

---

## 8. Review disposition

### 8.1 Round 1 (Astra, Reject, 12 findings): all fixed in rev 5.1

The table below shows where each fix lives now, after rev 5.2. Rev 5.1 had marked finding
r1-2 as "narrowed": rev 4 never updated `body_json`, but the risk came from rev 5's wording.

| # | Finding | Verified at | Fix (current) | Test |
|---|---|---|---|---|
| r1-1 | `card:` id fails ruling filename check | `ruling.ts:164,186` | `card-<ULID>-g<n>` id; colons only in request_id | Task 2.5 #6 |
| r1-2 | Ruling reads mutable `body_json` | `service.ts:97`, `feed.ts:109,123,139`, `catchup.ts:59`, `server.ts:254`; the only UPDATEs are `asks.ts:138,173` (not body) | Immutable generations plus trigger; state machine T3/T8 | Task 2.4 snapshot; A9 |
| r1-3 | One decision per task conflicts with overrides | `delegation.ts:142-179`, `store.ts:399` | Generations; override generation (T10, T11) | Task 2.5 #5 |
| r1-4 | Lookup by label; races; substring | tasks `db/store.ts:919-921` LIKE | Registry, exact parse, identity, flock, comment repair | Task 2.9 |
| r1-5 | First-sight-only discovery | — | Retry set independent of the digest | Task 2.4 |
| r1-6 | v2 reader breaks; worker sends every kind | `store.ts:183`, `wakes.ts:137`, `model.ts:251-255` | v1 stored; `card_writes`; DUE allowlist; min_reader 2 | A10 |
| r1-7 | `set` shell injection | — | Grammars, `shQuote`, full-tuple rule | Tasks 2.1, 2.8 |
| r1-8 | Rig doesn't own its server | `real.ts:1-8,34-47` | Owned server plus wrapper (corrected in r2-4) | Tasks 2.11, 2.12 |
| r1-9 | Mycroft wiring and private list | `main.go:94`, `escalate.go:72-74` | CardFiler wiring, `FileForPull`, list removed | Task 2.10 |
| r1-10 | feed and skill teach retired commands | `feed.ts:118,154`, `SKILL.md` | Migrated | Task 2.7 |
| r1-11 | no-exec grep hits `.exec()`; write rule; ordering | `ruling.ts:129,134` etc.; `serve.ts:4` | AST check, scoped writes, reorder | Task 2.7 |
| r1-12 | Hardcoded "by: mk" | — | `pick.by` | Task 2.5 #2 |

### 8.2 Round 2 (Astra, Reject, 11 findings): all accepted

All 11 findings were verified against a9853e2 and the bb sources. None is rebutted.

| # | Finding | Verified at | Disposition and plan change | Test |
|---|---|---|---|---|
| r2-1 **[ROLLBACK-ONLY]** | v2 override of a card generation leaves `task_id` NULL; re-upgrade only back-fills | Rev-4 `delegation.ts:154-176` builds the row with no card columns; `service.ts:88` mints `dec_…` | **Fix.** Adoption at v3 open (§1.3.8): a supersedes chain reaching a card generation → link-once update to the next generation; state recomputed; writes and blocks back-filled | Task 2.8a rollback test (moved from Task 2.3 test 6 in rev 5.3); A10 |
| r2-2 | Malformed edit after materialization leaves the stale generation pickable | Pick gate `store.ts:494-503` checks only that row (withdrawn, resolved, superseded, picked, revision) | **Fix.** T4: an invalidating edit withdraws the unpicked generation, so the existing gate refuses it in v3 **and** in a rolled-back v2. T5 reopens as a new generation | Task 2.4 T4 old-tab tests (three causes); scenario `card-invalidated-old-tab-pick` |
| r2-3 | Crash between unlabel and its acknowledgement can withdraw the override | Rev 5.1 rule depended on `home_unlabelled_at` | **Fix.** Label-loss withdrawal requires that no generation has a pick. The pick is committed in the same transaction that enqueues the unlabel. Override generations are also immune (T11) | Task 2.4 crash-window tests (lost response, restart before and after override); scenario `override-unlabel-crash` |
| r2-4 | Tasks data is in `<data>/plugins/tasks/data.db`; `/proc/<pid>/net/tcp` is per namespace | `plugin-sdk/src/backend-contract.ts:183`; live `<bb-data-dir>/plugins/tasks/data.db` | **Fix.** LISTEN inode ∈ fd socket inodes of the owned set, checked before any request; runtime-json cross-check; then the nonce read from `plugins/tasks/data.db`. Rev 5.3 (r3-4) widens the set to `{L} ∪ descendants(L)` and checks the runtime pid against `L` | Task 2.12 rig; `check-e2e --require-ownership` |
| r2-5 | No cutover for open pre-v3 asks | v1 kinds `decide`, `steps`, `machine` (`model.ts:12,250`) | **Fix.** Legacy lane (§1.3.7): drain in place, lifecycle commands legacy-only, `asks.ts` kept and guarded, retirement bead after drain | Task 2.8a cutover test (moved in rev 5.3); A11 |
| r2-6 | Delegation on `task.projectId` while the ruling targets `ask.project` | `delegation.ts:61` (names), `:129` (`d.project`) | **Fix.** The rev-5.1 switch is withdrawn. Delegation stays on `d.project` (ruling scope) and also requires a confirmed tasks → Home binding; mismatch is display-only. Names re-validated at open; unknown names inactive | Task 2.5 #4; A12 |
| r2-7 | `cards.request_id UNIQUE` contradicts display-only duplicates | Rev 5.1 schema | **Fix.** Non-unique `cards.request_key` plus insert-once `card_requests`; earliest `(createdAt, id)` before registration | Task 2.4 registry tests (reversed order, Request edit) |
| r2-8 | Replay accepts any matching agent comment; ingest uses the earliest | Rev 5.1 §1.3 | **Fix.** One `askingThread()` selector (earliest) in TS and Go, used by filer, ingest and root-run | Tasks 2.1 and 2.9 mixed-thread tests |
| r2-9 | `--limit 500` is one page | tasks `cli/index.ts:1448` emits `nextCursor` | **Fix.** Exhaust `--cursor`; any page failure exits 3 with no create | Task 2.9 pagination tests |
| r2-10 | Deleting ExecFiler in 2.9 breaks `cmd/autarch` | `decide.go:22-23` | **Fix.** 2.9 adds CardFiler beside ExecFiler; 2.10 moves callers, then deletes it. **Rev 5.2's inventory missed `pkg/mcp/handlers.go:624`; corrected in rev 5.3 (r3-2)**: the inventory is now repo-wide and the verify is `go build ./...` | Task 2.9 and 2.10 verify commands |
| r2-11 | Criterion 14 command still needs `HOME_E2E_BB` | rev-4 plan line 2224 | **Fix.** Full replacement command in Task 2.12 | Itself |

### 8.3 Self-pass for regressions introduced by the fixes

Before finishing, I walked the state machine for generation, override, unlabel, withdraw,
poller and crash points.

| # | Regression found in the rev-5.1/5.2 design | Resolution in this plan |
|---|---|---|
| S-1 | A stored `cards.generation` counter could drift from the rows (v2 override, crash between insert and bump) | Removed. The current generation is derived as `max(generation)`. Property test in Task 2.4 |
| S-2 | Notices and the feed read live `card_blocks`, so a post-pick edit could redirect notices to other threads | Per-generation `decision_blocks` snapshot (insert-only trigger); notices and feed read it |
| S-3 | `done`, unlabel or an invalid edit could withdraw an mk-pending override generation | T11: override generations are immune to card-state withdrawal |
| S-4 | T4 withdrawal followed by a fix: `insertReplacement` refuses a withdrawn predecessor (`store.ts:420,456-459`) | T5 inserts with `supersedes` NULL; lineage is the generation order |
| S-5 | Edits while an override generation is open could create a replace generation that hides mk's decision | T3 requires that no generation has a pick; T8 covers every edit after a pick |
| S-6 | A partial paginated poll could look like "label missing" and withdraw | A partial read counts as unavailable (T12); `getTask` plus a cache-bypassing `listLabels` confirm before any T6 (rev 5.3, r3-6) |
| S-7 | Filer fallback (earliest card) can disagree with Home's registry (first materialized) | **Rewritten in rev 5.3 (r3-5).** The rev-5.2 worst case was wrong: a registered card deleted or edited while Home is down could be re-created. There is no fallback now. The filer requires an authoritative registry answer (`registered`, `legacy` or `absent`); unreachable or not ready → exit 3, nothing created. Search runs only after `absent` |
| S-8 | Mycroft card filing could duplicate an open legacy mycroft ask | `FileForPull` checks the legacy key first; `list --pull` includes legacy rows |
| S-9 **[ROLLBACK-ONLY]** | The rolled-back v2 UI ignores bindings and project mismatch | A mismatched card is never materialized, so v2 never sees it. A vizier `rule` under v2 on an unconfirmed binding is a residual, bounded to the rollback window. It is listed in §5 G-15 as an accepted residual and needs mk's acknowledgement with Q5 |

The crash points covered by tests are:
- materialization (one transaction);
- pick plus write enqueue (one transaction);
- unlabel acknowledgement (informational only);
- override plus void (one transaction, rev 4);
- adoption (one transaction, idempotent);
- the poller digest commit (after a complete read).

### 8.4 Round 3 (Astra, Reject, 9 findings, 4 P1): all accepted in rev 5.3

Each finding was checked against committed code before it was fixed. None is rebutted.

| # | Finding | Verified at | Fix | Test |
|---|---|---|---|---|
| r3-1 (P1) | Adoption is blocked by its own trigger (`'home'` → `'card'` refused) | Rev 5.2 trigger allowed only NULL → value; v2 rows are inserted with `source` default `'home'` | `decisions_card_link` allows `'home'` → `'card'` only for an unlinked override row (`request_id = 'override:'‖supersedes`) whose predecessor is a card generation with the same `task_id` and `generation = p.generation + 1`. A `'home'` row may never carry a link column (UPDATE or INSERT) | Task 2.3 test 4 (transition table), test 6; Task 2.8a rollback |
| r3-2 (P1) | `pkg/mcp/handlers.go:624` constructs ExecFiler | `handlers.go:624` `f = &homeask.ExecFiler{}`; `server.go:22` comment | Repo-wide inventory in Task 2.10; default becomes `CardFiler`; verify is `go build ./...` plus `go test ./pkg/mcp/...` and a `git grep` over every `*.go` | Task 2.10 verify |
| r3-3 (P1) | H3 `revision` excludes Blocks, so a Blocks-only edit made no generation | `model.ts:463` | New H5 `card_fp` over the whole card; H5 alone drives T3; `decision_blocks` is per generation | Task 2.4 H5 tests (Blocks-only, title-only, root-run-only, pick on g1 refused); Task 2.5 #1 notices |
| r3-4 (P1) | The runtime file records the launcher's own pid, outside `D` | bb `packages/bb-app/src/launcher.ts:3715` `pid: process.pid` | Owned set `O = {L} ∪ descendants(L)`; runtime pid must equal `L`; `socket_owner_pid ∈ O` recorded separately; teardown over `O` | Task 2.12 rig; criterion-14 jq (`listen_inode_in_owned_set`, `launcher_pid == runtime_file_pid`) |
| r3-5 | Filer fallback re-creates a deleted or edited registered card while Home is down | Rev 5.2 §1.3.1 fallback searched tasks when Home was unavailable | No fallback. Only `absent` from `bb home get --request` permits search and create; unreachable or not ready → exit 3; registered but deleted or changed → exit 2 | Task 2.9 registry-authority tests (both cases, Home down and up); scenario `not-ready-at-start` |
| r3-6 | A stale cached label id falsely withdraws after delete, recreate, reapply | `getTask` returns label ids only | Label cache maps name → all ids; before any label-loss T6, a cache-bypassing `listLabels`; a failed read is T12 | Task 2.4 label tests (recreate/reapply, genuine removal, failed read, duplicate names) |
| r3-7 | Task 2.3 tests need 2.4–2.7; steps-ask progress impossible through v2 | `asks.ts:133` permits progress only on `machine` | Task 2.3 is store-level only; integration rollback and cutover move to new Task 2.8a; progress fixture is the machine ask | Task 2.3 tests 1–7; Task 2.8a |
| r3-8 | Scenario lists dropped `two-writers`, `not-ready-at-start`, real `vizier-chat` and others | `e2e/scenarios/index.ts`, `harness.ts:42`, `check-e2e.mjs` SCHEMAS | Every rev-4 scenario kept or mapped in the Task 2.11 table; real list and `harness.ts:42` default include `queued-then-archived` and `vizier-chat`; `ask-cli-proxy` → `filer-from-thread` | Criterion 13 and 14 commands; check-e2e retired-name test |
| r3-9 | CI prerequisite contradicted rev-4 G-0 | Rev-4 G-0; fresh `gh api repos/mistakeknot/Autarch` → 1140086114; `<host>-ci status` → mk-ag2s, `pending-inventory` | §5 and Task 2.0 restore the G-0 evidence and record the fresh lookup; claiming mk-ag2s.18 stays with mk (hub not touched) | None (documentation) |

### 8.5 Enumerated self-pass (rev 5.3)

**Every trigger.**

| Trigger | Table, event | Allows | Refuses |
|---|---|---|---|
| `approval_events_no_update` (existing, `migrations.ts:174`) | approval_events UPDATE | nothing | every update |
| `approval_events_no_delete` (existing, `migrations.ts:176`) | approval_events DELETE | nothing | every delete |
| `decisions_ask_immutable` | decisions UPDATE | `card_fp` NULL → value; every column not listed | `body_json`, `revision`, `identity`, `subject`, `semantic_key` changes; `card_fp` value → other or → NULL |
| `decisions_card_link` | decisions UPDATE OF source, task_id, generation, tasks_project_id | `'card'` row: NULL → value per link column. `'home'` → `'card'`: verified override adoption only **[ROLLBACK-ONLY]** (vizier-picked predecessor, `overridden` event, copied ask; r4-1) | `'card'` → `'home'`; value → other value for any link column; `'home'` → `'card'` that is a rollback replace, a forged `override:` replace, lacks the event or the vizier pick, differs in body, scope or `delegable`, has no card predecessor, has the wrong `task_id` or generation, or is out of supersedes order; a `'home'` row gaining any link column |
| `decisions_home_insert` | decisions INSERT | `'home'` rows with all link columns NULL (every v2 insert); `'card'` rows | a `'home'` row inserted with any link column |
| `card_requests` no-update / no-delete | card_requests UPDATE, DELETE | INSERT only | every update and delete |
| `decision_blocks` no-update / no-delete | decision_blocks UPDATE, DELETE | INSERT only | every update and delete |
| `cards_routing_frozen` (rev 5.4, r4-3) | cards UPDATE OF routing_mode, routed_thread | NULL → value | value → other value or → NULL |

v2 writes (`asks.ts:138,173`) touch only `updated_at` and resolution columns, and v2 inserts
never name the link columns, so no v3 trigger aborts a rolled-back build (Task 2.3 test 7).

**Every transition.** T1–T13 are in §1.3.4. Per column:
- `source`: `'home'` → `'card'` by adoption only; nothing else.
- `task_id`, `generation`, `tasks_project_id`: NULL → value on `'card'` rows; adoption sets
  all three at once; never value → other.
- `card_fp`: NULL → value (adoption copy); never changes after.
- `withdrawn_at`, `resolved_at`, `updated_at`, pick columns: rev-4 rules, unchanged.
- Card states: `observed` → `open` (T1) → `open` g n+1 (T3) / `closed` (T4, T6) → `open`
  (T5) / `ruled` (T9) → `open` override (T10); `display` from T2 and T13; T7, T8, T11, T12
  change nothing.

**Every ExecFiler and Filer construction site, whole repository** (`git grep` over all
files, 2026-10-01):

| Site | Kind | Rev 5.3 action |
|---|---|---|
| `cmd/autarch/decide.go:22-23` | `newFiler()` constructor | returns `*homeask.CardFiler` (Task 2.10) |
| `cmd/mycroft/main.go:94` | constructor used as Filer **and** Lister | `CardFiler` plus `CardLister` (Task 2.10) |
| `pkg/mcp/handlers.go:624` | default when `s.filer == nil` | `&homeask.CardFiler{}`; `server.go:22` comment updated (Task 2.10) |
| `internal/homeask/filer.go:49-206` | definition (`File`, `recover`, `List`) | deleted after callers move (Task 2.10) |
| `internal/homeask/filer_test.go:49-50` | test constructor | ExecFiler tests deleted; CardFiler tests added (Task 2.9) |

Interface-only users need no change: `internal/mycroft/escalate/escalate.go:77,187,198`,
`pkg/mcp/server.go:22,44`, and the test doubles `recFiler` (`cmd/mycroft/wiring_test.go`,
`internal/mycroft/scheduler/escalate_wiring_test.go`) and `fakeFiler`
(`internal/mycroft/escalate/home_test.go`, `pkg/mcp/decision_test.go`). Outside Go, only
the rev-4 plan docs mention ExecFiler.

**Every hash used for generation identity** (§1.3.4 table):
- H1 Request token: filer replay versus reuse only; frozen in `card_requests`. A changed
  `Request:` line is T4.
- H2 `identity`: the rev-4 `requests` row of each generation. It does not decide
  generations.
- H3 `revision` (`model.ts:463`): the pick gate only. It excludes Blocks, title and
  root-run, which is why it does not decide generations.
- H4 `semantic_key`: display only; `ingestCard` does not run the `service.ts:221-231` merge.
- H5 `card_fp`: the only generation decider (T3), immutable per generation.

**Regressions found by this pass and fixed in rev 5.3:**

| # | Found | Resolution |
|---|---|---|
| S-10 | `insertDecision` and `insertReplacement` (`store.ts:425-431`) have no card columns, so a T3 generation would lose `task_id` | Task 2.4 adds the card columns to both insert lists |
| S-11 **[ROLLBACK-ONLY]** | A v2 rollback *replace* of a card generation cannot be adopted (not an override) and would hide the card | T13: display "replaced during rollback by <id>"; the legacy row stays operable |
| S-12 | The rev-4 mention copy in `insertReplacement` (`store.ts:445-450`) | No-op for card rows (they have no mentions); asserted in Task 2.4 |
| S-13 | The P-11 semantic merge could fold two cards together | Not applied to cards (H4 display only) |
| S-14 | The rev-5.2 link trigger let a `'home'` row gain `generation` or `tasks_project_id` alone, and said nothing about INSERT | Last clause covers all three columns; `decisions_home_insert` added |
| S-15 | Deleting the earliest agent comment would change the asking thread and, through H5, open a generation that wakes a different thread | Asking-thread change is a T4 cause (withdraw, no wake); Task 2.4 T4 test |
| S-16 | With no fallback, no card can be filed while Home is down | Accepted: rev 4 also required Home to file; the filer exits 3 and the caller retries |

### 8.6 Round 4 (Astra, Reject, 6 findings, 3 P1): all accepted in rev 5.4

Each finding was checked against committed code (a9853e2) or the rev-5.3 text before it was
fixed. None is rebutted.

| # | Finding | Verified at | Fix | Test |
|---|---|---|---|---|
| r4-1 (P1) [ROLLBACK-ONLY] | Override adoption is forgeable by an agent replace with `request_id override:<pred>` | `model.ts:64,260` (`REQUEST_ID` allows `:`); `store.ts:412-415` (replace needs an unpicked predecessor); `store.ts:451` (event `replaced` vs `overridden`); `delegation.ts:142-180` (override copies body, scope, sets `delegable` false) | Adoption (§1.3.8, trigger and `adoptOverrides`) also requires a vizier pick on the predecessor, the `overridden` event naming it, `delegable = 0`, equal scope, and an equal body modulo `request_id`/`supersedes`. No other rollback machinery added; the rollback model goes to mk/vizier (§1.3.9, Q7) | Task 2.3 test 4 refusal rows (forged replace, no event, body/delegable/thread mismatch); Task 2.8a test 1 step 4a |
| r4-2 (P1) | Key-and-identity filter ignores a same-key card with a different payload and creates a duplicate | Rev 5.3 §1.3.1 step 2.4 | Match the Request key first over every search hit; the canonical card is the earliest (createdAt, id); an unparseable card carrying the raw key, or a differing identity, exits 2 before any create | Task 2.9 "Key first, identity second" (create argv never called; earliest wins; unparseable; substring is not a match) |
| r4-3 (P1) | Changing `pull` changes asker/thread, `insertReplacement` refuses on scope, old generation stays pickable | `store.ts:416-422` scope gate; `model.ts:421` `scopeNode` | `routing_mode` and `routed_thread` frozen at T1 (`cards_routing_frozen` trigger); any routing or scope change is T4; T3 requires unchanged scope and falls back to T4 in the same transaction if `insertReplacement` refuses | Task 2.4 "Routing frozen" (thread → pull, pull → thread, different earliest thread, restore → T5, forced refusal → T4, trigger rejects change); Task 2.3 test 5 |
| r4-4 | Routing invalidation not discoverable: digest excludes comments; comment retries cover only `observed` | Rev 5.3 §1.3.2 | Routing revalidation set over materialized open non-override cards, 20 per poll by oldest `routing_check_at` (about 30 s per 120 cards; worst-case age in Settings); mismatch is T4; read failure is T12 with the snapshot kept; wakes go only to the frozen `routed_thread` | Task 2.4 "Routing revalidation" (comment deleted with unchanged metadata, listComments failure, 150 cards, override excluded) |
| r4-5 | Task 2.4 verification needs Task 2.5 behaviour | Rev 5.3 Task 2.4 test list | Task 2.4 uses the fake client and store fixtures only (`recordPick`, `insertReplacement(…,"override")`); crash window, immutable snapshot and T11 end-to-end move to Task 2.5 `card-integration.test.ts` | Task 2.4 and 2.5 verify commands |
| r4-6 | Criterion-14 jq errors on valid evidence (`.` is the array inside `index`) | Reproduced locally with jq | `scripts/rig-acceptance.jq` binds `.evidence.rig as $r` and uses `any($r.owned_set[]; . == $r.socket_owner_pid)`; checked with jq: valid exits 0; foreign socket owner, missing rig, empty input exit 1 | `rig-acceptance.test.ts`: valid (owner = launcher, owner = descendant) and one invalid fixture per clause; a jq runtime error counts as failure |

**Self-pass notes (rev 5.4):**
- T3's fallback to T4 means a refused replacement can never leave the old generation open.
- Between revalidation passes, a deleted routing comment cannot misroute a wake: wakes go to
  the frozen `routed_thread`, so the window only delays the T4 withdrawal.
- S-15 is superseded by the routing freeze (a thread change is T4 by the frozen comparison).
  The H5 asking-thread input is now redundant with it but harmless.
- `cards_routing_frozen` is added to the §8.5 trigger table.
- Option (a) in §1.3.9 uses only SQL (`VACUUM INTO`, a pragma), with no exec or script, so it
  stays inside the no-server-side-execution constraint.
