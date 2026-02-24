# Flux Drive Enhancement Summary — Autarch Repo Review

Reviewed by 7 agents (5 codebase-aware, 2 generic) on 2026-02-06.

## Key Findings

1. **Bigend binds to `0.0.0.0` — only server bypassing netguard** (fd-security, P0). All other servers use `netguard.EnsureLocalOnly()` but Bigend's web + daemon servers expose session spawn, project enumeration, and terminal streaming to the entire network. One-line fix.

2. **Dual TUI implementations are the #1 structural issue** (4/7 agents: fd-architecture, fd-user-experience, fd-code-quality, code-simplicity). `App` and `UnifiedApp` duplicate ~400 LOC, diverge on Ctrl+C behavior, and create inconsistent UX. Already planned for Phase 2 of unified navigation.

3. **`SprintState.Clone()` doesn't deep-copy `ExplorationResult` map** (fd-code-quality, P0). The Clone method deep-copies 6 fields but misses this mutable map, violating the documented thread-safety contract.

4. **NextID TOCTOU race allows duplicate spec IDs** (data-integrity-guardian, P0). Two concurrent spec creations can generate the same PRD ID. Fix exists in codebase (`fileutil.LockFile`) but isn't applied here.

5. **30+ file write sites use `os.WriteFile` instead of existing `AtomicWriteFile`** (data-integrity-guardian, P0). Crash during write produces truncated/empty files. The fix already exists — just need to adopt it consistently.

## Issues to Address

### P0 — Must Fix

- [ ] **Bigend 0.0.0.0 binding** — add `netguard.EnsureLocalOnly()` to web + daemon servers (fd-security, 1/7)
- [ ] **SprintState.Clone() shallow copy of ExplorationResult** — add deep-copy for `map[string]any` (fd-code-quality, 1/7)
- [ ] **NextID TOCTOU race** — apply `fileutil.LockFile` to spec ID generation (data-integrity-guardian, 1/7)
- [ ] **Non-atomic file writes** — replace ~30 `os.WriteFile` calls with `fileutil.AtomicWriteFile` (data-integrity-guardian, 1/7)
- [ ] **internal/tui is a god package** — 18 cross-tool imports in 12 files (fd-architecture, 1/7)
- [ ] **Dual TUI implementations** — merge App into UnifiedApp, ~400 LOC duplication (4/7 agents)
- [ ] **Ctrl+Right collision** — tab cycling steals keybinding from SprintView accept-draft (fd-user-experience, 1/7)
- [ ] **Ctrl+S triggers XOFF** — scan keybinding conflicts with terminal flow control (fd-user-experience, 1/7)
- [ ] **OnboardingOrchestrator dead code** — 137 LOC, zero callers (code-simplicity, 1/7)
- [ ] **Subscription.Stream() deadlock** — unconditional send on full channel ignores context (2/7: fd-code-quality, fd-performance)

### P1 — Should Fix

- [ ] **MCP path traversal** — `handleGetPRD`/`handleUpdateTask` accept unsanitized IDs with `../` (fd-security)
- [ ] **USDA API key in URL query string** — visible in logs/proxies, other hunters use headers (fd-security)
- [ ] **Tab switch discards active sprint** — no confirmation or autosave (fd-user-experience)
- [ ] **Pollard imports Gurgeh arbiter types** — inverted dependency direction (fd-architecture)
- [ ] **pkg/contract phantom layer** — 7 types defined, 1 importer (2/7: fd-architecture, code-simplicity)
- [ ] **pkg/compound 821 LOC zero importers** — dead code, safe to delete (code-simplicity)
- [ ] **Triple entity type system** — contract, intermute, events all define Status/Task/Epic with ~400 LOC converters (code-simplicity)
- [ ] **Pollard hunters zero tests** — 13 implementations, 0 test files (fd-code-quality)
- [ ] **Signal store bypasses pkg/db.Open** — separate connection pool, no MaxOpenConns(1) (data-integrity-guardian)
- [ ] **No schema migration versioning** — only `CREATE IF NOT EXISTS`, no ALTER TABLE safety (data-integrity-guardian)
- [ ] **Foreign keys not enabled** — only Coldwine enables `PRAGMA foreign_keys` (data-integrity-guardian)
- [ ] **Archive engine partial move** — no rollback on failure (data-integrity-guardian)
- [ ] **lipgloss.NewStyle() per frame** — 219 occurrences in render path, hoist to package vars (fd-performance)
- [ ] **Broker.Publish holds mutex during fanout** — snapshot subscribers under lock, fanout without (fd-performance)
- [ ] **Mixed logging** — fmt.Fprintf(Stderr) + slog + log.Print, should standardize on slog (fd-code-quality)
- [ ] **Silent swallow of unknown slash commands** — no user feedback (fd-user-experience)
- [ ] **Ctrl+U collision** — doc panel scroll vs revert last run (fd-user-experience)
- [ ] **MinShellWidth 100 too aggressive** — fails on typical terminal splits (fd-user-experience)

### P2 — Nice to Fix

- [ ] Duplicate `generateID()` across 4 signal emitters (fd-architecture)
- [ ] `internal/intermute` deprecated but still wired (fd-architecture)
- [ ] `spec` vs `specs` naming ambiguity (fd-architecture)
- [ ] Shell separator rebuilt every frame (fd-performance)
- [ ] Insights API reads all YAML per request (fd-performance)
- [ ] GetStats uses 5 sequential queries (fd-performance)
- [ ] 93 files with legacy tool names in internal types (fd-code-quality)
- [ ] `advanceInternal` 120+ lines, 4-level nesting (fd-code-quality)
- [ ] Untested shared packages: netguard, httpapi (fd-code-quality)
- [ ] No 16-color terminal fallback (fd-user-experience)
- [ ] Footer overcrowded, truncates on narrow terminals (fd-user-experience)
- [ ] Help overlay duplicate entries (fd-user-experience)
- [ ] `g/G` in LogPane clashes with printable-key policy (fd-user-experience)
- [ ] `j/k` in Sidebar conflicts with chat-focused printable-key policy (fd-user-experience)
- [ ] HTTP servers decode JSON without size limits (fd-security)
- [ ] Tmux session name unsanitized in exec.Command (fd-security)
- [ ] Synthesizer AgentCmd parsed with strings.Fields (fd-security)
- [ ] cmd/testui and cmd/archviz as top-level binaries (code-simplicity)
- [ ] pkg/timeout is 3 constants (code-simplicity)
- [ ] Windows file locking is a no-op (data-integrity-guardian)
- [ ] Silent time.Parse failures across stores (data-integrity-guardian)
- [ ] No explicit WAL checkpoint strategy (data-integrity-guardian)
- [ ] OpenShared connection pool never closed (data-integrity-guardian)

## Convergence Map

| Finding | Agents | Confidence |
|---------|--------|------------|
| Dual TUI implementations (App vs UnifiedApp) | fd-architecture, fd-user-experience, fd-code-quality, code-simplicity | 4/7 HIGH |
| Subscription.Stream() deadlock/blocking | fd-code-quality, fd-performance | 2/7 |
| pkg/contract phantom/unused layer | fd-architecture, code-simplicity | 2/7 |
| Legacy tool name references | fd-code-quality, code-simplicity | 2/7 |

## Agent Reports

| Agent | Tier | Verdict | P0 | P1 | P2 | P3 | Report |
|-------|------|---------|----|----|----|----|--------|
| fd-architecture | T1 | needs-changes | 1 | 3 | 4 | 0 | [fd-architecture.md](fd-architecture.md) |
| fd-user-experience | T1 | needs-changes | 2 | 6 | 6 | 0 | [fd-user-experience.md](fd-user-experience.md) |
| fd-code-quality | T1 | needs-changes | 2 | 3 | 3 | 0 | [fd-code-quality.md](fd-code-quality.md) |
| fd-performance | T1 | needs-changes | 0 | 2 | 4 | 3 | [fd-performance.md](fd-performance.md) |
| fd-security | T1 | needs-changes | 1 | 2 | 3 | 2 | [fd-security.md](fd-security.md) |
| data-integrity-guardian | T3 | needs-changes | 2 | 5 | 6 | 0 | [data-integrity-guardian.md](data-integrity-guardian.md) |
| code-simplicity-reviewer | T3 | needs-changes | 2 | 5 | 5 | 0 | [code-simplicity-reviewer.md](code-simplicity-reviewer.md) |

**Totals: 10 P0, 26 P1, 31 P2, 5 P3** (before deduplication — some overlap, especially on dual TUI and entity types)
