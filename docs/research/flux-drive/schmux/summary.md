# Schmux Review: Copy / Adapt / Inspire for Autarch

> **Source:** https://github.com/sergeknystautas/schmux (v1.1.1, Apache 2.0)
> **Review date:** 2026-02-11
> **Agents:** fd-architecture (complete), fd-user-product (complete), agent-native-reviewer (complete)
> **Reviewer:** Manual deep-read of all key source + 3/3 agent reviews completed
>
> **Agent outputs:**
> - [Architecture review](summary.md) (this file — synthesized from fd-architecture)
> - [UX & Product review](ux-product-review.md) (fd-user-product — 1027 lines)
> - [Agent-Native review](agent-native-review.md) (agent-native-reviewer — scoring + gap analysis)

## What schmux is

Multi-agent orchestration tool built on tmux. Go backend daemon + React/TypeScript web dashboard. Manages parallel AI coding sessions (Claude, Codex, Gemini) with workspace isolation, real-time terminal streaming, and LLM-based agent state classification.

Key differentiator from Autarch: schmux is *session-centric* (spawn agents into tmux windows, watch them work via dashboard), while Autarch is *artifact-centric* (specs, tasks, research, signals).

---

## COPY (adopt directly)

### 1. Dual-format instruction injection

**What:** When spawning an agent session, schmux provisions signaling instructions two ways:
- **File-based**: Writes instructions to `.claude/CLAUDE.md`, `.codex/AGENTS.md`, `.gemini/GEMINI.md` with idempotent `<!-- SCHMUX:BEGIN -->` / `<!-- SCHMUX:END -->` markers
- **CLI-based**: Injects via `--append-system-prompt` flag for tools that support it

**Source:** `internal/provision/provision.go` (244 lines)

**Why copy:** Autarch already has per-tool dispatch via `pkg/agenttargets/`. Adding marker-based idempotent file injection would let Coldwine tasks and Gurgeh specs provision context-specific instructions into agent work directories. The `<!-- BEGIN/END -->` marker pattern prevents duplicate injection on re-runs.

**Specific application:**
- Coldwine task briefs → inject task constraints into `.claude/CLAUDE.md` before dispatching agent
- Gurgeh spec context → inject acceptance criteria as system prompt for validation agents
- Use `provision.SupportsSystemPromptFlag(tool)` pattern to choose file vs CLI injection

### 2. Bracket-based signaling protocol

**What:** Agents emit `--<[schmux:state:message]>--` on their own line. States: `needs_input`, `needs_testing`, `completed`, `error`, `working`. Regex parser strips ANSI escape sequences before matching.

**Source:** `internal/signal/signal.go` (145 lines — remarkably clean)

**Why copy:** Autarch's `pkg/signals/` already has signal types but no wire format for agent-to-orchestrator communication. The bracket format is:
- Grep-friendly (easy to spot in logs)
- Won't match in code blocks (requires own-line + whitespace-only prefix)
- Handles terminal ANSI cruft (cursor movements replaced with spaces before regex match)
- Tiny implementation (145 lines with tests)

**Adaptation needed:** Replace `schmux` namespace with `autarch` in the bracket markers. Map schmux states to Autarch's existing signal vocabulary.

### 3. Config/state separation

**What:** User preferences (`~/.schmux/config.json`) kept separate from runtime state (`~/.schmux/state.json`). Config is version-controllable; state is ephemeral and machine-specific.

**Source:** `internal/config/config.go`, `internal/state/state.go`

**Why copy:** Autarch already does this partially (`.gurgeh/specs/` vs runtime). Formalizing the pattern for `.autarch/config.json` (repos, tool preferences, quick-launch presets) vs `.autarch/state.db` (active sessions, last-seen timestamps, workspace assignments) would be valuable as we add multi-agent dispatch.

---

## ADAPT (take the pattern, change the implementation)

### 4. Multi-method agent detection with fallback chain

**What:** Runtime detection of Claude/Codex/Gemini via ordered fallback: PATH lookup → native install path → Homebrew cask → npm global. Parallel execution with 3s timeout. `ToolDetector` interface.

**Source:** `internal/detect/agents.go` (469 lines)

**Adapt because:** Autarch's `pkg/agenttargets/` already has agent dispatch infrastructure, but detection is ad-hoc. The interface pattern is great:
```go
type ToolDetector interface {
    Detect(ctx context.Context) (Tool, bool)
    Name() string
}
```

**Changes for Autarch:**
- Cache detection results in state (schmux stores in config — dual-source-of-truth bug)
- Add TTL-based cache invalidation (don't re-detect every startup)
- Lazy detection per tool (detect when first needed, not all at startup)
- Keep Autarch's existing backend abstraction (`BackendClaude`, `BackendCodex`) and add detection logic there

### 5. Overlay file provisioning for workspaces

**What:** Copy files from `.schmux/overlay/{repo}/` into workspace directories on session creation. Used for per-repo `.env` files, custom CLAUDE.md instructions, tool configs.

**Source:** `internal/workspace/overlay.go` (part of workspace package)

**Adapt because:** Autarch doesn't use git worktrees for workspace isolation (schmux's approach), but the overlay concept maps to:
- Gurgeh spec onboarding: copy template files into new spec directories
- Coldwine task setup: provision tool-specific configs before dispatching agents
- Pollard hunt initialization: seed `.pollard/sources/` with per-domain templates

**Changes for Autarch:**
- No git worktree complexity — just file copying with template variable substitution
- Store overlays in `.autarch/overlays/` (global) or per-tool directories
- Add to existing Intermute-based provisioning flow rather than a new workspace manager

### 6. Debounced activity tracking

**What:** Session tracker debounces activity updates (500ms) to avoid state churn. Only persists "last activity" timestamp when terminal output settles.

**Source:** `internal/session/tracker.go:241-253`

**Adapt because:** Autarch's signal system + Intermute could benefit from debounced state updates for:
- Coldwine task progress (avoid DB churn from rapid agent output)
- Pollard insight extraction (debounce file watcher events)
- Bigend dashboard refresh (batch UI updates)

**Changes for Autarch:**
- Use `time.AfterFunc` debounce pattern (simpler than schmux's channel-based approach)
- Apply to Intermute event bus, not PTY tracking (Autarch doesn't attach to terminal sessions)

### 7. NudgeNik LLM-based state classification (concept only)

**What:** When agents don't signal their state directly, schmux captures the last 100 lines of terminal output and sends to an LLM with a structured JSON schema. The LLM classifies the agent's state: blocked, waiting for input, working, or completed.

**Source:** `internal/nudgenik/nudgenik.go` (187 lines)

**Adapt because:** The *concept* is directly applicable to Autarch's Bigend Agent Intelligence (deferred feature in MEMORY.md). When dispatched agents don't use bracket signals, Autarch could:
- Read the agent's recent output from Intermute stream
- Send to LLM for state classification
- Surface as suggestions in Bigend dashboard

**Changes for Autarch:**
- Don't hardcode the prompt (schmux does — bad for customization)
- Use Autarch's existing LLM dispatch infrastructure (not a separate Claude call)
- Add confidence thresholding (schmux accepts any parseable JSON response)
- Make it opt-in per agent type (not every agent needs LLM classification)

---

## INSPIRE (learn from, but don't directly implement)

### 8. Daemon lifecycle pattern (PID file + health check)

**What:** Daemon uses PID file (`~/.schmux/schmux.pid`) with `syscall.Signal(0)` health checks. Clean lifecycle: `ValidateReadyToRun()` → `Start()` (fork) → `Run()` (long-lived) → `Shutdown()` (context cancellation).

**Source:** `internal/daemon/daemon.go` (707 lines)

**Inspiration:** Autarch's Intermute server could benefit from formalized lifecycle management. Currently uses `exec.Command` + context. The PID file pattern is useful if Intermute ever runs as a standalone daemon (currently embedded).

### 9. Session tracking via PTY attachment

**What:** Attach to tmux sessions via PTY, stream terminal output through WebSocket to web dashboard. Handles UTF-8 boundary detection, ANSI stripping, reconnection.

**Source:** `internal/session/tracker.go` (507 lines)

**Inspiration:** Autarch's TUI is local (Bubble Tea), not remote (WebSocket). But if Bigend ever adds a web dashboard mode, the PTY → WebSocket forwarding pattern is well-implemented here. The UTF-8 boundary handling (buffering incomplete multi-byte sequences) is a useful reference for any terminal I/O.

### 10. Quick Launch presets

**What:** User-configured one-click actions: shell commands or AI agent dispatch with pre-set prompts. Global presets merge with per-workspace presets.

**Source:** `docs/targets.md`, `internal/config/config.go`

**Inspiration:** Maps to Autarch's slash command system. The interesting pattern is the *two-tier merge* (global + workspace-local presets with workspace taking precedence on name conflicts). Could inform how Autarch handles per-project vs global slash command overrides.

### 11. Workspace reuse logic

**What:** Before creating a new git worktree, check if an idle workspace matches (same repo + branch, or same repo with up-to-date default branch). Reuse idle workspaces to avoid disk bloat.

**Source:** `internal/workspace/manager.go:166-214`

**Inspiration:** Coldwine tasks could reuse idle task directories (match task type, clean state before reuse). Not directly applicable since Autarch doesn't use git worktrees, but the matching heuristic (exact match → relaxed match → create new) is a useful pattern for resource pooling.

---

## AVOID (anti-patterns to not replicate)

### God modules
`workspace/` is 23K LOC across 20 files with 4 mutexes and 11 struct fields on the manager. Includes git operations, overlay provisioning, conflict resolution, Linear sync, PR integration, and git graph analysis. **Autarch lesson:** Enforce package size limits. If a package needs 20 files, split into sub-packages.

### Tight manager coupling
Daemon directly instantiates 5+ managers (`workspace.New()`, `session.New()`, `github.NewDiscovery()`, etc.) with no interface boundaries. Every manager holds refs to both config AND state. **Autarch lesson:** Keep using Intermute for cross-tool coordination. Use interface boundaries between components.

### Dual-source-of-truth
`RunTargets` stored in user-facing config but populated by runtime detection. Detection results should live in ephemeral state, not version-controlled config. **Autarch lesson:** Agent detection results belong in state/cache, not config.

### Premature generalization
`workspace/linear_sync.go` (30K LOC), `workspace/git_graph.go` (13K LOC), remote workspace support (5K LOC), external diff commands. These are niche features that add massive complexity. **Autarch lesson:** Keep Autarch's YAGNI discipline. Remote/multi-host is already explicitly deferred.

---

## Priority Implementation Order

If implementing from this review, suggested order:

1. **Bracket signaling protocol** (signal.go — 145 lines, clean copy) — enables all subsequent agent communication
2. **Dual-format instruction injection** (provision.go — 244 lines, adapt) — needed for Coldwine agent dispatch
3. **Config/state separation** (formalize existing pattern) — foundation for multi-agent state management
4. **Agent detection with caching** (adapt detect/agents.go pattern) — improve `pkg/agenttargets/` reliability
5. **Debounced activity tracking** (pattern only) — optimize Intermute event bus
6. **NudgeNik-style classification** (future — Bigend Agent Intelligence) — requires bracket signaling first

---

## Architectural Comparison

| Dimension | schmux | Autarch | Winner |
|-----------|--------|---------|--------|
| Module boundaries | Monolithic managers (23K LOC workspace) | Independent `internal/{tool}/` packages | Autarch |
| Cross-component coordination | Direct struct refs | Intermute HTTP/WS bus | Autarch |
| Agent signaling | Clean bracket protocol | No wire format yet | schmux |
| Tool detection | Multi-method fallback | Ad-hoc | schmux |
| Instruction injection | Dual-format (file + CLI) | Manual | schmux |
| State management | Config/state split (with bugs) | SQLite + YAML | Tie |
| Complexity | High (god modules, remote, linear sync) | Moderate (focused tools) | Autarch |
| User interface | Web dashboard (React) | TUI (Bubble Tea) | Different goals |

**Bottom line:** Autarch's architecture is healthier. Adopt schmux's *communication patterns* (signaling, injection, detection) without adopting its *structural patterns* (monolithic managers, tight coupling).

---

## Cross-Agent Convergence (3/3 agents flagged)

These findings appeared independently across all three reviewer agents:

### 1. Agent signaling protocol is the #1 priority (3/3)
All three agents identified bracket-based signaling (`--<[schmux:state:message]>--`) as the single most valuable pattern to adopt. Architecture called it "clean copy", UX called it "CRITICAL", agent-native scored it highest on action parity.

### 2. NudgeNik concept is high-value but needs adaptation (3/3)
Architecture recommended adopting the concept with a template system instead of hardcoded prompts. UX emphasized it as the core "attention allocation" innovation. Agent-native recommended adding confidence thresholding and bidirectional feedback.

### 3. Environment variable injection for agent context (2/3)
Both UX and agent-native flagged `SCHMUX_ENABLED=1` / `SCHMUX_SESSION_ID` / `SCHMUX_WORKSPACE_ID` as a pattern Autarch should copy. Directly maps to `AUTARCH_ENABLED=1`, `AUTARCH_TOOL=gurgeh|coldwine|pollard`, `AUTARCH_SPEC_ID`, `AUTARCH_TASK_ID`.

### 4. Agent-native gap: context starvation (agent-native only)
Agent-native reviewer identified a critical gap: schmux provisions *static* instruction templates but never injects *runtime* context (available specs, tasks, capabilities). Autarch should go further — inject available specs/tasks into agent system prompts dynamically. Score: schmux 3/10 on "Dynamic Context" vs Autarch target 9/10.

### 5. UX insight: prompt-first design (UX only)
UX reviewer identified a design philosophy worth adopting: describe the task *before* choosing tools. schmux puts a large textarea at top of spawn wizard, then tool selection below. This reduces "tool-fixation bias" — thinking "I need Claude" instead of "I need code review."

### 6. Agent-native gap: bidirectional signaling (agent-native only)
schmux signals are one-way (agent → orchestrator). Agent-native reviewer scored this 3/10 and recommended Autarch add orchestrator → agent messaging via Intermute (e.g., "your spec was merged", "task reassigned"). This maps to the deferred Bigend Agent Intelligence feature.

---

## Final Synthesis

**What to build (in order):**

| # | What | Source | LOC estimate | Autarch location |
|---|------|--------|-------------|-----------------|
| 1 | Bracket signal parser + format spec | signal.go (145 lines) | ~200 | `pkg/signals/parser.go` |
| 2 | Env var injection at agent spawn | detect/agents.go pattern | ~50 | `pkg/agenttargets/backend_*.go` |
| 3 | Dual-format instruction injection with markers | provision.go (244 lines) | ~300 | `pkg/agenttargets/provision.go` |
| 4 | Agent detection with caching + fallback | detect/agents.go interface | ~400 | `pkg/agenttargets/detect.go` |
| 5 | NudgeNik-style LLM classifier (later) | nudgenik.go concept | ~300 | `pkg/nudge/classifier.go` |
| 6 | Quick launch presets in config (later) | config.go pattern | ~200 | `.autarch/config.json` schema |

**What NOT to build:** tmux dependency, git worktrees, web dashboard, workspace overlays, git sync workflows, Linear integration.

**Philosophy to adopt:** "Agents need two communication channels: output channel (what they did) + state channel (what they need)." Most orchestration tools only have #1. Add #2.
