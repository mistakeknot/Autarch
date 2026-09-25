# Autarch inside Aleph, and Aleph as a Go or Rust port — options

**Date:** 2026-09-24 · **Status:** exploration, no code · **Decider:** mk

> **Superseded in part (2026-09-24).** mk ruled that the goal is one place
> to direct attention, not a merger. Mycroft does not enforce Aleph's
> roadmap. See [One place in Aleph](2026-09-24-one-place-in-aleph.md).
> Question 2 (no port) still stands.

## What is true today

- **Aleph is mk's fork of bb**, not a separate product: `mistakeknot/aleph`,
  version `0.43.4+aleph.1`, merged from `get-bb/bb` rather than rebased. Its
  mission is making coordinator sessions spend their usage on progress. Its
  philosophy says the fork stays *thin and current*
  (`~/projects/Aleph/FORK.md`, `PHILOSOPHY.md`, `docs/aleph-roadmap.md`).
- **Size:** about 540k lines of non-test TypeScript. That is the app (216k),
  server (91k), bundled plugins (147k), host daemon (24k), db (21k) and CLI
  (19k), plus smaller packages.
- **Upstream moves fast:** about 1,030 non-merge commits on `upstream/main`
  in the last 30 days.
- **Plugins are TypeScript that bb executes.** Each has an `app.tsx`, and
  optionally a `server.ts` or `host.ts`. They can provide RPC handlers, HTTP
  routes, agent tools (`registerTool`), commands, thread actions and mention
  providers, and host code can spawn processes (`github`, `keep-awake`). Provider bridges are
  exports inside a plugin's `bb.host` artifact, speaking the provider bridge
  JSON-RPC protocol. Aleph's SDK is `@bb/plugin-sdk` 0.5.24. Full-page
  panels (`fixedTabs`, `experimental_useAppPanel`) are still marked
  experimental.
- **Autarch** is about 130k lines of Go: Bigend, Gurgeh, Coldwine, Pollard,
  Mycroft, the door/catch-up and the registry. Its interfaces are Bubble Tea
  TUIs and an htmx web UI. Mycroft spawns agents through tmux
  (`internal/mycroft/spawn`).

## Question 1: Autarch inside Aleph

The rejected proposal, one Autarch nav panel, kept Autarch as a guest in
someone else's house. The ambitious version makes each tool part of how Aleph
already works.

| Option | Shape | Cost | Verdict |
|---|---|---|---|
| **A1. One plugin page** | A nav panel that embeds or re-renders Autarch | Low | Judged too timid |
| **A2. Autarch as Aleph's orchestration layer** *(recommended)* | Each tool becomes a native bb surface. The Go engine runs as a sidecar that a plugin's `host.ts` starts, reached over RPC | Medium, per tool, with the Go logic reused | See below |
| **A3. Rewrite Autarch in TypeScript inside Aleph** | Drop Go | ~130k lines rewritten | Not worth it; A2 reuses the logic |
| **A4. Fold into Aleph core as fork patches** | Autarch in `apps/` or `packages/` | High, and it recurs on every upstream merge | Contradicts the thin-fork rule |

**A2 in detail. Where each tool lands:**

- **Mycroft becomes Aleph's dispatcher.** It stops spawning through tmux and
  starts bb threads. Those threads inherit the account pool, the concurrency
  limit, receipts and provider switching. Its T0–T3 autonomy ladder is the
  enforcement layer Aleph's roadmap is missing: wakes only for news, bounded
  review rounds, cheapest adequate model. This part carries the most leverage.
- **Coldwine becomes the sprint and outcome view.** Epics and runs map onto
  Aleph's planned outcome tag: a declared unit whose children and usage
  inherit its ID. Coldwine's run board becomes the operator view of usage per
  outcome.
- **Bigend and the catch-up/estate map become a fixed tab and sidebar
  badges.** They show what changed across projects since you last looked,
  with bb threads as a source. The `feat/bb-catchup` branch already reads bb
  threads.
- **Gurgeh becomes a thread action and a PRD panel:** "turn this thread into
  a PRD", with validation shown as the panel.
- **Pollard becomes agent tools (`registerTool`) and a research panel**, so
  any thread can call a hunter.
- **The TUIs stay** for terminal use and share the same engine.

**Risks.** The experimental panel APIs can change with upstream. Mitigate by
keeping each panel thin over RPC, so the Go engine absorbs little churn.
Folding Mycroft into bb dispatch changes its trust boundary: bb threads run
with whatever access their environment gives them. The overlay rule (Autarch
writes no world state) has to hold for the plugin too.

## Question 2: Aleph as a Go or Rust port of bb

| Option | What it means | What it buys | What it costs |
|---|---|---|---|
| **B1. Full port** | Rewrite all ~540k lines | One language, one static binary | Person-years. Every bb plugin is lost, including Aleph's own account pool. The port can never merge upstream again, and upstream changes ~1k commits a month |
| **B2. Port the backend, keep the React app** | Server, host daemon, CLI and db (~155k lines) in Go or Rust, behind `server-contract` | Lower memory, and no Node or native add-ons on the server | Plugin server and host halves need an embedded JS runtime (goja or deno_core), which means rebuilding bb's plugin host. The half that changes most still stops merging |
| **B3. Port one edge** | A Go host daemon speaking `host-daemon-contract` (typed HTTP routes) | A single binary on remote machines, with no `better-sqlite3` or `node-pty` builds | You track a contract upstream changes without notice. It pays only if host installs are a real pain |
| **B4. No port** *(recommended)* | Aleph stays a thin TypeScript fork. Go lives in Autarch and in sidecars (A2) | Keeps upstream's pace working for mk | Two languages in the stack |

**Recommendation.** B4 now. The ambitious move is A2, not a port. A port
spends mk's scarcest resource, attention on the fork, reproducing what
upstream ships for free. It also works against the Aleph mission: every hour
spent porting is an hour not spent on waste. Keep B3 as a scoped spike only if a
specific pain is named, such as host installs failing or daemon memory on
small machines.

If the real aim is independence from upstream, that is a separate decision:
a hard fork. A port is the most expensive way to make it. A hard fork in
TypeScript gets the same independence at a fraction of the cost.

## Suggested first step if mk picks A2

A spike on Mycroft as a bb dispatcher, with no UI. A plugin `host.ts` starts
the Autarch engine. Mycroft T1 suggestions are then approved to start bb
threads under the pool, tagged with an outcome ID. Done when one real
suggestion runs as a pooled bb thread and its receipt names the outcome.
Panels come after the engine boundary holds.

## Open questions for mk

1. **Porting:** what should a port buy you: performance, one binary, one
   language, or independence from upstream? The answer decides between B3,
   B4 and a hard fork.
2. **Mycroft's job:** should Mycroft become Aleph's dispatcher (A2's core),
   or stay an observer beside it?
3. **Carrying the plugins:** should Autarch's plugins be carried in Aleph's
   `plugins/` like `account-pool` (bundled, released with Aleph), or
   installed separately?
4. **The TUIs:** do they stay first-class, or become thin clients of the
   same engine?
5. **The catch-up branch:** merge `feat/bb-catchup` (sylveste-35bo) now as
   the engine's bb source, or fold it into the A2 spike?
