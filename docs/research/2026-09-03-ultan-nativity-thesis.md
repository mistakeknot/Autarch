# Ultan — the nativity thesis for the estate's own graph

```yaml
artifact_type: design-record
evidence_class: primary (mk's rulings of 2026-09-03, each given through a structured question, one at a time; CanonGraph source read at commit 28f072d; the sylveste profile queried live the same morning)
goal: intercore fdaae66d "Write the nativity thesis for the estate's own graph"
spec: docs/cujs/autarch-03-seasonal-reshaping.json (the must_stop this record closes, at v1.4)
ruled_by: mk
ruled_at: 2026-09-03
transcribed_by: "Claude Fable 5.1 (claude-fable-5-1)"
session_id: "5920c9b1-6a3f-4a7d-8566-e6067aaeaf01"
rulings: 9 (one on the pain, seven structural, one amendment surfaced mid-interview, one name)
recommendations: kept out of this record. Each question carried one, marked as such; the record holds what mk chose, and marks the one place mk chose otherwise.
```

## What this record is

Three of Autarch's four journeys lean on a graph of the estate: the walk's garden rows and theme pivot, the seasonal reshaping's layer view, canon tending's discourse routing. Today that graph is CanonGraph's `sylveste` profile, and on 2026-09-01 mk ruled that it is transitional: "i want to get out of canongraph and build our own, better version of canongraph that is fully our own", and in the same conversation, "CanonGraph seems to be working, but it's my colleague's and I would rather make a version from the ground-up that is informed by Sylveste's philosophy and is native to Sylveste/Clavain/Intercore/the Interverse/Autarch/Garden Salon/Meadowsyn/etc". The autarch-03 spec turned that into its one `must_stop`: the build is gated on a thesis saying what a Sylveste-native graph must model that a generic knowledge graph structurally cannot. This is that thesis. It is argued against the running system and its source, not from the armchair, and every structural claim in it was put to mk as a question before it was written. Building the graph is out of scope here; this record is what the build waits on.

## The pain, in mk's words

The GATE asked that mk articulate the pain before anything was drafted. The question put the live profile's shape in front of them (seven entity types, twelve edge types, ten named queries, 51 projects on layers A through X, five serving edges, eight decisions about canongraph itself, all theirs, all `decided`) and offered four moments the material suggests. mk's answer was **"all of the above"**, so all four stand as the pain:

- **Shape: catalog, not canon.** It answers where a project lives and which layer letter it carries, but a theme, a card, a stamp or a season has no node to land on; the estate's structure stays in mk's head and in `docs/why.md` files.
- **Decisions are flat.** Eight decisions, each a title and a rationale string: none supersedes or contradicts another, none points at the artifact it ratified, the sitting is a bare run id. The trace of governance is metadata on the fact, not the fact's form.
- **Reaching it is the friction.** zklw-only over a bearer-token MCP, the Kùzu single-writer lock forbids the CLI, capture is a skill mk must remember to invoke, and the lane doctrine is a markdown rule the graph cannot enforce.
- **Not in the graph at all.** It works. The pain is that it is a fourth memory system with its own shape beside cards, beads and the salon, instead of the one world model that intercore, Autarch and the salon all read.

Read with the 2026-09-01 ruling, the pain is not a defect list. It is that the graph's shape is somebody else's answer to a different question, and parity with it is the floor, not the goal.

## CanonGraph as it is, read rather than remembered

**The live profile.** Topology `sylveste`: Person, Machine, Project, Plugin, Client, Decision, Run; twelve relationships, each between one fixed pair of types; ten named cypher queries, no other query path. Projects by layer on 2026-09-03: A 9, B 5, C 8, D 5, E 4, F 3, G 0, H 3, I 1, J 3, K 1, L 0, X 9, total 51. Five `serves` edges, every one on zklw. Eight decisions concern canongraph, every one `made_by: mk`, every one `decided`, none pointing at any other. Zero decisions concern Autarch, and Autarch has no Project row at all: the HUD that reads the graph is absent from it. The catalog fields (designation, layer, ecosystem, constellation) arrived by an additive `extend` on 2026-07-15, the only schema change the system permits.

**The source, at 28f072d.** Six limits that are architecture, not missing features:

1. **No supersession or retraction.** Replay merges each new payload's properties over the existing row, last write wins (`canongraph/backends/sqlite_log_store.py:256-267`). The only removal is physical deletion by redaction (`sqlite_log_store.py:173-191`). Nothing can say "this was true and is no longer".
2. **Flat scalar property bags.** Entities and relationships each hold one flat list of properties; the optional type hint is limited to STRING, DOUBLE, INT64, BOOL (`canongraph/topology.py:17-28, 145-148`). No nesting, no enums, no validity interval, no time-varying value.
3. **Binary, singly-typed edges.** A relationship is declared from one entity key to one entity key (`topology.py:17-28`). It cannot reach "whatever a theme concerns", cannot point at another relationship, cannot carry an interval.
4. **Exact identity.** `resolve` lowercases and folds whitespace and then matches exactly (`canongraph/backends/log_base.py:216-236`). One name per entity, no aliases; the 2026-07-14 decision "zklw spelling wins" exists because the system cannot hold two spellings.
5. **Asserted actors, one token.** `actor` is caller-supplied text that defaults to "assistant" (`canongraph/server.py:187`); one shared bearer token covers every writer (`server.py:450-485`). Nothing in the store knows who mk is.
6. **`verified` is a boolean nobody reads.** It is a column on the event log (`log_base.py:25`) with no read path, no workflow, no state beyond true and false.

And around them: queries run only from the topology's declared catalog (`server.py:218-227`); `search` is a separate vector index over fixed 1100-character chunks (`canongraph/chunk.py:3-16`, `server.py:337-355`) whose passages touch the graph only when a caller passes explicit links; two profiles cannot see each other (`canongraph/profile.py:200-207`); `extend` is additive only, never rename or retype (`canongraph/migrate.py:21-89`); the embedded Kùzu store is single-writer and file-locked, so a CLI and the service cannot both hold `graph.kuzu` (`canongraph/viewer.py:9-11`), which is why capture moved to HTTP and why the estate's standing rule is never to run graph commands against the live service. `canongraph export --format json|markdown` exists (`canongraph/cli.py:139-146, 370`) but opens both stores (`cli.py:322-326`), so it runs against a stopped service or a copied profile; only the viewer's log-only path reads beside a running one.

## What Ultan must model: the seven rulings

Each ruling below is mk's, given 2026-09-03 in session 5920c9b1-6a3f-4a7d-8566-e6067aaeaf01 by choosing among concrete options put one at a time, and transcribed by Claude Fable 5.1. The chosen option's wording is the ruling; the structural consequence and the reason a generic graph cannot carry it follow.

### 1. Files are truth; Ultan is an index

**Ruled:** every estate fact has a git home mk owns: cards (`docs/why.md`), CUJ specs, decision records, and a theme file per theme. The graph is compiled from them plus live sources (tmux, beads, the salon) and can be deleted and rebuilt losing nothing. A write through Autarch is a stamped file edit; the graph re-indexes. Chosen over "graph is truth for structure" and "a ledger of stamped acts is truth".

**Consequence:** Ultan is a projection, never a store of record. Its own database is a cache. Sovereignty is structural: the record is in mk's repositories, versioned and diffable, and the graph can be replaced without loss. CanonGraph cannot be this: its truth is its own append-only log, files are ingested into it, and nothing reads back out (limit 1; the export path exists only as a dump).

### 2. The ruling is the unit of a write

**Ruled:** every fact carries who acted (mk, or a named agent with session id and goal) and a state: proposed until mk stamps it, ruled when mk acts directly. Proposed facts are indexed and shown as provisional, exactly as card fields are today, never silently canon. The graph marks them; Autarch renders them; only mk can move one to ruled. Chosen over "agents write canon directly" and "agents never touch canon files".

**Consequence:** attribution and state are the form of the fact, not metadata beside it (Garden Salon's Trace-as-Form, already the card's `ratification` block). The identity is real, not asserted: mk's stamp is mk's, an agent's proposal names the agent, session and goal. CanonGraph's actor is a free string defaulting to "assistant" behind one shared token, and its only approval field is a boolean nothing reads (limits 5 and 6).

### 3. Theme and salon are both stored

**Ruled, against the recommendation:** the theme aggregates; each salon sitting is its own record (participants, start, end, what was absorbed) linked to the theme. Heavier, but the sitting history is queryable from the graph without the salon substrate. The recommendation had been a theme object with the salon as its unstored live view; mk chose the stored sitting.

**Consequence:** the theme is the estate's missing object, given a home: a first-class node with one polymorphic `reaches` edge into any project, platform, plugin or decision it concerns, its own canon, open questions and cadence, and a sitting record per salon that carries what the theme absorbed at parking (mk, 2026-09-01: parking is the theme absorbing what the session learned). With ruling 1, the sitting needs a file home too; that is opened on the spec, not decided here. CanonGraph cannot declare the reach edge at all: every relationship is from one type to one type (limit 3), and a theme's membership is exactly the edge whose target type varies.

### 4. Pace is a property with time

**Ruled:** any node (project, platform, theme, decision) carries a layer letter, an expected revisit cadence, and a last-tended timestamp written by parkings and stamps. Season boundaries and "gone dark" are then computed from the graph, and a fast probe sedimenting into doctrine is a layer change with a stamp, not a move in a tree. Chosen over "layer letter only, cadence local" and "layers are nodes".

**Consequence:** pace is an attribute axis, as the ontology note sketched, and it has time in it. Cadence and last-tended are typed temporal values on every kind of node, and the walk's "idle for days" and reshaping's "season boundary" become graph reads. CanonGraph carries a layer string on Project only, no time on anything but a decision's `decided_on`, and can type a property as nothing richer than a scalar (limit 2).

### 5. Two routers, delivery classes on the edge

**Ruled:** a finding routes topically to every theme whose reach it touches, and structurally along serving and containment edges, upward into fast layers and downward toward doctrine. Each edge kind carries a default delivery class as a graph property (a contradiction inside a theme's reach is popup class; a cross-garden echo is toast), and the in-context retune of autarch-03 rewrites that property as mk's stamped act. Chosen over "two routers, classes local" and "theme router only".

**The amendment this forced.** This ruling contradicted autarch-03's closed decision of 2026-09-01, which kept "local prefs (delivery classes, ranking)" in Autarch's config. The contradiction was put to mk as its own question rather than reconciled quietly. **mk: "Amend 03: classes are world facts."** The default class lives on the edge kind in the graph; an in-context retune is a stamped world edit like any other; Autarch's config keeps display preferences only (pins, layout, last visit). Ranking was not re-ruled and stays local. The older decision stays on the spec and is pointed at by the amendment, not rewritten, which is ruling 6 applied by hand a minute before it was made.

**Consequence:** routing is graph-native and shared. The salon sees the same classes Autarch does, because the class is a property of the world, not of one client. CanonGraph has the serving edges and the layer letters but no theme, no containment edge distinct from hosting, and no property on an edge kind that a client could tune.

### 6. Decision edges, contradiction proposed

**Ruled:** decisions are first-class with lifecycle edges: supersedes, amends, contradicts, depends_on. Scope is the decision's reach edges (project, theme, platform). An agent pass over decisions sharing scope proposes a `contradicts` edge; it stays proposed until mk stamps it or rules which stands, and the older decision is never overwritten, only pointed at. The graph never resolves a contradiction itself. Chosen over "computed only, never stored" and "supersession chain only".

**Consequence:** a decision has a lifecycle and a neighbourhood. Detection is an agent's proposal in the sense of ruling 2, and resolution is mk's stamp. CanonGraph's decisions have no edge to any other decision, and its replay would have overwritten the 2026-09-01 ruling with today's, silently (limit 1).

### 7. Parity is the ten queries; migration is export to files

**Ruled:** the parity floor is the ten named queries answered from Ultan with matching rows. Migration: the event log is exported into the files Ultan indexes, a catalog file per layer for the 51 projects (designation, layer, status, ecosystem, hosting, serving), decision records carrying `made_by` and `decided_on` as ruled by mk with source "migrated", runs left to intercore. CanonGraph stays read-only until the ten match, then the service stops. The lane doctrine keeps the graph lane's litmus test under the new name. Chosen over "re-derive from live sources" and "run both, split the lane".

## Why a generic knowledge graph structurally cannot

A generic knowledge graph's primitive is the asserted fact: a typed node or edge with provenance beside it. CanonGraph is a good one of these, and the eight decisions mk made about it are decisions about deploying it well, not about its shape. Ultan's primitive is the ruling: an act by an identified actor, in a sitting, under a goal, in a state that only mk can advance, pointing at the rulings it amends or contradicts, on a node that carries pace and time, recorded in a file mk owns and indexed rather than stored. Each of the six limits blocks one part of that primitive. No retraction blocks lifecycle (rulings 5 and 6). Flat scalars block cadence and time (ruling 4). Singly-typed edges block the theme's reach (ruling 3). Asserted actors and the unread boolean block stamps (ruling 2). A log that is its own truth blocks the index (ruling 1). And additive `extend`, the one change the system allows, can add a Theme type but cannot add a polymorphic edge, cannot give events a lifecycle, cannot authenticate a writer, and cannot turn the log into a projection of files. These are the architecture, which is why parity is the floor and not the destination.

The claim is about CanonGraph's architecture and its class, not about graphs. Ultan will itself keep a graph as its projection; what makes it native is what the projection is of and what a write is.

## Where Ultan sits

In the containment tree Ultan is a platform beside intercore, as the ontology note sketched: the long-memory and canon layer of the native stack, under the salon substrate's live state and Autarch's terminal client. Autarch and the salon are its two readers; intercore is the writer of runs and sittings; the interverse plugins are its capture path, replacing the canongraph capture skill; the estate's repositories are its truth. Meadowsyn's role relative to it stays unplaced, as before.

## Parity and migration path (ruling 7, spelled out)

1. **Freeze.** The `sylveste` profile becomes read-only in practice: capture stops writing to it once Ultan's proposal path exists. Recall keeps reading it.
2. **Export.** `canongraph export --format json` against a copy of `~/.canongraph/sylveste` on zklw (the command opens `graph.kuzu`, so not beside the running service), or a direct read of `log.sqlite`, the viewer's path, which is safe beside it. The export carries every entity and relationship with `source` and `confidence`; the log carries `actor` and `ts` per event.
3. **Files.** A catalog file per layer for the 51 projects, holding designation, layer, status, ecosystem, constellation, hosting and serving; a decision record per decision with `made_by: mk` and `decided_on` preserved and `source: migrated from canongraph <event_id>`; runs are not migrated, intercore owns them. Where these files live is opened on the spec (provisional: the Sylveste monorepo, where the lane doctrine already lives).
4. **Parity.** The ten named queries run against both sides, rows diffed; both reads are read-only. The floor is met when every query matches for every parameter the estate uses (each layer letter, each machine, each project with decisions).
5. **Cutover.** The recall hook's graph step, the capture skill and Autarch point at Ultan; `canongraph.service` on zklw stops; `ops/canongraph/memory-lanes.md` renames the lane and keeps its litmus test; `recall-lanes.md` rewrites row 1.
6. **Upstream.** The 2026-07-14 decision "Adopt CanonGraph as tracked upstream" is superseded by a new decision that points at it (ruling 6's first real use). The PR watch on jvattimo1/canongraph is a courtesy and can continue.

## Open questions derived here, not ruled

Recorded on autarch-03 at v1.4 as open questions, so the build cannot silently invent them:

- The sitting record's file home (rulings 1 and 3): a sittings log inside each theme file, or a sittings directory beside the theme files.
- Which repository holds the canon files Ultan indexes.
- How a proposed, unstamped fact is written in a file: the card format's per-field state extended to decisions, theme members and sittings is the candidate.

Not opened, because they are build questions and the build is out: Ultan's projection store, its query surface, and whether recall's semantic document lane moves with it or stays separate.

## The name

**Ultan**, mk's pick from a shortlist of four two-syllable character and place nouns, each checked for collision against `~/projects`, the Sylveste apps, os and interverse trees, mk's 237 GitHub repositories, and the sylveste profile itself (all four resolved as new). The others were Pascale (Reynolds: Sylveste's biographer), Cuvier (Reynolds: the capital of Resurgam) and Kabe (Banks: the Homomdan chronicler of the Culture). Ultan is Wolfe's: Master Ultan, the blind curator of the Library of Nessus, who knows where every book stands without owning a word of them. Same shelf as Autarch. An index over cards it does not hold.

## The recognition check

Owed to mk after reading, never automated: does this read as the graph you meant when you said "fully our own"?

## Out of this record

Building Ultan, migrating the profile's data, salon substrate work, the waiting-on-me axis, and the stamp flow, each its own goal.
