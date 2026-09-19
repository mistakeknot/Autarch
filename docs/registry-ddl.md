# Estate registry DDL — Autarch Phase B1

Status: **reviewed across four passes; the watcher is approved to run**. This is the first deliverable of Phase B1, ahead of any consumer, because the capture schema is the one thing in the attention-router plan that cannot be backfilled. Schema lives in `internal/registry/schema.go`; the invariants below are each covered by a test in `internal/registry/schema_test.go`, and each was mutation-checked — removing the constraint turns the test red.

Reviewed under governed dispatch by `claude-fable-5-1` at high effort, `plan-review` seat, producer `claude-opus-5`, policy hash `81481511…98fea`, classification `foundational-invariants` + `broad-consequences`, requirement `other-frontier`. Verdict: **land with changes**, seven blocking (B1–B7) and seven following (F1–F7). All fourteen are in. The review record and what each changed is at the end of this document.

## What the live estate actually looks like

Measured on Clavain, 2026-09-19, from `~/.claude/sessions/*.json`. Twelve records, all twelve processes alive. Every design decision below is answering something in this table rather than something imagined.

| Observation | Count | Consequence for the schema |
| --- | --- | --- |
| Records whose `cwd` is `/Users/sma/projects` | 10 of 12 | cwd cannot carry project attribution |
| Distinct panes | 11 | so two conversations share one pane |
| Panes holding two live conversations | 1 (`%67`) | a pane binding is many-to-one, at a single instant |
| Records where `entrypoint` is `sdk-cli` | 1 | the second occupant of `%67` is a dispatched child, not a human launch |
| Records whose `tmux` session name disagrees with the live server | 2 | the session name is an observation, never a key |
| Records with `waitingFor: "input needed"` | 5 | five rulings are outstanding as this is written |
| Waiting records whose session name carries no UUID | 1 (session `2`) | transcript-keyed discovery cannot see it at all |
| Orphaned `.key` files with no `.json`, from August | 3 | the directory is not self-cleaning; pids recur |

Two of those rows are worth stating plainly, because they are the difference between a schema that works and one that looks right.

**The session name is not identity.** Record `55409` claims its pane `@98` belongs to tmux session `tmux-organizer`. The live server calls that same pane `iterm[autarch - e4bedaf5…`. Separately, the two records sharing pane `%67` disagree with each other about that pane's session name by one bracket: `iterm[]linsekasten` versus `iterm]linsekasten`. Both claims were true when written. Neither is a key.

**The name-embedded UUID identifies the pane's first occupant, not the process reading it.** The `sdk-cli` child in `%67` has session id `90057d0b…`, but sits in a pane whose name embeds `74e5950e…` — its parent. Anything that resolves a transcript by parsing the tmux session name reads the wrong conversation, confidently.

## Three identities

A single "session" record would have to be three things at once, and the estate proves all three lifetimes differ.

**`conversation`** is durable and outlives every process that runs it. Keyed `(provider, host, provider_session_id)`. `host` is in the key because the estate spans Clavain and zklw: without it, a synced copy of another machine's record merges into this one's history. It also carries `display_name` and `display_name_source`, because the provider's `auto` names are genuine topic summaries (`completion-driven-handoffs`, `constellation-front-door`) while its `derived` names are placeholders (`projects-14`) — and storing which is which is what stops a placeholder being read as a subject.

**`launch_instance`** is one process launch. Keyed `(host, pid, proc_start_ms)`; pid alone recurs after a reboot, and the orphaned August records prove the directory keeps no tombstones. `entrypoint` distinguishes a human `cli` launch from an agent-spawned `sdk-cli` one, which is what separates the parent from the child inside `%67`.

**`pane_binding`** is time-bounded and deliberately permits many conversations per pane. Its key follows `agenttransport.Target.SamePane()` — socket, server pid, server incarnation, pane id, pane pid — and is computed by the database as a `STORED` generated column rather than by the writer, so it cannot drift from the Go definition it mirrors.

## The invariants, and the constraint that enforces each

Every rule below is a column or a `CHECK`, not a convention. A rule enforced by good intentions is a rule that holds until the first hurried patch.

**Nothing closes without named evidence.** `launch_instance`, `pane_binding` and `item` each carry `CHECK (<closed_at> IS NULL OR <basis> IS NOT NULL)`. A failed tmux read has no basis to offer, so it is structurally incapable of ending a live record. This is the storage-layer form of "we could not look must never render as there is nothing there."

**Absence is structural, never a label.** `outcome.action` has no `ignored` value, and there is no `unassigned` project. Three states are derived, not stored: an item with no exposure and no outcome was *missed*; an item with an exposure and no outcome was *shown and not answered*; an outcome with a `NULL exposure_id` was *answered out of band*, which on this estate means typed straight into the pane. Merging those into one label destroys the only field that tells us whether the router is working, and no later pass can separate them again.

The naive derivation gets this wrong by one clause. Counting "no exposure" as missed marks an item answered directly in tmux as a failure. Since missed rulings are the plan's primary metric, that clause is the difference between a real number and a flattering one. The test asserts all three counts, and it is how the error was found.

**A body may exist only if it has been redacted.** `CHECK (body IS NULL OR body_redacted = 1)` on `evidence`. Transcripts and pane captures carry secrets and are about to be copied into a new database; raw text cannot be inserted by accident. Retention drops bodies while keeping the fact that the observation happened, so pruning never looks like an absence of evidence.

**A source that has never run does not read as healthy.** `source.status` defaults to `unchecked` and `last_success_ms` to `NULL`, never `0`. Every count a view renders must be read together with its source row. This is the table that makes the stale-source guard real rather than aspirational.

**Ingest is idempotent.** `event.dedupe_key` is `UNIQUE` and derived from content, not the clock, so re-reading the entire sessions directory at every startup inserts nothing new. Watchers use `INSERT OR IGNORE`.

**Order comes from `event_id`, not wall clock.** `occurred_ms` is the provider's claim about when the world changed and may be absent or out of order; `observed_ms` is when we saw it; `event_id` is the only total order. It is `AUTOINCREMENT` specifically so evidence retention cannot free an id and silently rewind a replay cursor past it.

**An unconfirmed claim can never be mistaken for a verified one.** A binding built only from the session record's own `tmux` string gets `binding_basis = 'session_file_claim'` and a `pane_key` prefixed `claimed:`, which cannot collide with a verified key. A binding claiming `tmux_inventory` without full server identity is rejected outright.

## What Phase B1 writes, and what it does not

B1 writes `source`, `event`, `evidence`, `conversation`, `conversation_lineage`, `launch_instance`, `pane_binding` and `project_association`. Its only producer is a read-only watcher over `~/.claude/sessions/`; it writes nothing into any configuration file.

`item`, `item_source`, `exposure`, `outcome` and `decision` are defined here but have no writer until B2 and Phase C. They are in this review deliberately: the absence fields are the ones that cannot be backfilled, so they must be reviewed before the first row is written, not after. Freezing them with nothing writing them is the failure Phase B2 exists to prevent — door is losing exposures today.

`item_source` is the union rule as a table. Three independent sources may attest to a ruling — transcript evidence, first-party wait state, detector evidence — and the table is insert-only by construction: there is no column a source could clear to withdraw its attestation. Resolution is a separate act on `item`, requiring positive evidence.

## Questions this review should settle

1. `project_association` records only positive associations, and the *reason* a conversation is unattributed lives in an `attribution.attempted` event rather than a row. That keeps the table meaning one thing, at the cost of making `#unassigned`'s reason an event scan. Is that the right split, or should a failed attempt be a first-class row?
2. `conversation_lineage.relation` includes `spawn` for the SDK-child case. A dispatched child is arguably not lineage at all but a distinct conversation with a causal parent. Same edge, different word — does the distinction need two tables?
3. `pane_binding` stores the tmux `session_name_seen` for display. It is excluded from `pane_key`, so it cannot corrupt identity, but it will go stale in the row. Should it instead be resolved at read time from the live server?
4. `host` is a bare string. zklw sessions are unobserved today and the coverage line must say so rather than reading as empty. Is a string enough, or does coverage need its own table from the start?
5. `SchemaVersion` is stamped in `PRAGMA user_version`, and `Open` refuses a database newer than the build but does not migrate an older one. For a registry that is rebuildable by replay, is refuse-and-rebuild the right migration story, or is that too casual about the event log itself?

## Review record

The first draft passed its own 13 tests and was still wrong in a way no test could have caught, because the flaw was in what the tests were shaped around.

**The spine foreign-keyed into its own projections.** `event.conversation_id` referenced `conversation`, whose `first_event_id` referenced back into `event`. The cycle meant an event had to be written, then updated once its conversation existed — on a table documented as append-only — and it meant `DROP TABLE conversation` failed outright. The whole replay story was decorative. The fix is that `event.conversation_id` and `event.instance_id` are plain text holding **deterministic** ids computed from natural keys (`ids.go`), with no foreign key. A random id would have survived the rebuild as an orphan; a deterministic one is regenerated identically. `TestNothingForeignKeysIntoAProjection` reads `PRAGMA foreign_key_list` for every spine and durable table rather than trusting a comment, and `TestProjectionsCanBeDropped` performs the drop.

**`/clear` replaces `sessionId` in place.** The review deferred this to an experiment; it was run on a scratch session and confirmed: same pid, same `startedAt`, same file, new `sessionId`. `launch_instance` therefore cannot hold `conversation_id`, because `UNIQUE(host, pid_domain, pid, started_ms)` would have forced the second conversation to overwrite the first. The link moved to a time-bounded `instance_conversation`, with a partial unique index allowing exactly one open conversation per process and any number over its life.

**"Nothing closes without named evidence" was overstated.** The original `CHECK (ended_ms IS NULL OR end_basis IS NOT NULL)` accepted `end_basis = ''` and, worse, `end_basis = 'tmux_read_failed'`. It stopped a forgotten basis, not a failed read — which is the case it existed for. Every closing basis is now a closed enum of positive observations, and closing additionally requires the `end_event_id` that carries it.

**`INSERT OR IGNORE` was the wrong idiom.** It suppresses `NOT NULL` and `CHECK` failures as well as duplicates, so a write that could not happen reads as "nothing new" — precisely the suppression the schema exists to prevent. Ingest uses `ON CONFLICT(source_id, dedupe_key) DO NOTHING`, and the uniqueness is scoped per source, since a global key lets two producers' collision drop a row silently.

**The unverified `pane_key` was built from the field proven to drift.** It hashed the session name and window id, both of which change under a rename, a `join-pane` or a `break-pane` — so one instance could take two open bindings on one pane and the unique index could not see it. It is now `'claimed:' || pane_id`.

**Coverage gaps were unrecorded.** `source` held only the latest attempt, so "nothing happened between 14:00 and 15:00" and "the watcher was down" read identically. `source_scan` records each sweep and whether it was complete; only a complete sweep may support an absence claim, which is also the only honest basis for citing a vanished record.

**Three further losses, all now closed.** The event payload is the real schema and was unspecified — it now carries the full raw provider record under `CHECK (json_valid(payload))`, including `ppid` and `pidDomain`, which are observable only while the process lives. `proc_start` had two incompatible readings, a millisecond epoch and a timezone-less one-second string that disagree by over a second; the epoch is canonical and the string is kept unparsed as evidence. And foreign keys were set by `Exec` on a single connection, which a reconnect silently drops — they now go through the DSN, which also disproved the stale comment in `pkg/db/open.go` claiming modernc rejects DSN parameters.

**On the attention tables**, `outcome` gained a `channel` and lost `answered_out_of_band`, which encoded the same fact twice and could disagree with `exposure_id`; a composite foreign key stops an outcome citing another item's exposure; `exposure` gained `mode` and `rank`, neither reconstructable later; `item` gained an `anchor_key` and a `merged_into_item_id`, without which one ruling seen by two sources becomes two items and the primary metric double-counts; and `project_association` gained a `stance`, because an operator saying "this is *not* project X" is a fact a positive-only table cannot hold and an automatic sweep would otherwise overwrite.

**The missed-rulings derivation was wrong in the test that was meant to protect it.** It filtered on unresolved items, which hides the truest miss on the board: an item nobody ever saw, whose conversation then ended. Missed is derived from absent exposure and absent outcome, regardless of resolution.

## Four review passes, and the bug that kept coming back

The DDL was reviewed once, the watcher three more times, all under governed dispatch by `claude-fable-5-1` at the `plan-review` seat with `claude-opus-5` as producer. The second receipt was **invalidated** — the dispatch guard caught a checkout mutation, which was mine: I was editing the working tree while the review read it. Its findings were acted on anyway, and the pass was rerun against a clean tree.

The final verdict is **approved to run the watcher; not approved as the base for Phase B2**.

One bug appeared seven times in this work, in seven different disguises, and it is worth listing them together because the whole schema above exists to refuse exactly this: **an absence, or a failure, read as a positive claim about the world.**

1. An event-derived roster read every deduped, unchanged record as a departed agent.
2. `agenttransport.list` folds "no server running" into an empty success, so a live 106-pane estate reported as "0 panes, complete=true".
3. A deduped *pane* meant a conversation arriving in a quiet pane could never have its binding verified, because the event that would verify it was never going to be written.
4. `ps -o pid=,etimes=` is a Linux field; BSD ps prints "keyword not found", lists bare pids and **exits 0**. With a default verdict of dead, one sweep closed all eleven live agents.
5. The fix for (4) produced its mirror: BSD ps exits 1 and prints nothing when *none* of the pids exist, so "every agent died" and "the probe failed" were identical, and nothing would ever close. The last agent to exit would stay open forever.
6. A false closure could not be taken back: a later observation upserted around it, links and bindings accumulated underneath, and the agent went on writing records into a row nobody could see.
7. And the hole in the fix for (6): recovery fired only on a *changed* record, so it reached every agent except the quiet ones — and the quietest agent on this estate is the one waiting for an answer, which is the exact agent the attention router exists to surface.

Three of the seven were found by review, one by a test being written, and three by running the thing against the live estate. None was found by reading the code. The pattern in every case is the same: a default, an empty container, or a failed call resolving to a confident statement. The constraints above catch it at the storage layer; the probe's sentinel, the scan roster, and `source_scan` catch it at the producer.

## Still open after the gate

- **Pane verification can rest on an observation of any age.** The tmux sweep writes no `scan.completed` event and no pane roster, so nothing records that a pane was absent, and a fresh claim can be verified against a pane that died days ago. The roster content cannot be backfilled. This blocks Phase B2 work that reads bindings; it does not block the watcher.
- **`verifyPane` matches on `pane_id` alone** and overwrites the claimed window and session name, destroying the evidence of a mismatch. Latent while there is one socket, and it wants `ppid` to do better.
- **`ppid` is unpopulated.** It is only observable while a process lives, so every day without it is lost.
- **A reopened instance flaps** if the condition that caused the false closure persists. `reopened_count` surfaces it rather than hiding it.
- **Link intervals mix two clocks** — `observed_from_ms` is the provider's, the closing `observed_to_ms` is the sweep's — so an interval can come out negative if the provider's clock runs ahead.
- **Roster retention:** `scan.completed` carries the roster every sweep, about 1.4MB a day.
