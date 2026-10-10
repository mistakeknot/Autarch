# Home Idea box and weekly idea digest (bead mk-2zojo)

Source: bead mk-2zojo (hub tracker), approved 2026-10-10 (the bead holds the ruling and mk's words): Home gets an Idea box
(pick a project, type one line; Home files an idea in that project's tracker labelled `idea` and `from-mk`
with mk's words verbatim and tells the project's coordinator; an idea is not work until it is picked) and a
weekly digest card listing open ideas, each with pursue, park or drop. Seed idea: mk-3f0j.9.

## Design choices (mine, not rulings; each is flagged to the vizier)

1. **An idea is a tasks-plugin card in the project's tracker**, created over `plugins.callRpc` (`createTask`,
   `createLabel`), as Home already does for reads and comments. The plugin still never spawns `bd` or the bb CLI.
   Labels `idea` and `from-mk` are created in the project if missing. The description is mk's words verbatim plus a
   `home-idea: <id>` marker line, so a retried file finds the card it already made instead of making a second one.
2. **No schema change** (Home Plugin Deploy scope: no database schema or migration change). Ideas live in the tracker; the only Home state is two
   `settings_kv` values: `ideaFiled` (idea id to task id, for idempotence) and `ideaDigest` (the ISO week last
   cleared). Review is told to confirm no migration is touched.
3. **Telling the coordinator** reuses the wake loop: one `notice` obligation per idea (exactly-once, reconciled),
   keyed `idea:<task id>` in `decision_id` (the column has no foreign key). Recipient is the project's coordinator
   when a `projectCoordinators` setting names one, else the vizier thread, which relays. There is no UI to set that map
   in this change; the vizier fallback is the default. This deviates from "tells the coordinator" until the map is
   filled, and is raised as a question.
4. **Pursue / park / drop** each post a Home comment and never close anything the coordinator owns: pursue adds
   the `pursue` label and tells the same recipient (so it can be picked as work); park adds `parked`; drop sets
   status `canceled`. The `idea` label stays so the card can be found; the second label takes it off the open list.
5. **The digest** is a section on the desktop queue and in the phone Idea view while the current ISO week is uncleared
   and at least one idea is open. It is shown beside the Waiting count and is not in it, so the count keeps its
   defined meaning. "Not this week" marks the week cleared. The Idea box sits in the same place.
6. **Phone layout** keeps the three tabs mk chose; the Idea box is a header button beside Full view.

## Tasks
1. `ideas.ts`: pure helpers (marker, week key, listing, label names) + tests.
2. Service: `fileIdea`, `ideas`, `ideaAction`, `ideaDigest`; RPC methods in `contract.ts`; tests with a fake tasks client.
3. UI: `ui/ideas.tsx` panel (box, list, actions), digest row in Waiting, phone header button; render tests.
4. Verify: tsc, full vitest -race-equivalent, build; cross-lab review; PR; vizier merges; deploy as a Home Plugin Deploy with backup.
