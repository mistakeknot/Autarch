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

## Closed at schema v4 (B1.5)

The three capture gaps above the line were the ones whose content could not be backfilled. They are closed.

- **The tmux sweep publishes a roster.** `scan.completed` from `tmux-inventory` carries the socket, the server incarnation, and every pane as `%id:pane_pid`, factored so the server identity is named once. A pane's absence is now a positive, replayable observation, and a binding closes with `pane_absent_from_complete_scan` — or `pane_pid_changed`, an enum value that had existed since v3 with nothing to write it.
- **Verification is bounded by the last complete sweep.** "Recent" cannot mean a young observation: the sweep dedupes, so a pane unchanged for a week has a week-old observation and is perfectly alive. It means the last complete sweep listed that pane, at that pane pid, on that server. Before this, a fresh claim could be verified against a pane that died days ago.
- **`verifyPane` no longer matches on `pane_id` alone**, and no longer overwrites the claim. Measured 2026-09-19 over every live record: the claimed window id agrees with the live server 11 times out of 11, while the claimed session name disagrees 3 times — `tmux-organizer` against `iterm[autarch - e4be…`, `iterm]` against `iterm[]`, and one trailing space, all three the same pane. So the window gates verification and the session name cannot; both are preserved in `claimed_window_id` and `claimed_session_name` beside what the server said.
- **`ppid` is captured** by the same `ps` run as the liveness probe, in a roster field of its own so v3 events still replay. Targets now include the records the sweep itself just read, not only rows the projector has already written — otherwise nothing is known about a process until its second sweep, and a dispatched child that finishes inside one interval would have its parent read exactly never.

Measured on the live estate, first sweep after migration: **11 of 11 `cli` agents' ppid is exactly their pane's root process.** The one `sdk-py` child's is not, and does not resolve to an instance either: the chain is `sdk-py(33275) → Python shim(32795) → claude cli(3466) → zsh(35924, pane root)`. One hop does not reach the parent agent on this estate. `parent_instance_id` is left NULL rather than guessed, and the raw pid is kept so a later pass can walk further if B2 needs it.

## Corrected at schema v5, after independent review

The review found the ninth instance of the recurring bug, in the measurement itself.

**"Claimed window agrees 11 of 11" measures only that the gate admits true claims.** It never sampled the rate at which the gate admits *false* ones, and an estate with one socket cannot: window ids are per-server counters exactly as pane ids are, so another server's `@0.%0` passes against this server's `@0.%0`. The pane-id-only bug had been moved one column over, and then confirmed by measuring the wrong thing.

- **The process table is a second instrument, and pids are host-unique.** `pane_binding.corroborated_by` records `'parent_pid'` when an agent's parent is this pane's own root process — re-evaluated every sweep, because it is a statement about present agreement, not a historical one. It is never a gate: a dispatched child's parent is legitimately the shim. But a **contradiction** refuses, because a parent that is some *other* pane's root process on the swept server is positive evidence of a mismatch.
- **Verification is now uniformly roster-driven.** Verifying on claim insert ran one step ahead of parent capture, so a claim was always judged before the process table had said anything about it. `retryStuckClaims` picks it up moments later in the same watcher run — which also fixes a claim refused on a window disagreement and then corrected by the record, on a pane that never changes, which nothing could previously reach.
- **A verified binding no longer freezes at the moment of verification.** `verifyPane` refreshes the observed columns of rows matched by full pane key, so a pane moved by `break-pane` carries its new window instead of reading "confirmed present 30 seconds ago" beside one it left days ago.
- **Refusals outlive the pass that made them,** in `projection_state`. `source.last_error` was overwritten by the next successful pass, and one sweep projects twice.
- **`recordParent`** requires the parent to be `alive` in the same probe run, bounds it with `parent.started_ms <= child.started_ms`, and the reconcile loop is ordered, which it was not.
- **`ppid` is set once.** A reparented process reports ppid 1, and overwriting with it destroys exactly what the field was captured for.
- **A vanished pane is not reclaimed** while the roster still does not list it, and a `remain-on-exit` pane is recorded as `:dead` in the roster — present, and not a place anything is running.

A migration bug surfaced on the copy and never reached the live database: `projection_state` was emptied rather than dropped on reset, and `CREATE TABLE ... IF NOT EXISTS` is a no-op against an existing table, so a version that added a column to it migrated "successfully" and failed on the first write. It is now dropped with the other projections, which is what it is.

## Still open after B1.5

- **A claim from an unswept server can still pass the window gate** when its parent contradicts nothing this registry can see. `corroborated_by` is what makes that row distinguishable; B2 must filter on it rather than on `binding_basis` alone.
- **Multi-socket is unbuilt, not merely untested.** One tmux source holds one socket in its locator; a second server needs a second source.
- **Parent lineage stops at one hop.** Measured: `sdk-py(33275) → Python shim(32795) → claude cli(3466) → zsh(35924, pane root)`. Walking further needs the whole process table rather than the pids already asked about. Deferred to B2, where lineage has a consumer.
- **The pane-level `Unparsable` path is unreachable.** `agenttransport.List` validates every pane and fails the whole call, so a tmux sweep is complete or it errored; there is no partial. Harmless today — the conservative direction — but the registry's own check is dead code and should not be relied on.
- **The v3→v4 rule change is visible in the log.** Bindings verified under v3's unbounded lookup replay as claims under v4's roster rule, because no rosters existed before the first v4 sweep. B2 must not read "was a claim" as "was never in tmux."
- **Superseded server incarnations keep their bindings open.** The reviewer showed the "agents die with the server" argument is unsound both ways — `setsid` children outlive a server kill, and `/tmp` cleanup can unlink a socket while the server lives — so leaving them open stays the safe outcome, and no `server_superseded` closure should be added. B2 filters on `last_present_event_id` against the latest roster.
- **A reopened instance flaps** if the condition that caused the false closure persists. `reopened_count` surfaces it.
- **Link intervals mix two clocks** — `observed_from_ms` is the provider's, the closing `observed_to_ms` is the sweep's — so an interval can come out negative if the provider's clock runs ahead.
- **Roster retention.** Session roster ~1.4MB a day; pane roster ~1.4KB a sweep, about 4MB a day at 30s. Unbounded; retention has no consumer yet.
