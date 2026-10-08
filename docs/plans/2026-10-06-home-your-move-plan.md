# Home "Your move": everything that waits on mk (bead mk-okek.24)

Status: draft for cross-lab review. Base: 6bcf7ff (vizier resolver). Home layer schema v3 -> v4.

## Problem

mk: "i want home to be the true home of my attention so i am never blocking work."
Today a pick whose option means "mk acts" (run a root script, merge a PR, read a doc)
rules the card, and a ruled card leaves Home. Work then waits on mk while Home is
empty. Undeliverable wakes (recipient archived, HTTP 409) are marked `undeliverable`
and dropped from view.

## Decisions

D1. **A card stays on Home until evidence closes it, not until mk picks.** A new Home-layer
table `moves` holds one row per (task ULID, generation) that waits on mk. A ruled card
with an open move shows in "Your move" and not in Asks.

D2. **Evidence is observed, never typed by an agent.** Closers:
- script: a `report-tell` success message from the script's thread whose body names
  the script's sha256 (match on full 64-hex). A failure report keeps the move open and
  records the failure text on the row.
- pr: GitHub state `merged` or `closed`, read with `gh pr view --json state` by a
  poller (read-only; no merge capability). Poll failure leaves the move open and shows
  "state unknown".
- read: mk presses "Done" in Home (the only mk-marked closer).
- needs-context: mk supplies the context through Home (a text box that appends a
  `Context:` note to the card and closes the move) or marks it done.
Closing records `closed_by` (evidence kind), `closed_at`, and the evidence reference.
Nothing else closes a move; a vizier or agent comment cannot.

D3. **Home never runs anything.** Rows only display commands (existing display-only
rule, badge "run by paste, not authenticated"). The PR poller only reads.

D4. **Untrusted text.** All move fields come from card text; Home renders them as
text, validates the sha (64 hex), the script path (absolute, no whitespace/control
chars), and the PR URL (exactly `https://github.com/<owner>/<repo>/pull/<n>`) and
shows an invalid row as "needs repair" rather than a command.

## Data model

Card convention gains an optional fenced block ```home-move``` (JSON, `home-move/v1`):

    { "kind": "script"|"pr"|"read"|"context",
      "title": "...",
      "script": { "path": "/abs/path.sh", "sha256": "<64 hex>", "check": "bash /abs/path.sh --check", "run": "bash /abs/path.sh" },
      "pr": { "url": "https://github.com/o/r/pull/N", "method": "squash"|"merge"|"rebase" },
      "read": { "url_or_path": "..." },
      "context": { "needs": "what mk must supply" } }

Only the member for `kind` is allowed. `check`/`run` are derived from `path` when
absent (`bash <path> --check`, `bash <path>`), so the displayed command always carries
the absolute path; the row also shows the full sha256 (never truncated).

Table `moves(task_id, generation, kind, payload_json, state open|closed, opened_by
card|pick, opened_at, closed_at, closed_by, evidence, last_checked_at, last_error)`,
primary key (task_id, generation). Migration v3 -> v4 adds the table; no data rewrite;
down-path is leave-in-place (v3 code ignores the table).

## Item 1: pick keeps the card

Option gets an optional `then: "mk-acts"` marker (home-ask/v2 already carries option
kind; `instruction` and `needs-context` imply it; a new optional `move` object on the
option provides the home-move payload). When mk picks such an option, the ruling
writes as today, then in the same transaction a `moves` row opens with `opened_by=pick`.
If the option has no move payload, a `read`-like generic move "do what option X says"
opens with the option text (never silently dropped). `ruling-only` options open no move.
Card closes in tasks only when the move closes (existing close path).

## Item 2: file "mk action" without options

`bb home file --move <json|file>` and the Home plugin RPC `file` accept a card with a
`home-move` block and no `home-ask` block. Such a card is valid (the strict parser
accepts exactly one of: home-ask, home-move). Label `needs-mk` plus `mk-move`.
Filing derives the idempotency key as for asks (Request line). Re-filing the same key
is a no-op; changed payload bumps generation (old move closes as `superseded`).
Authority to file is the same as asks (vizier or an authorized thread). A filing
thread cannot also close the move (D2).

## Item 3: undeliverable wake goes to the vizier

In `wakes.ts` `settle`/`onThreadGone`, on transition to `undeliverable` the obligation
enqueues a `vizier-notice` obligation addressed to the resolved vizier (resolver from
6bcf7ff, adopt:false) containing: original recipient, subject, the wake text (truncated
to 2 KB), reason (archived / HTTP 409). If the vizier is itself the gone recipient or
the resolve fails, the notice stays visible in Catch-up as `undeliverable:<id>` (today's
behaviour) and also appears as a "Your move: unrouted notice" read item. A vizier notice
is sent at most once per obligation id (unique key), is never itself re-routed on
failure (no loops), and the original stays `undeliverable` with `routed_to_vizier=1`.
Backfill: a one-time scan at migration routes the undeliverable obligations from the
last 14 days once.

## Item 4: needs-context

A picked `needs-context` option opens a `context` move (even with no payload; text
defaults to the option label). It stays until mk supplies context (Context note) or
marks done. Supplying context also tells the owner thread via the existing wake path.

## UI

"Your move" section above Asks; queue-row style as Asks. Row: title, age, kind badge,
card id/project, and exact action: script (two copy boxes: check, run; absolute path;
full sha256 line); pr (the URL as the link text, merge method line, state:
open/merged/closed/unknown + last checked); read (link/path + Done button); context
(needs text + input + Done). Empty state: "Nothing waits on you." Count in the Home
header/tab. A move whose script report failed shows the failure and stays open.

## Evidence plumbing

- Reports: the plugin already receives `[bb message ...]` wakes via its thread-message
  hook; add a matcher for `report-tell` bodies (script name + sha + OK/FAILED). Only
  messages from the thread recorded on the move (`owner_thread`, from the card's
  Blocks/owner line) or the forwarded-to live same-title thread count.
- PR poller: every 5 minutes for open pr moves, `gh pr view <url> --json state,mergedAt`,
  bounded concurrency 2, backoff on errors, no secrets stored. Runs on the host only
  while the plugin is up; startup polls immediately.

## Stopgap migration

The vizier's ordinary cards for open scripts/PRs stay valid. Migration helper
`bb home adopt-move <card> --move <json>` attaches a move to an existing open card
(same authority as filing); closing the card is unchanged. Duplicates close as
`superseded` once the move card exists. No automatic conversion of the 40 legacy cards.

## Tests

- migration v3 -> v4 on a v3 db (data kept, table created, idempotent rerun).
- parser: move block valid/invalid per kind, extra members rejected, sha/path/URL
  validation, ask+move together rejected, no-options move card accepted.
- pick opens move; card stays out of Asks and in Your move; close paths: report OK
  (matching sha), report OK wrong sha (stays), report FAILED (stays + text), report
  from another thread (stays), PR merged/closed/open/poll-error, read Done,
  context supplied.
- agent/vizier comment cannot close a move.
- undeliverable wake routes once to vizier; vizier-gone fallback; no loop; backfill once.
- UI render snapshots for each kind including long sha and invalid-payload row.
- real-host check: file a move card on the installed build, see it, close by evidence.

## Rollout

PR from branch `home-your-move` for mk (no push to main). Install via a mk script
(check mode first, backup of data.db, schema 4 verified), same shape as
home-install-6bcf7ff. Rollback: reinstall previous build; v3 ignores `moves`.

## Risks / open

- Matching report-tell bodies is text-based; a spoofed report from the owner thread
  closes a move. Mitigation: owner-thread match plus full-sha match; residual risk
  stated, same trust as wakes today. Reviewer to challenge.
- PR poller needs `gh` auth on the host; unknown state is displayed, never treated as closed.

Decision context: reasons `broad-consequences`, `difficult-verification`;
rationale: changes what closes an authority-bearing card and adds an evidence path.

## Phase 2: Home populates and routes itself

Phase 1 (above) ships first and is independently useful. Phase 2 removes the vizier
as the pipe. Sources already exist; Home subscribes, it does not ask agents to file.

### Ingest sources

1. **ESCALATION messages** (vizier-tell). The plugin's message hook parses the
   first line `ESCALATION` and the fields: thread, project, bead, kind, question,
   options (with what each does, reversible or not), recommendation, meanwhile.
   - kind `decision|approval|blocker` files a home-ask/v2 card (sender thread =
     owner and wake target). Options and recommendation are required; a message
     that does not parse files a card with a single `needs-context` option and the
     raw text, flagged "unstructured", never dropped. The sender is told once, by
     wake, to resend in the structured form.
   - kind `done` becomes a feed item (Catch-up), not an ask.
   - The vizier keeps delegated rulings: it rules delegable cards with
     `bb home rule` (existing delegation path) instead of relaying.
2. **PRs.** `sonnerie register owner/repo#N` on mk's repos (mistakeknot, gensysven)
   emits a registration event Home reads (sonnerie registry file/DB, read-only).
   It files a `pr` move "Your move: merge" with the github.com URL and merge
   method, closing on GitHub merged/closed (D2). The vizier or owner thread may mark
   it `not-ready` (draft, under review, with a reason); not-ready moves show in a
   collapsed "Waiting on others" group and return when cleared. Draft PR state read
   from GitHub also sets not-ready automatically.
3. **Root scripts.** A handoff (script path + full sha256) files a `script` move.
   Source: a `home-handoff` marker the handoff writes (the existing vizier-tell /
   report-tell flow carries `HANDOFF script=<abs path> sha256=<64hex>` as a line;
   Home parses it only from the owner thread). Closes on report-tell success matched
   by full sha.

### Dedup (one item per underlying thing)

Identity key: PR = canonical URL; script = sha256; escalation = (project, bead or
question slug). Table `move_keys(key UNIQUE -> task_id)`. A second filing with the
same key updates the existing card (new text/options bump generation only if
material; an unchanged repeat is a no-op) instead of filing a new one. The stopgap
hand-filed cards: `bb home adopt-move` matches by key (URL/sha parsed from their
text); on match the stopgap closes as `superseded -> <card>`; on no match it stays
and is listed once in Catch-up for the vizier to close. Never auto-close a card
mk has picked on.

### Routing, never drop

Wake target order: owner thread; if archived/gone, its live successor (same title,
unarchived, newest, the report-tell rule); else the vizier (resolver, adopt:false);
else keep the obligation `pending` and show it in Catch-up and "Your move" as an
unrouted notice. A wake is only `done` after delivery; `undeliverable` is never
terminal-and-invisible. Retries on 409 re-resolve the target rather than re-sending
to the same thread. Item 3 of phase 1 is subsumed by this order.

### Spoofable evidence (all three closers)

Threat: text in a message or card claims completion. Controls:
- Script: report must come from the move's owner thread or its title-successor, carry
  the full 64-hex sha, and name the script path; a stale sha (script re-cut) does not
  close the new move. Where possible the script's report-tell also writes a signed-free
  but non-chat receipt file in thread storage (`report-<sha>.json` with exit status);
  Home reads the file as the primary evidence and the message as a trigger only. A
  message with no matching receipt keeps the move open, labelled "claimed, unverified",
  and mk can confirm it with one click (mk-marked closer).
- PR: only GitHub API state closes it; never message text. Poll auth failure = unknown.
- Read/context: only mk's action.
- Ingest authority: ESCALATION and HANDOFF lines are honoured only from a live bb thread
  the host knows (sender thread id from the bb message envelope, not the body). The
  body's claimed thread must equal the envelope sender or the item is filed as
  "unverified sender".
Residual: a compromised owner thread can forge both message and receipt; same trust as
any thread today. Stated, not mitigated.

### Phase 2 tests

ESCALATION parse (each kind, missing options, mismatched claimed sender, done -> feed);
sonnerie registration -> move, draft -> not-ready, merged -> closes; handoff line only
from owner; dedup (re-escalation, same PR twice, same sha twice, stopgap adoption,
picked card never auto-closed); routing order (owner, successor, vizier, pending);
receipt present/absent/mismatched sha; forged message without receipt stays open.

### Phase 2 rollout

Separate PR on the same branch lineage after phase 1 review. Ingest runs in
observe-only mode first (cards created in a `shadow` state, shown to the vizier only
in Catch-up) for one cycle, then enabled by a Settings toggle mk or the vizier sets.

### Projects with no root (UNC, INTV, ICOR, JAWN)

Decision: **fallback bucket, not a precondition.** Home must show every needs-mk card
whether or not `autarch serve` has a project root for its tracker. A card whose
tracker prefix has no bound root is shown under an "Unbound projects" group (prefix,
card id, title, age), with the full Your move/Ask rendering. What needs a root
(opening files, the project map, root-run path checks) degrades to text: a root-run or
script row still shows its absolute path and sha from the card, with the badge
"project not bound". Binding a root later moves the cards into their project with no
data change (grouping is derived from the binding, not stored). The vizier-facing
Catch-up lists unbound prefixes once ("add a root or leave unbound") so adding roots
stays a choice, not a blocker. Seed check: the 8 existing cards (UNC-1, INTV-6/7/10,
ICOR-6/7, JAWN-12) must render in the bucket in a test fixture with no root.

### Dedup against prior rulings on siblings

Same-key dedup is not enough (4 duplicate asks from the intake steward, which judged
from descriptions and missed later comments). Before auto-filing an ask for card C:
1. Read C's comments, not only its description; a later comment from mk ("You") or a
   recorded ruling counts as input.
2. Look up sibling cards: same bead, same Request subject slug, same PR URL or script
   sha, or the same parent/Blocks target. If a sibling has a ruling (ruled generation,
   or a closed move with evidence), do not file a fresh ask: attach the prior ruling to
   C as a note "already ruled on <card>: <pick>" and file nothing, unless the new
   text differs materially (different options or a new fact), in which case file with
   the prior ruling quoted and flagged "re-ask: prior ruling X".
3. Same-key open sibling: update it (existing dedup rule).
Matching is conservative: it only suppresses on exact subject-slug, bead, URL or sha
equality, never fuzzy text, so a miss yields one extra card, not a lost ask. Every
suppression is logged to Catch-up so a wrong suppression is visible and reversible.
Tests: sibling with ruling suppresses; sibling ruled but new options re-asks quoted;
later mk comment on the card counts; fuzzy-similar different slug still files;
suppression is listed in Catch-up.

## Conversation on the card

mk: "i should be able to have these conversations on the cards themselves." Each Home
card is where talk about it happens. This is Phase 1b (shippable after Phase 1, before
Phase 2; depends only on the card model). Nothing here may close, rule on, or change
the options of a card.

### Rendering (req 1, 3, 4)

The selected card shows a "Conversation" panel: the task card's comments, oldest
first, each with author class and time. Author class is derived from the comment's
recorded author id, not its text: `mk` (the human user id), `owner` (the card's owner
thread or its title-successor), `vizier` (the resolved vizier), `other` (shown plainly
as "other: <id>"). All comment bodies are untrusted text: rendered as plain text
(preserved line breaks, no markdown, no HTML, no links activated, no ANSI), length
capped on display with "show more". mk's pasted script output is stored and shown the
same way, in a monospace block, never interpreted.
Live update: Home subscribes to the tasks comment event for open cards and falls back
to a 15 s poll of the selected card plus a 60 s poll of the rest; no reload.

### Marked updates (req 3)

A comment that changes what mk must do is marked "Changes your move". Marking is by
convention a first line `UPDATE: <sha|gates|hold|cancel> ...` from the owner, vizier or
filing thread only, parsed strictly; other authors' marker lines render as ordinary
text. Effects are display only: a banner on the row ("new sha", "don't run yet") and
an unread highlight. It does NOT mutate the move: a new script sha reaches the row
only through an authorised `adopt-move`/re-file (bumps generation) as before, so the
banner never displays a command Home has not validated. A `hold` marker shows the
row as "hold requested, check the conversation" and does not hide the command.

### Reply (req 2)

mk replies from the card. The reply posts as a task comment authored as mk (via the
plugin's tasks-comment capability under mk's session identity; Home never posts under
an agent identity) and enqueues a wake to the owner using the routing order:
owner thread, live title-successor, vizier, else `pending` shown in the card ("not
yet delivered") and in Catch-up. The comment is written first (the durable record),
the wake is an obligation that retries; a failed wake never loses or duplicates the
comment (idempotency key = comment id). Wake text carries the card key and the reply,
truncated to 2 KB with a pointer to the card.

### Unread (req 5)

Home layer table `card_seen(task_id, user, last_seen_comment_id/ts)`. Selecting a card
and viewing the panel sets it to the newest comment id shown. A queue row shows an
unread dot and count when any non-mk comment is newer than `last_seen`; owner and
vizier comments count, mk's own do not. Seen state is per user, local, and never
affects ranking or closure.

### Authority boundary (req 6)

The conversation is data, not control. No parser of comment text reaches rule, close,
options, move payload, or evidence. The only comment-derived behaviour is the
`UPDATE:` display marker (display-only, above). Rulings come only from mk's pick; moves
close only by D2 evidence. A test asserts that a comment of any author, containing
`Request:`, a home-ask/home-move block, a root-run block, "ruled", or a report-tell
success line, changes neither card state, options, nor any move.

### Tests (1b)

order and author-class derivation (mk, owner, successor, vizier, other); forged "mk:"
text from an agent comment is not class mk; plain-text rendering of markdown, HTML,
ANSI, very long and control-char bodies; pasted output stored and shown verbatim;
live event and poll update; UPDATE marker honoured from owner only, ignored from other;
reply writes comment then wake, retries through archived owner to successor then
vizier then pending, no dupes on retry; unread count with own comments excluded and
reset on view; authority-boundary test above.

### Interim

Until this ships the vizier posts card-specific updates as `bb tasks comment` as well
as telling mk; those comments are the history Home will show on day one.

## Revision 1: answers to cross-lab review r1 (FAIL, 9 findings)

Where this section conflicts with text above, this section wins. Review output:
the review output file in the thread storage. Implementation Step 0 below is a gate:
none of Phase 1 starts until the Step 0 spikes report.

### Step 0 spikes (must pass or the plan narrows)

S-A. **Backup gate.** Reproduce the finding on a real v3 db: the pre-migration backup
verifier (`backup.ts`) rejects `min_reader_version` above 2. Fix: raise the accepted
ceiling to the version being migrated from, with a test that backs up and migrates a
real v3 db to v4, and a test that a v5 db is still refused.
S-B. **Attested caller.** Find out whether the plugin host exposes an authenticated
caller identity for UI-originated RPC versus CLI/agent RPC (today `by: mk` is advisory).
Outcomes: (1) host attests UI sessions as the human user: mk-only actions are accepted
only from UI-originated calls; (2) no attestation: every "mk-only" action (Done,
context, reply, confirm) is recorded `by=unattested-ui`, closers that rest on it are
labelled "marked done in Home (unattested)", and the plan states plainly that Home
cannot stop an agent from calling the same RPC. Under (2) no closer relies on mk-only
identity for safety; the real controls are evidence closers. Author class on comments
is shown only if the tasks API returns an author id; otherwise comments display
"author: unknown" with the raw author field text and no class badge, and
`UPDATE:` markers are not honoured (marked-update feature disabled until attested ids
exist).
S-C. **Ingest interfaces.** Determine what the host actually offers for: inbound
thread-message bodies, task-comment events, and the Sonnerie registry. Findings: the
plugin only sees queued-message dispatch/cancel; Sonnerie `register` writes a registry
file and removes entries on terminal state. Plan from this: no promise of event
ingest. Home **polls**: tasks list/comments via the tasks API on a cursor (last seen
comment id per card, replay-safe, idempotent by comment id); Sonnerie registry read
through a documented read-only path with a cursor on first-seen PR URL, and a
tombstone table in Home so a PR that Sonnerie later removes still resolves via GitHub
state. ESCALATION and handoff ingest need a source: the vizier-tell transport is the
only place they exist, so Phase 2 ingest depends on a small contract change owned by
vizier-tell (it writes a structured record to a drop-box Home polls: `<dataDir>/home-inbox/*.json`,
one file per message, with envelope sender id supplied by the bb thread message
metadata, never the body). If S-C finds no way to get an attested sender, Phase 2 ingest
files items as "unverified sender" cards (still shown, never silently dropped, never
auto-closing anything).
S-D. **Rootless cards.** Read `service.ts` ingestion: cards without a resolved
`project_root` stay `observed` and never materialise. Design: add a materialisation
path keyed by tracker prefix with `project_root = NULL`, a display record only; disabled
actions: open file, map, root-run hash check against the live file (still shows the
path and sha text, badge "project not bound", never a runnable "verified"). Tests with
the 8 named cards in a no-root fixture.

### Closers rewritten (finding 3, and 2)

Phase 1 no longer closes any script move on chat text. A `report-tell` body naming the
sha is a **claim**: the row shows "reported OK by <thread> at <time>, unverified" and
stays open. Close paths for a script, in order of strength:
1. **Receipt**: the install/handoff script's EXIT trap writes `report-<sha>.json`
   (exit status, timestamp, uid, hostname) into a Home-readable receipts dir
   (`<dataDir>/home-receipts/`, mode 0644, created by root-run scripts as the operator's
   report path). Home accepts a receipt only if its file owner is the operator account or root (stat),
   it names the full sha and the move's script path, and status 0. This is still
   forgeable by anything that can write as the operator account or root; stated residual.
2. **mk confirmation** of a claim or receipt-less run (one click, attestation per
   S-B).
Phase 1 ships (2) plus claim display; (1) ships in Phase 1 only if the script header
change lands in the same PR (the script template used for handoffs is in the vizier's
and my tooling; I own the update for my scripts, the vizier owns theirs). Failure
reports keep the move open. A re-cut script (new sha) never closes the old move.
Successor-by-title is only a routing rule, never an evidence rule: claims from a
successor are shown as claims.

### State machine changes (finding 5)

- Today: a pick queues removal of `needs-mk`, and a ruled card leaves Home; card asks
  close only through tasks, and a description edit does not bump generation. Changes:
  (a) a pick on an mk-acts option writes the ruling as today but **keeps `needs-mk`**
  and adds label `mk-move-open` instead of removing; removal of both happens only on
  move close, by the same write-back path, idempotent by (task, generation).
  (b) Generation is bumped by an authorised re-file/adopt-move that changes the
  move payload (not by description edits); the old move closes `superseded`.
  (c) When a move closes, Home asks tasks to close the card only for cards Home filed
  (`mk-move` label); a stopgap card the vizier filed is left for its owner to close
  and shown "closed on Home, card still open" in Catch-up.
  (d) Home reads moves from the `moves` table first, so a v4 reader shows open moves;
  rollback to a v3 reader leaves `needs-mk` in place (we keep the label), so cards
  still appear as asks. Rollback is therefore safe but loses the Your move display;
  the install script's check mode states this.

### Dedup made fail-open (finding 6)

Suppression requires **all** of: exact same key (bead + subject slug, or PR URL, or
sha), the same option set (hash of option labels+kinds), no new fact (description
hash of the Request body unchanged apart from whitespace), and a prior ruling attested
as mk's pick in Home's own ruling record (not read from comment text). Comments are
never inputs to suppression; they can only add a note on the card. When any condition
is uncertain, file the ask and quote the prior ruling. A suppression with all
conditions met posts a "suppressed duplicate" notice to the owner thread (wake) and
shows as a collapsed row in Home ("Suppressed duplicates (n)"), not only Catch-up, so
a wrong suppression is visible where mk looks. The conversation section's "comments
never affect cards" stays true: the only comment-influenced behaviour is the
display-only note.

### Wake fallback transitions (finding 7)

Durable design: table `wake_route(obligation_id, attempt, target, state, created_at)`
with a unique key (obligation_id, attempt). On `undeliverable` the same transaction that
sets the original obligation `undeliverable` inserts attempt n+1 with the next target
(owner -> live title-successor -> vizier -> `pending-unrouted`), so a crash cannot
leave one without the other. A resolver failure leaves `pending-unrouted`, which a
retry loop (60 s, backoff to 15 min) re-resolves; it is shown in Your move as an
unrouted notice with age. Backfill covers **all** undeliverable obligations ever
recorded (not 14 days), bounded to 50 per run, oldest last, each routed once.
The existing short-circuit for already-undeliverable rows is changed to look up
`wake_route` first. Tests: crash between mark and insert (transaction), resolver down,
vizier is the gone recipient, no loops, backfill idempotent.

### Safe commands and links (finding 8)

`check` and `run` are never taken from card text. Home derives both from the validated
path only: path must match `^/[A-Za-z0-9._/+-]+$` (no spaces, quotes, `$`, backticks,
`;`, `&`, `|`, `<`, `>`, newline), no `..` segments; the command is built as
`bash <path>` and `bash <path> --check` with the path single-quote-escaped by the same
helper `rootrun.ts` uses. Any `check`/`run` member in a card is rejected as invalid.
The row verifies the live file's sha256 before showing a runnable command when the
file is readable from the host: match -> "hash verified on this host"; mismatch ->
"file changed since filed, do not run" and no copy box; unreadable -> "not verified"
(path and sha text only). `read.url_or_path` accepts only `https://github.com/...`,
`https://` links to hosts on a small allowlist in Settings, or an absolute path shown
as text; no other scheme is ever activated. PR URL pattern stays strict and the merge
method is displayed as text from the enum.

## Revision 2: answers to review r2 (FAIL, 7 findings)

Wins over earlier text. Principle adopted: **Home never lets unattested input make a
mk-owed item disappear.** Only independent external state (GitHub PR state) closes a
move by itself. Everything else moves a row to a visible "Reported done" state.

### Closure states (findings 1, 2)

States: `open` -> `claimed` -> `closed`. `claimed` = someone reports done (report-tell
claim, receipt file, or an unattested Done click). A claimed row stays in Your move,
under a "Reported done, not verified" group (collapsed, count in the header), with who
claimed it and when. It leaves only by:
- PR: GitHub merged/closed -> `closed` (independent evidence), or
- attested mk confirmation, when S-B finds attestation (outcome 1), or
- under S-B outcome 2 (no attestation): Home offers "Hide" (mk's own attention
  preference), which sets `hidden_by=ui` and moves the row to a visible "Hidden
  (n)" audit list; it never reports `closed` to anyone, never wakes or closes the card,
  and agent-originated calls to the same RPC are indistinguishable, so the audit list
  shows every hide with time. Receipts are claims, not closers, regardless of owner uid
  (finding 2); the `0644` mode applies to files, the dir is `0755`.
- A claim never wakes an owner to say "closed"; the owner's own words are claims too.
Replies/context (finding 1): under outcome 2 a reply is recorded as a comment labelled
"posted from Home (unattested)" and not as class mk; context-supply does not close a
`context` move; it moves it to `claimed`. No closing text or click is attributed to mk
as an authenticated act unless S-B proves attestation.

### Rollback (finding 3)

Withdrawn: "rollback is safe." True statement: a v3 reader hides picked cards, so
rollback loses the Your move display for open moves. Mitigation, in the PR: a
`moves-export` command (`bb home moves --json`, plus a markdown dump written to thread
storage by the install script's check mode and before every run) lists all open moves
so nothing is lost, and the rollback instructions say to read it. Also keep a
compatibility fallback: while any move is open, Home v4 writes the compact summary to a
single `needs-mk` stub card per project? Rejected: it would add filing noise. Decision:
export only, stated plainly in the install script header and handoff.

### Wake delivery path (finding 4)

No `wake_route` table. A route is a **new obligation row** in `obligations`
(`recipient = next target`, `parent_obligation_id = original`, `route_attempt = n`),
so the existing claim/send/settle machinery applies unchanged. In one transaction
(sync, as with the vizier resolver): mark the original `undeliverable`, insert the child
unless a child with the same (parent, attempt) exists (unique index). The target is
resolved *before* the transaction (async), then the transaction re-checks the original is
still the same state and the target id still equals what was resolved; if not, abort and
retry. A resolver failure inserts a child with `recipient = NULL, state = pending-unrouted`
that the existing due-scan skips and a 60 s retry loop (backoff to 15 min) resolves by
updating recipient in a guarded UPDATE (`WHERE recipient IS NULL`). Two delivered
attempts are prevented by the parent being terminal (`undeliverable`) before the child
exists and the unique index; a child can itself fall through to attempt n+1, capped at 4.
Migration adds the two columns and the index.

### Polling and ingest contracts (finding 5)

- Comments: `listComments` has no cursor, so cost is bounded by design: poll only the
  selected card (15 s) and cards with open moves or open asks (60 s, at most 25 per
  cycle round-robin, concurrency 2); stop polling closed cards. Replay-safe: comments
  are upserted by comment id into Home's local mirror table `card_comments`
  (id, task, author raw, time, body); a failed poll changes nothing and shows "comments
  may be stale".
- Sonnerie: removal before first poll cannot be recovered from the registry. So Home
  does not depend on Sonnerie for PR discovery: PR moves arrive via the ingest drop-box
  or the card; the Sonnerie registry is read only as a hint for PRs not already known.
  Statement of limit: a PR that is registered and terminal within one poll interval may
  never reach Home; this is acceptable because it was resolved before mk could act.
- Drop-box: writers publish atomically (write temp in `<dataDir>/home-inbox/.tmp`,
  `rename` into `home-inbox/new/`), file mode 0600 owned by the writer's uid; Home
  accepts only files owned by mk's uid (same-UID trust boundary, stated residual).
  Home moves a file to `processed/` only after the card write commits (ack), and a
  replay of a processed or half-processed file is idempotent by file id (content hash)
  recorded in a table. Unreadable or malformed files go to `rejected/` and are listed in
  Catch-up, never deleted.

### Dedup includes comments (finding 6)

Suppression requires, additionally: no non-Home comment on the sibling or the new card
newer than the sibling's ruling that is not itself the Request update. Any later
comment from anyone defeats suppression (file the ask, quote the prior ruling and link
the comment). This is conservative: a chatty card gets re-asked rather than suppressed,
one extra card, not a lost ask. Comments still never change card state; they only veto
suppression, which fails open.

### Rootless scope (finding 7)

Decision: unbound cards are **read and act-visible, not rulable**, in v1. Home shows
the full card text, the move (path, sha, URL), the conversation, and the owner link, but
shows no pick buttons: a pick writes through the root-dependent path. Row text: "Project
not bound: rule this on the card or add a root". Picks on these cards stay with the
vizier's delegated path or the task board. This removes the ambiguity: the earlier
phrase "full Ask rendering" means rendering, not picking. Binding a root later enables
picking without data change. Moves on unbound cards still work (they do not need the
root for evidence: GitHub state, claims).

## Revision 3: answers to review r3 (1 High, 2 implementation checks)

Wins over earlier text.

- **Unbound Asks cannot be ruled until a root is bound.** There is no card-level ruling
  path; a task comment or closing is not a Home ruling, and delegated `rule` needs a
  decision id and verified binding. So the row text becomes "Project not bound: add a
  root to rule this", and the vizier's escalation to mk lists the prefix. Closing the
  gap is therefore an operations item, not a code one: adding the four roots (UNC, INTV,
  ICOR, JAWN) to `autarch serve` is a mk-approved config change the vizier or I can
  file as a script; until then those cards are visible with their moves and
  conversation but not rulable in Home. Unbound *moves* (merge, run, read) still work
  because they need no ruling. Test: an unbound ask renders with no pick controls and a
  pick RPC for it is refused with a clear reason.
- **Migration test (wakes):** the v3->v4 migration must widen the `obligations.state`
  CHECK constraint to include `pending-unrouted` (table rebuild if SQLite requires),
  add `parent_obligation_id`, `route_attempt` and the unique index; test on a real v3 db
  with existing `undeliverable` rows.
- **Rollback export:** `bb home moves --json` is generated by the rollback procedure
  itself immediately before the plugin is reinstalled (the install script's rollback
  mode runs it first and writes it into thread storage), not only at install time; test:
  open a move after install, run rollback mode, assert the export contains it.

## Step 0 results (2026-10-06)

- **S-A confirmed and fixed** (db26eca): the backup gate hard-coded `min_reader_version <= 2`,
  so every backup of a v3 database was refused. It now allows a backup to demand no more than
  the live file does. The restore-script test that refuses a v3 backup is a separate v2-era
  tool and is unchanged. A v3 -> v4 migration test lands with the v4 migration.
- **S-B = outcome 2 (no attestation), confirmed in the SDK:** `bb.rpc.register` handlers
  receive only the validated input, no caller; the endpoint is served with "local" auth.
  Comments carry `kind` (user|agent|system), `authorName`, `threadId`, reported by the host and
  unattested. Consequences as planned: nothing but GitHub PR state closes a move by itself;
  author badges read "host reports: user" and the `UPDATE:` marker is disabled in Phase 1b.
- **S-C:** the plugin hears only `message.dispatched`, `message.cancelled`, `thread.archived`;
  no inbound message or comment events. Sonnerie keeps `registry.json` (`owner/repo#N -> thr`),
  read-only readable. Phase 1 therefore polls tasks (comments, cards) and GitHub; Phase 2 ingest
  uses the drop-box.
- **S-D:** `service.ts` materialises a card only after `projects()` resolves its root and it
  equals `project_root`; otherwise it stays `observed`. The unbound display record is new code.

## Revision 4: (button outlines, "Other" field, sequencing)

Wins over earlier text.

**Sequencing.** Phase 1 and Phase 1b ship together as the "work from Home" milestone, one PR,
ahead of Phase 2 and mk-okek.25. Build order inside it: parser + moves service, Your move UI,
conversation store and poller, Other field, option styling, then the whole-suite run.

**Option styling (UI only, no model change).** Every option is a bordered button with spacing, hover
and focus-visible rings; a distinct selected state; a marked "recommended" badge; options whose
`reversible` is false get a danger border and the text "cannot be undone". Tokyo Night tokens only.
Before and after screenshots (real rendered app) go in the PR.

**"Other" box on every card**, below the options, plain text, 2000-character cap, rendered as text and
never interpreted. Two actions:
- *Ask / note*: posts a comment "posted from Home (unattested)" and enqueues a wake by the 1b route
  (owner, successor, vizier, pending-unrouted). It never rules, closes or alters the card. This is
  exactly the 1b reply path; it needs no new mechanism.
- *Answer with this*: records a ruling with the reserved option id `other` and mk's text in the
  existing `picks.reason` column (cap raised from 1000 to 2000 by the RPC only; no schema change).
  `other` is a virtual option on every ask (not stored in `body_json`, so the decision identity and
  hashes are unchanged). Its obligation is a notify to the card's owner carrying the text, routed like
  any pick; it opens no move, and the owner answers in the conversation. A pick on `other` never opens a delegated-rule path and never counts toward auto-rule
  cooldowns (delegation ignores it).
- Unbound cards: Ask / note works (comments need no root); Answer with this does not (a ruling needs a
  root, as Revision 3).

**Review impact.** Ask / note is covered by the 1b review. "Answer with this" touches the ruling
path (virtual option, new obligation source, delegation exclusion), so it gets its own delta review
round (cross-lab, delta only: store.recordPick with `other`, obligation creation, delegation ignore,
replay idempotence by pick_id) rather than a full re-review. Tests: pick `other` replay, empty and
over-length text refused, text with markup and ANSI shown as text, delegation never fires on `other`,
unbound `other` refused.

## Revision 5: (copyable commands)

Joins the Phase 1+1b milestone. Wins over earlier text.

- **Source of every command is the structured move**, never card prose. For a `script` move Home derives,
  from the validated `path` and `sha256` (and optional `args`, each matched against `^[A-Za-z0-9._/:=+@-]{1,200}$`):
  1. `sha256sum '<path>'`, with the expected sha256 shown beside it (never truncated);
  2. `bash '<path>' --check`;
  3. `bash '<path>' [args]` (the real run);
  4. an optional recovery: `recover` is a second validated path (same rules, own sha), rendered as
     `bash '<recover>'`. Free-text recovery commands are not accepted.
  Single-quote escaping uses the existing `rootrun.ts` helper. Root runs keep their display-only rule.
- **Rendering:** one monospace block per command in run order, each with a label, a Copy button
  and "Copied" for ~1.5 s. The block is a single line, `white-space: pre`, `overflow-x: auto`, no wrapping,
  `user-select: all` on the code element so triple-click and drag selection both pick exactly the
  command; no prompt glyph, backticks or trailing space in the DOM text (a prompt, if any, is a CSS
  pseudo-element that is not selectable).
- **Copy:** `navigator.clipboard.writeText(cmd)` where `isSecureContext`; otherwise fall back to
  selecting the node's text range (`Selection.selectAllChildren`) and `document.execCommand("copy")`,
  and if that also fails leave the text selected and show "Press Ctrl/Cmd+C". The remote view is https, so
  the Clipboard API is the normal path.
- **Tests:** the string passed to the clipboard equals the derived command byte for byte (and equals
  what the move parser produced from the filed fields; including a path with a space-free but quote-risky
  character, an args value, and a recover path); no newline, no leading or trailing whitespace; the
  fallback path runs when `navigator.clipboard` is absent; a rendered-DOM test that `textContent` of the
  block equals the command. Screenshots (Playwright against the real rendered app, desktop and
  narrow width with a long command) go in the PR.
- No review round of its own: it is display-only derived from the already-reviewed move payload.

## Revision 6: (what mk did is not what the script did)

Joins the Phase 1+1b milestone. Wins over earlier text.

**Buttons say what mk is saying about himself, never the work's outcome.**
- script move: "I ran --check" (optional), "I ran it", "Later / skip". No "Done".
- pr move: "I'll merge it" (opens the PR link; records nothing as done) or nothing. "Merged" appears only from
  GitHub state, as before.
- read move: "I read it" (a claim by mk about reading). context move: reply box (Revision 4).
- A click records a claim in `moves.claimed_at/claimed_by` (state `claimed`, unattested as before). Skip sets
  `skipped_at` and moves the row under "Later" (visible, count in the header, never closes anything).

**The script's result is a separate field, shown separately.**
- After "I ran it" the row reads "Ran: waiting for the script's report" and records
  `report_deadline_at = claimed_at + the script's declared timeout (default 2 h)`.
- Sources of a report, in order: the script's report-tell comment on the card (parsed from the owner's
  agent comment), or its report.md path in thread storage (read-only, size-capped, text). Either sets
  `report_state`, `report_json` (outcome, failing step, error line, report link, time) and `report_at`.
  Neither closes the move: "Script reported success", never "Verified".
- `failed`: shows "Script reported failure at step N: <error line>" with the report link; the card stays
  open and the owner is woken once (deduped by the report id) so its next step appears in the conversation.
- No report by the deadline: `report_state = no-report`, text "No report received", and the owner is woken
  once.
- Owner or vizier verification evidence is added as a comment and rendered as such under "Evidence"
  (labelled by author, unattested), not as a state change.
- All report text renders as plain text and is capped (errors one line, 300 characters).

**Model impact.** The `moves` table gains six display columns (report_state, report_json, report_at,
report_deadline_at, skipped_at; in the unreleased v4 DDL, so no extra migration). The `open -> claimed -> closed`
machine, the pick model and the closer rules are unchanged: a report never closes a move. So no pick or move
model change; one more delta item for the cross-lab review (the report parser and the wake-once dedup).
Tests: claim then success/failure/no-report render the three states; a report never changes `state`;
spoofed report text renders as text; deadline wake fires once; skip never closes; buttons carry the
self-describing labels (a test asserts "Done" appears on no script card).

### Revision 6, addendum: reversibility and footer display (CLAV-70 screenshot)

A filed option's "reversible / not reversible" text describes the whole move, not the pick, so a "Not now" button read as irreversible. Display rules, which win over earlier wording:

- Reversibility is shown once, on the move itself ("This publish cannot be undone"), taken from the move, not repeated per button.
- A pick that only records what mk did or decided ("I ran it", "Not now", "Later / skip", "I read it") carries no reversibility badge. Only a pick that triggers an irreversible act (an instruction an agent will carry out, a publish) keeps the danger style and "cannot be undone".
- The internal option kind (`ruling-only`, `instruction`, `needs-context`) is never shown to mk.
- The footer "An instruction is sent to <thread>" shows only when the pick actually instructs an agent (kind `instruction` or `needs-context`), never for ruling-only picks.

No pick or move model change; this is rendering only, so it adds nothing to the delta review round beyond a UI test (a ruling-only option renders no badge, no kind label and no footer).

### Revision 6, addendum: clearing Stalled rows

mk had no way to clear "Stalled" rows (undeliverable notices and wakes to archived threads): the `dismiss` RPC existed but `ui/asks.tsx` rendered no button. Phase 1+1b adds:

- A Dismiss button on every Stalled row, calling the existing `dismiss` RPC (undeliverable to dismissed only; no change to that rule).
- The owner → title-successor → vizier → pending routing applies to Phase 1 wakes and notices too, not only Phase 2: an HTTP 409 "Thread is archived" re-resolves the recipient and retries to the successor instead of stalling. A row stalls only when no route resolves.
- Tests: a 409 to an archived owner retries to the live title-successor; no successor falls to the vizier; a Stalled row renders Dismiss and the click calls `dismiss`.

Routing is delivery plumbing: no pick or move model change, so no extra review round.

### Revision 6, addendum: one effect line per option

The "what the agent is told" collapsible is removed (it rendered an empty box for an empty instruction). Under each option the card shows one plain, never-empty line built from structured fields: "Tells <thread>: <first sentence of the instruction, at most 140 characters>" plus "Reversible." or "Cannot be undone."; for needs-context, "Asks you for the missing context first, then tells <thread>."; otherwise (ruling-only, or no instruction) "Records your pick only; nothing is sent." with no reversibility text. The per-option kind and reversible labels are gone. The "An instruction is sent to <thread>" footer shows only when some option instructs an agent. The full instruction text belongs only in the card's conversation/history view (1b). UI only; no pick or move model change; its tests join the delta round.

### Revision 6, addendum: reversibility stays on every option line (supersedes the "no badge" rule above)

Per mk (5d1a5e9), every option's effect line ends with "Reversible." or "Cannot be undone.", record-only options included ("Records your pick only; nothing is sent. Reversible."), from the filed `reversible` field. The move-level "This publish cannot be undone" line stays too. No `summary` field: the instruction's first sentence is the summary; revisit if filers' first sentences prove poor.

### Revision 6, addendum: a later ruling supersedes the every-option reversibility rule

A later mk ruling ("it adds a lot of noise") supersedes 5d1a5e9's every-option rule and the previous addendum. A reversible option's effect line carries no reversibility text. Only an option that cannot be undone ends with "Cannot be undone." This holds for record-only options too. The move-level "This publish cannot be undone" line is unchanged. UI only.

## Provenance: who each rule comes from

Added for mk-o06o.4. Every rule in this plan and its addenda carries one source tag. A claim of mk's intent needs a ruling ID and mk's exact words. The ledger has no `quote` for rows before the provenance ruling (`quote: null` means unsourced wording), so for those the only mk words below are the ones the vizier relayed in quotation marks, marked "relayed". Anything not an exact mk quote is `vizier` (the vizier's reading or relay, including its numbered requirement lists) or `agent` (this thread's design choice). Where this section and an earlier line disagree, this section wins.

| Rule | Source | Ruling | mk's exact words (if any) |
|---|---|---|---|
| Home is the place mk works from; a pick that leaves mk a task stays on the card until evidence closes it; Phase 1 and 1b are one milestone ahead of Phase 2 and mk-okek.25 | mk, then vizier | ledger rulings | relayed: "i want home to be the true home of my attention so i am never blocking work" |
| Card conversation, comments mirrored, replies wake the owner (Phase 1b) | vizier (requirement list) | ledger entry | none in the ledger |
| Phase 2 auto-ingest and dedup | vizier | ledger entry | none in the ledger |
| Nothing but GitHub PR state closes a move; the rest is "Reported done, not verified"; rollback exports moves first; vizier-tell drop-box via dotfiles PR; roots script | vizier | ledger entry (by=vizier) | none (not an mk ruling) |
| Better button outlines; an "Other" box | mk (request), vizier (requirements: hover, focus, selected, recommended badge, danger style, "Ask / note" and "Answer with this") | ledger ruling | relayed: "should we have better button outlines on the home cards? it's a bit confusing and i also wish there was an "other" text field or something where I could ask you a question or leave a note" |
| Copyable commands: one block per command, one-line Copy, run order, byte-for-byte test | mk (request), vizier (requirements) | ledger ruling | relayed: "also it's weirdly hard to copy script commands, which sort of defeats the purpose" |
| Claim vs report: "I ran it", no "Done" on script cards, the result comes from the script's report | mk (request), vizier (requirements) | ledger ruling | relayed: "it's weird in the home task card to press a button that says done when me running a command and saying i did that when that's not the same thing as the script running succesfully" |
| Reversibility shown once on the move, record-only picks carry no badge (CLAV-70 addendum) | vizier (from mk's screenshot; no words) | none | none |
| Every option line says reversible or not reversible (the 5d1a5e9 "again" rule and the addendum that kept it) | agent (inferred from a commit message); superseded | superseded by a later ruling | none; mk did not ask for it |
| Reversible options carry no reversibility text; only "Cannot be undone." | mk | ledger ruling | none in the ledger (quote: null) |
| One effect line per option; the "what the agent is told" box is removed; first sentence of the instruction is the summary | vizier (relay of mk's feedback, no words) | none | none |
| Stalled rows get a Dismiss button; 409 re-routes to the successor | vizier (relay of mk's observation) | none | none |
| Build the provenance safeguards (mk-o06o) | mk | ledger ruling | "yes do all 5 in parallel" |
| Design choices of this thread: no `mk-move-open` label, no `then: "mk-acts"` option marker, `moves` table shape, closer rules, command derivation | agent | none | none |
