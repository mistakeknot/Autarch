# Autarch — Vision Document

**Version:** 1.1
**Date:** 2026-02-19
**Status:** Draft
**See also:** [Architecture diagram](../../../docs/architecture.md)

---

## The Core Idea

Autarch is the application layer of the Interverse stack — the interactive surfaces through which Clavain's agency is experienced. Where Clavain (the OS) provides the developer experience via CLI skills, hooks, and slash commands, Autarch provides it via rich terminal UIs.

Autarch sits above the OS, which sits above the kernel:

```
Layer 3: Apps (Autarch)
├── Interactive TUI tools: Bigend, Gurgeh, Coldwine, Pollard
├── Shared component library: pkg/tui (Bubble Tea + lipgloss)
├── Renders OS opinions into interactive experiences
└── Swappable — Autarch is one set of apps, not the only possible set

Layer 2: OS (Clavain)
├── The autonomous software agency — macro-stages, quality gates, model routing
├── Skills, prompts, routing tables, workflow definitions
├── Companion plugins (interflux, interlock, interject, etc.) are OS extensions — each wraps one capability
├── Configures the kernel: phase chains, gate rules, dispatch policies
└── Reacts to kernel events (agent completed → advance phase)

Layer 1: Kernel (Intercore)
├── Runs, phases, gates, dispatches, events — the durable system of record
├── Host-agnostic Go CLI + SQLite
└── Mechanism, not policy — the kernel doesn't know what "brainstorm" means

Interspect (Profiler) — cross-cutting
├── Reads kernel events (phase results, gate evidence, dispatch outcomes)
├── Proposes changes to OS configuration (routing, agent selection, gate rules)
└── Never modifies the kernel — only the OS layer
```

> **Terminology:** For term definitions across all three vision docs, see the [shared glossary](glossary.md).

### Target vs Transitional Reality

> **Banner:** The architecture described in this doc is the target design. Today's reality differs in important ways, acknowledged inline. Where you see "transitional," that marks a gap between target and current state.

**Target state:** Autarch apps are pure rendering surfaces. They read kernel state, submit intents to the OS, and display results. All agency logic (model routing, gate policies, phase orchestration) lives in the OS layer.

**Current reality:**
- Gurgeh's arbiter contains agency logic (LLM conversation sequencing, confidence evaluation, phase advancement). A replacement app would need to reimplement this, violating "apps are swappable." This logic is scheduled for extraction to Clavain.
- Coldwine's task orchestration overlaps with Clavain's sprint skill. Both drive agent dispatch. The resolution is that Coldwine submits intents to the OS — but this intent submission mechanism does not exist yet.
- Bigend discovers projects via filesystem scanning and tmux scraping, not kernel queries. Migration to `ic` is the first planned migration.
- Pollard operates independently of the kernel's discovery subsystem, which doesn't exist yet (v3).

**What "swappable" means today:** Bigend and Pollard are genuinely swappable — they render read-only data. Gurgeh and Coldwine are not fully swappable until their arbiter logic migrates to the OS.

### Apps Are Swappable

Autarch is one realization of the application layer. The kernel and OS are designed so that any set of apps can render them. A different team could build a web dashboard, a VS Code extension, or a mobile client — all reading the same kernel state, all driven by the same OS policies. Autarch is the reference implementation, not the only implementation.

This is the same principle that makes Clavain portable across host platforms: if Claude Code disappears, the OS and kernel survive. Similarly, if Autarch is replaced, the OS and kernel are unaffected. Apps render; the OS decides; the kernel records.

### Relationship to Clavain

In the target architecture, apps don't contain agency logic. Autarch doesn't decide which model to route a review to, or what gates a phase requires, or when to advance a run. Those are OS decisions. Autarch renders those decisions into interactive experiences:

- Bigend renders kernel run state as a monitoring dashboard
- Gurgeh renders kernel phase chains as a PRD generation workflow
- Coldwine renders kernel dispatches as a task coordination interface
- Pollard renders kernel discoveries as a research intelligence tool

When Clavain's policies change (new gate rules, different model routing), Autarch's UIs reflect the change without code modification — they read kernel state, which the OS controls.

**Write-path contract.** Autarch apps are read-only consumers of kernel state. They submit intents to the OS (Clavain) for any action that implies policy — creating runs, advancing phases, overriding gates. They do not call kernel primitives directly for policy-governing operations. See the [Intercore vision doc](intercore-vision.md) for the full write-path contract.

**Intent submission mechanism.** In v1, apps submit intents by calling Clavain's CLI operations (e.g., the TUI calls the same `clavain sprint start` that the CLI user would). The OS validates the intent against current policy (e.g., "is this user allowed to create a run with this complexity?"), performs the kernel mutation, and returns the result. This keeps policy enforcement in the OS layer. Future versions may introduce a structured intent API (JSON-RPC or similar) for richer error reporting and async operations, but the v1 mechanism is direct CLI invocation.

**Minimal Intent Contract (v1).** The v1 contract between apps and OS is deliberately minimal — four operations cover the common cases:

| Intent | App Action | OS Response |
|--------|-----------|-------------|
| `start-run` | User clicks "Start Sprint" in Bigend or Coldwine | OS creates run via `ic`, applies routing policy, returns run ID |
| `advance-run` | User clicks "Advance" or auto-advance triggers | OS evaluates gates via `ic`, advances if passing, returns result |
| `override-gate` | User clicks "Override" on a failed gate | OS records override via `ic` with reason, advances, returns result |
| `submit-artifact` | Tool generates a spec section (Gurgeh) or research result (Pollard) | OS registers artifact via `ic`, returns artifact ID |

All other interactions are reads — apps call `ic` directly for query operations (run status, event tails, dispatch lists). Only policy-governing mutations go through the OS.

### Arbiter Extraction Schedule

Gurgeh's arbiter (the spec sprint orchestration engine) and Coldwine's task orchestrator contain agency logic that belongs in the OS layer. Extraction is staged:

**Phase 1 (v1.5):** Extract Gurgeh's confidence scoring model into a reusable OS-level component. The scoring logic (completeness, consistency, specificity, research axes) maps to kernel gate evidence types. Gurgeh's arbiter calls the OS scoring component instead of embedding the logic.

**Phase 2 (v2):** Extract the spec sprint sequencing from Gurgeh into a Clavain skill. Gurgeh becomes a TUI renderer for the spec sprint — it displays sections, shows confidence scores, and accepts user input. The sprint sequencing (which section to generate next, when to advance, when to loop back) moves to the OS.

**Phase 3 (v2):** Extract Coldwine's task decomposition and agent coordination into Clavain skills that use kernel dispatch primitives. Coldwine becomes a TUI renderer for task progress and agent status.

After Phase 3, both tools satisfy the "apps are swappable" contract — a replacement app only needs to render kernel state and submit intents.

## The Four Tools

**Bigend** — Multi-project mission control. A read-only aggregator that monitors agent activity, displays run progress, and provides a dashboard view across projects. Currently discovers projects via filesystem scanning and monitors agents via tmux session heuristics. Has both a web interface (htmx + Tailwind) and an in-progress TUI.

**Gurgeh** — PRD generation and validation. The most mature tool. Drives an 8-phase spec sprint with per-phase AI generation, confidence scoring (0.0-1.0 across completeness, consistency, specificity, and research axes), cross-section consistency checking, assumption confidence decay, and spec evolution versioning. Specs persist as YAML.

**Coldwine** — Task orchestration. Reads Gurgeh PRDs, generates epics/stories/tasks, manages git worktrees, coordinates agent execution, and integrates with Intermute for messaging. Has a full Bubble Tea TUI (the largest single view at 2200+ lines).

**Pollard** — Research intelligence. Multi-domain hunters (tech, academic, medical, legal, GitHub), continuous watch mode, and insight synthesis. CLI-first with integration into Gurgeh and Coldwine.

## Shared Component Library: `pkg/tui`

Autarch's shared TUI component library is fully portable and immediately reusable:

- `ShellLayout` — split-pane layout with resizable panels
- `ChatPanel` — streaming chat interface with message history
- `Composer` — text input with command completion
- `CommandPicker` — fuzzy-searchable command palette
- `AgentSelector` — agent selection with status indicators
- `View` interface — clean abstraction for pluggable view implementations
- Tokyo Night color scheme — consistent theming across all views

These components depend only on Bubble Tea and lipgloss. They have no Autarch domain coupling.

**Component contracts:** `pkg/tui` follows Bubble Tea conventions — each component implements `tea.Model` (Init, Update, View). Components communicate via Bubble Tea messages, not direct method calls. The `View` interface is the integration boundary: Autarch tools implement `View` to plug into `ShellLayout`. Dependencies flow one way: tools → `pkg/tui` → Bubble Tea. The kernel has no dependency on `pkg/tui` — all TUI concerns live in the app layer.

## Migration to Intercore Backend

Each tool migrates from its own storage backend (YAML files, tool-specific SQLite) to Intercore's kernel as the shared state layer. The migration follows coupling depth — least coupled tools migrate first:

**1. Bigend (read-only — migrate first).** Bigend is a pure observer. Today it discovers projects via filesystem scanning and monitors agents by scraping tmux panes. Migration swaps these data sources:
- Project discovery → `ic run list` across project databases
- Agent monitoring → `ic dispatch list --active`
- Run progress → `ic events tail --all --consumer=bigend`
- Dashboard metrics → kernel aggregates (runs per state, dispatches per status, token totals)

Bigend never writes to the kernel — it only reads. This makes it the lowest-risk migration and the first validation that the kernel provides sufficient observability data.

**2. Pollard (research → discovery pipeline).** Pollard's multi-domain hunters map directly to Intercore's discovery subsystem. Migration connects Pollard's research output to the kernel:
- Hunter results → `ic discovery` events through the kernel event bus
- Insight scoring → kernel confidence scoring with Pollard's domain-specific weights
- Research queries → `ic discovery search` for semantic retrieval
- Watch mode → kernel event consumer that triggers targeted scans

Pollard becomes the scanner component that feeds the discovery → backlog pipeline (see [Clavain vision doc](../../../os/clavain/docs/clavain-vision.md) for the full pipeline workflow). Its hunters become Intercore source adapters.

**3. Gurgeh (PRD generation → run lifecycle).** Gurgeh's 8-phase spec sprint maps to Intercore's run lifecycle with a custom phase chain. Migration creates runs for PRD generation:
- Spec sprint → `ic run create --phases='["vision","problem","users","features","cujs","requirements","scope","acceptance"]'`
- Phase confidence scores → kernel gate evidence (Gurgeh's confidence thresholds become gate rules)
- Spec artifacts → `ic run artifact add` for each generated section
- Spec evolution → run versioning (new run per spec revision, linked via portfolio)

Gurgeh's arbiter (the sprint orchestration engine) remains as tool-specific logic during the migration — it drives the LLM conversation that generates each spec section. The kernel tracks the lifecycle; Gurgeh provides the intelligence.

> **Transitional state.** The arbiter is agency logic (it decides how to sequence LLM calls, when to accept confidence scores, and when to advance). In the target architecture, this intelligence migrates to the OS layer (Clavain), making Gurgeh a pure rendering surface for PRD generation. Until that migration, the "apps are swappable" claim is partially false for Gurgeh and Coldwine — a replacement app would need to reimplement the arbiter and orchestration logic, not just render kernel state. This is an acknowledged architectural debt, not an intentional design choice.

**4. Coldwine (task orchestration — migrate last).** Coldwine has the deepest coupling to Autarch's domain model (`Initiative → Epic → Story → Task`). Its migration is the most complex:
- Task hierarchy → beads (Coldwine's planning hierarchy maps to bead types and dependencies)
- Agent coordination → `ic dispatch` for agent lifecycle
- Git worktree management → remains in Coldwine (kernel doesn't manage git)
- Intermute integration → remains in Coldwine (kernel doesn't manage messaging)

Coldwine's migration overlaps with Clavain's sprint skill — both orchestrate task execution with agent dispatch. The resolution is that Coldwine submits orchestration intents to the OS (Clavain), which performs the actual kernel mutations. Both CLI and TUI surfaces call the same OS operations.

**Migration coexistence.** During migration, tools operate in dual-write mode: they write to both their legacy backend (YAML/tool-specific SQLite) and the kernel. Reads prefer the kernel when available, falling back to legacy. Data migration for existing artifacts (e.g., Gurgeh YAML spec sections → kernel artifact records) runs as a one-time import script per tool. Template ownership transfers from tool-owned (embedded in Go code) to OS-owned (Clavain config) as each tool's arbiter logic migrates to the OS layer.

## Relationship to the Three-Layer Architecture

Autarch is Layer 3 — the application layer atop the OS:

```
Layer 3: Apps (Autarch)
├── Bigend: multi-project mission control (monitoring dashboard)
├── Gurgeh: PRD generation with confidence scoring (spec workflow)
├── Coldwine: task orchestration with agent coordination (execution interface)
├── Pollard: research intelligence with multi-domain hunters (research tool)
└── pkg/tui: shared Bubble Tea components (ShellLayout, ChatPanel, Tokyo Night)

User Interaction (all surfaces share the same kernel state)
├── Clavain (CLI: slash commands, hooks, skills) → calls ic
├── Autarch (TUI: Bigend, Gurgeh, Coldwine, Pollard) → calls ic
└── Direct CLI (ic run, ic dispatch, ic events) → for power users and scripts

A run created via Clavain's /sprint is visible in Bigend's dashboard.
A discovery from Pollard's hunters triggers the same kernel events that
Clavain's hooks consume. The kernel is the single source of truth.
```

## What `pkg/tui` Enables

Beyond the four Autarch tools, the shared component library enables a lightweight **Autarch status tool** — a minimal TUI that provides basic observability without requiring the full tool suite:

- Run list with phase progress bars
- Event stream tail (live-updating)
- Dispatch status dashboard
- Discovery inbox for confidence-tiered review

This minimal TUI is an Autarch tool (not a kernel subcommand) built on `pkg/tui` components that calls `ic` for all its data. It preserves the layering: the kernel is a CLI binary with no TUI dependencies; the app layer provides all interactive surfaces. It's simpler than Bigend but covers the common "what's running right now?" question.

## Signal Architecture

When latency-sensitive consumers (TUI dashboards, live event streams) need sub-second event delivery, the kernel's pull-based `ic events tail` API may be insufficient. Autarch's signal broker addresses this with an app-layer real-time projection:

**Process model.** The signal broker is an embedded goroutine within the Autarch app process — not a standalone daemon or service. It starts when any Autarch tool launches and dies with that process. It is ephemeral by design: all its state is derived from the kernel event log and can be rebuilt from scratch on restart.

**Capabilities:**
- **In-process pub/sub fan-out** with typed subscriptions — each TUI view subscribes to the event types it renders
- **WebSocket streaming** to TUI and web consumers — Bigend's dashboard and the Autarch status tool connect via WebSocket rather than polling
- **Backpressure handling** — evict-oldest-on-full for view consumers (TUI views that can tolerate dropped frames), blocking for audit consumers (session-local audit trails within the app process)

The kernel's durable event log remains the source of truth. The broker is a real-time projection of it — a convenience for latency-sensitive rendering, not a replacement for the event bus. Durable event consumption (Interspect, Clavain reactor) always goes through the kernel's cursor-based API, never through the broker. If the broker crashes, TUI consumers reconnect and the broker rebuilds from the kernel cursor position.

> **Status:** This architecture exists in Autarch's current codebase (the signal broker pattern is proven in Bigend and Coldwine). It has not yet been connected to Intercore's event bus — that integration is part of the Bigend migration (see Migration to Intercore Backend above).

> **Scope clarification:** The signal broker is a **rendering optimization**, not an architectural component. It exists to give TUI consumers sub-second event delivery that the kernel's pull-based API can't provide. It is not a replacement for the kernel's event bus, does not add durability guarantees, and does not participate in the write-path contract. Durable event consumption always goes through the kernel's cursor-based API. If the signal broker is removed entirely, the system works identically — TUI updates are slightly slower (polling-based instead of push-based). No design decisions should depend on the broker's existence.

### Autarch Status Tool — The Primary Wedge

The minimal Autarch status tool is the first app that should ship, because it validates the full stack without requiring the complexity of the four specialized tools:

```
┌─ Autarch Status ─────────────────────────────────┐
│                                                    │
│ Runs           Phase        Dispatches   Events    │
│ ──────────     ─────────    ──────────   ──────    │
│ R42 auth       executing    3 active     47 total  │
│ R41 refactor   shipping     0 active     23 total  │
│                                                    │
│ Active Dispatches                                  │
│ ──────────────────────────────────────────────     │
│ D12 reviewer-arch   running  2m14s  Opus           │
│ D13 reviewer-qual   running  1m48s  Haiku          │
│ D14 reviewer-safe   completed 3m02s Sonnet         │
│                                                    │
│ Event Stream (last 5)                              │
│ ──────────────────────────────────────────────     │
│ 14:23:01  dispatch.completed  D14  reviewer-safe   │
│ 14:22:58  gate.passed         R42  plan-review     │
│ 14:22:45  phase.advanced      R42  executing       │
│                                                    │
└────────────────────────────────────────────────────┘
```

**Why ship this first:**
1. It validates the kernel's query APIs are sufficient for real-time display
2. It validates `pkg/tui` components work with kernel data
3. It gives immediate value — "what's running right now?" is the most common question
4. It has zero agency logic — pure rendering of kernel state
5. It becomes the foundation for Bigend's migration

## What Autarch Is Not

- **Not the OS.** Autarch renders OS decisions; it doesn't make them. Routing, gate policies, and model selection are Clavain's domain.
- **Not the kernel.** Autarch reads and writes kernel state through `ic`; it doesn't own the system of record.
- **Not required.** Everything Autarch does can be done via Clavain CLI or direct `ic` commands. Autarch is a convenience layer that makes the system more accessible, not a dependency.
- **Not a single monolith.** Each tool is independently useful. You can run Bigend for monitoring without Coldwine for orchestration.

---

*This document was extracted from the Intercore vision doc on 2026-02-19 to establish the application layer as a distinct architectural concern, separate from both the kernel (Intercore) and the OS (Clavain).*
