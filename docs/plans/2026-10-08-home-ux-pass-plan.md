# Home UX/UI/design pass: one count, no hidden state, runnable commands (bead mk-yjp7)

Status: draft for coordinator review. Base: a6621fc (home-mobile merged). Plugin: `integrations/bb-plugin-autarch`.
UI-only plus additive read RPCs. No schema change, no migration. Source: bead mk-yjp7 and mk-7209 (the evidence below is from them).

## Findings (reproduced from the code before any fix)

1. **"mark all seen" is a silent no-op.** `snapshotIds` (`ui/catchup.tsx:82`) sends only items that are expanded AND on screen; `expanded` starts empty, so the first click sends nothing, calls no RPC and says nothing. Routine groups also cannot be "seen" honestly: expanding one shows only `cites`, never the updates inside it.
2. **The sidebar badge is a different number from every list.** `HomeBadge` (`app.tsx:568`) adds unseen catch-up items to owed asks (8 = 5 + 3) while Decide says 5, and says nothing about the 3. The badge counts held asks too (`asks.owed.length`) while Decide hides them. "Your move" items are in no count at all. The Queue tab, the section headings, the overlay strip and `bb home stats` each count their own thing.
3. **Important state hides in a collapsed Catch-up row.** "Home adopted <thread> as the vizier thread; delegated rulings stay suspended until you see this" is a `pinned()` item (`delegation.ts:318`) shown only as a collapsed row under "Since you left", at the bottom of the Queue. Delegated `rule` stays refused while it is unseen (`suspended()`), yet nothing on Home's first screen says so.
4. **Commands in a card are prose.** `CommandBlock` (`ui/copy.tsx`) exists, but only a structured script move uses it. A card filed as free text (AUTA-133) shows `bash /…/x.sh --check`, then "then", then `bash /…/x.sh --apply --accept-install-diff` inside one `whitespace-pre-wrap` paragraph; only the path is a chip. Flags are not marked and the two commands look alike, which is how an `--apply` ran without its `--accept-install-diff`.

## Changes

### C1. One count: "Waiting on you"
New pure module `waiting.ts` (server) + RPC `waiting` + field in `bb home stats`. One definition, used by the badge, the Queue header, the tab, the overlay and the CLI:

    waiting = live owed decisions (not held)
            + open "Your move" rows (unclaimed, not skipped, not hidden), de-duplicated by task_id against owed decisions
            + 1 per unseen item that suspends delegation (vizier adopted / delegation settings changed)

Catch-up reading (failed runs, undeliverable messages, delegated rulings, notes, routine groups) is **not** in that number; it is shown beside it as a separate, named figure ("3 updates to read"). Held asks are shown as "N on hold", never counted. The badge shows `waiting`; the "!" (serve not ready / unowned machine blocker) stays. Queue header, one line under the tab row: `Waiting on you: 7 — 5 to decide, 1 move, 1 notice · 3 updates to read · 2 on hold`. Each part is a link that scrolls to its section. The Queue tab reads "Queue (7)".

The vizier's list disagreed with Home; I cannot see how that list counts. `bb home stats` will print `waiting` with its parts, so the list can cite one definition. **Open question for the coordinator:** what does the vizier's list count (which cards, which states)? If it differs from the definition above (for example it counts claimed moves, or on-hold cards), that is a ruling, not something to guess.

### C2. Hidden state moves to the top
A "Needs your eyes" banner above "Needs you now" for pinned items that change what Home does: today the vizier-adoption and delegation-settings notices. It states the consequence ("Delegated rulings are suspended"), names the thread, and has an explicit **Acknowledge** button (calls `markSeen` for that item, an affirmative click). Not auto-marked by dwell: acknowledging lifts the suspension, so it should be deliberate. The item leaves the catch-up list once acknowledged; before that it is not duplicated there.

### C3. Catch-up that does what it says
- Each unread row (not owed) gets its own **Mark seen** button, visible without expanding. It marks only that row.
- "mark all seen" becomes `Mark N seen`, where N = rows it will mark. A non-routine row is markable when it is on screen (its full text is shown); a routine group only when expanded (its member lines are then listed: new optional `lines` on the catch-up item, built from titles the store already has). The button is disabled with a visible reason when N = 0, and after a click it reports `Marked 3. 1 routine group left: open it to mark it.` Never silent. Server `markAllSeen` is unchanged and still validates every id.
- The D-12 rule becomes "marks exactly what the panel showed in full on screen, and says so".

### C4. Cards: commands you can copy whole
`ui/commandtext.ts` (pure) splits a card's question text into prose and command lines (a line that starts with `bash`, `sudo`, `runuser`, `scp`, `ssh`, `sh`, `python3`, `gh`, `bb`, `bd`, `autarch`, `sonnerie` or `$ ` and has an argument). In `AskCard` each command run is a `CommandBlock`; paragraphs get spacing. When there are several commands, the last is labelled **Final command** and shown first-class (primary button); earlier ones are labelled "Before it (dry run / check)" when they carry `--check`/`--dry-run`, else "Earlier step". Flags (`--apply`, `--accept-install-diff`) are bold in the block, and a line under it lists them: "Flags in this command: --apply --accept-install-diff. Copy the whole line; the Copy button includes all of them." Copy still puts exactly the line on the clipboard. Structured script moves keep their existing steps but get the same flag emphasis. Nothing is ever executed (existing rule D3 of the Your-move plan).

### C5. Smaller things found while in there
- Section headings in the Queue carry counts; the stray orphan doc comment before `OverlayPage` (`app.tsx:544`) is fixed.
- The overlay strip gains `Waiting on you: N` (same function).
- Not done (named so they are not lost): collapsing very long card questions; a search/filter for Catch-up; the vizier-list reconciliation above.

## Tests (written first; each fails on a6621fc)
- `snapshotIds`/mark-all: nothing expanded → button disabled with reason, and one click marks the on-screen non-routine rows; routine group only when expanded.
- `waiting.ts`: 5 owed + 1 held + 3 unseen catch-up + 1 adoption → 6, parts as above; a move on a card with an owed decision counts once.
- Badge === Queue header === `stats.waiting` over the same fixture.
- Adoption notice: appears in the banner, not in the catch-up list; Acknowledge → `markSeen` and the suspension lifts (`suspendedNow()` false).
- `commandtext`: AUTA-133's text yields prose / check / final; flags extracted; a prose line starting with "run" is not a command; copied text is byte-exact.
- Mobile: 390x844 and 430x932 harness metrics: no page overflow, tap targets ≥ 44px for the new buttons.
- Run: `npx vitest run` and `npx tsc --noEmit` in the plugin. No Go changes planned; if any, `go test -race`.

## Screenshots
Harness `docs/plans/screenshots/home-mobile/harness` extended with a new fixture set (invented project and thread names, no hostnames or private paths): Queue header with the count, banner, catch-up with Mark seen, a card with a free-text command. Before/after at 390x844 and 430x932 into `docs/plans/screenshots/home-ux/`.

## Rollout
UI plus read RPCs only, deploy from merged main by the reviewed Home deploy command (q509/q510); no schema change (q712 B). Live-data preview only after a backup. I do not merge, deploy or tag.
