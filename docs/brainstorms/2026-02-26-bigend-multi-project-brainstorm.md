# Brainstorm: Bigend Multi-Project Support in Unified TUI (iv-o0955)

## Problem Statement

Bigend in the unified TUI (`internal/tui/views/bigend.go`) is hardcoded to a single project context. The `projectID`/`projectName` fields are string scalars set once during onboarding and never changed. In a monorepo like Demarch (40+ subprojects), this means:

1. Sessions from all projects are mixed together with no grouping
2. No way to switch project context without restarting
3. Runs/dispatches from Intercore are global (not project-scoped)
4. The vision doc describes Bigend as "multi-project agent mission control" — the TUI contradicts this

## Current Architecture

### What Already Works (Multi-Project)
- **`discovery.Scanner`** (`internal/bigend/discovery/scanner.go`): Filesystem scanner that discovers projects by detecting `.gurgeh`, `.coldwine`, `.pollard`, `.clavain` directories. Returns `[]Project` with rich metadata (task stats, PRD stats, Intercore presence).
- **`autarch.Client.WithProject()`**: Sets `c.project` which auto-appends `?project=...` to all HTTP requests. The Intermute API already supports project-scoped queries.
- **`intercore.Client.RunList()`**: Returns runs with `ProjectDir` field — can be grouped by project.
- **Bigend web/daemon** (`internal/bigend/`): Already multi-project via aggregator + scanner.
- **Data models**: `Spec.Project`, `Epic.Project`, `Task.Project`, `Session.Project` fields exist.

### What's Single-Project
- **`BigendView` struct**: `projectID string`, `projectName string` — scalars, not arrays
- **List calls**: `ListSessions("")`, `loadDispatches()` — no project filter
- **No project sidebar**: The view has Sessions/Tasks panes but no project dimension
- **`SetProjectContext()`**: Called once, never updated

## Design Options

### Option A: Project Sidebar (Full Discovery)

Add a project sidebar to Bigend that uses `discovery.Scanner` to find all projects on disk. User selects a project, and the dashboard reloads with project-scoped data.

**Layout:**
```
┌─ Projects ──┬─ Sessions (3) ──────────┬─ Chat ──────────────┐
│ > Autarch   │ ● agent-1   coldwine    │                     │
│   Intermute │ ○ agent-2   gurgeh      │                     │
│   Intercore │                         │                     │
│   Interflux │ Runs (2)               │                     │
│             │ ● abc123 sprint [exec]  │                     │
│             │ ✓ def456 review [done]  │                     │
│             │                         │                     │
│             │ Dispatches (1)          │                     │
│             │ ● a1b2c3 fd-safety 3m   │                     │
└─────────────┴─────────────────────────┴─────────────────────┘
```

**Pros:** True multi-project navigation. Matches vision. Uses existing discovery code.
**Cons:** Larger scope. ShellLayout needs 4 panes (or project list replaces sidebar). Scanner adds startup latency.

### Option B: Project Filter in Existing Sidebar

Add a project dropdown/selector to the existing sessions pane. Projects are inferred from the data (unique `Project` values from sessions, specs, runs). No filesystem scan needed.

**Layout:**
```
┌─ Sessions ─────────────────────┬─ Chat ──────────────────────┐
│ Project: [Autarch ▾]          │                              │
│                               │                              │
│ Sessions (3)                  │                              │
│ ● agent-1   coldwine          │                              │
│ ○ agent-2   gurgeh            │                              │
│                               │                              │
│ Runs (2)                      │                              │
│ ● abc123 sprint [executing]   │                              │
│                               │                              │
│ Dispatches (1)                │                              │
│ ● a1b2c3 fd-safety 3m        │                              │
└───────────────────────────────┴──────────────────────────────┘
```

**Pros:** Minimal UI change. No new dependencies. Fast.
**Cons:** Only shows projects that have data in Intermute/Intercore. Won't discover projects without sessions.

### Option C: Aggregated Dashboard + Drill-Down

Show all projects aggregated on the main dashboard (like a portfolio view). Click into a project to see its detail. No sidebar — project cards in the document pane.

**Layout (aggregated):**
```
┌─ Sidebar ──┬─ Portfolio ──────────────┬─ Chat ──────────────┐
│ Overview   │ Autarch         3 runs   │                     │
│ Autarch    │ ████████░░ 80%  2 agents │                     │
│ Intermute  │                          │                     │
│ Intercore  │ Intermute       1 run    │                     │
│            │ ████░░░░░░ 40%  1 agent  │                     │
│            │                          │                     │
│            │ Intercore       0 runs   │                     │
│            │ ░░░░░░░░░░  0%  0 agents │                     │
└────────────┴──────────────────────────┴─────────────────────┘
```

**Pros:** Best overview. Natural for monorepo. Shows all projects at once.
**Cons:** Most complex. Needs two render modes (portfolio + detail). Progress bars need real data.

## Recommendation

**Option A (Project Sidebar)** is the right choice:

1. **Matches the vision**: "Multi-project agent mission control" needs per-project navigation
2. **Reuses existing code**: `discovery.Scanner` is battle-tested from web/daemon Bigend
3. **Consistent pattern**: ColdwineView already has a sidebar with mode toggle; Bigend can follow the same pattern
4. **Extensible**: Once the sidebar exists, it's trivial to add project health indicators, active agent counts, etc.
5. **Scanner latency**: Mitigated by scanning on Focus() (background) rather than Init()

**Key constraint**: The `ShellLayout` (3-pane: sidebar/doc/chat) already exists. The project list becomes the sidebar content, and the current sessions/runs/dispatches become the document content. No layout changes needed.

## Migration Path

1. Add `discovery.Scanner` to BigendView (injected via constructor or setter)
2. Replace the sessions-only sidebar with project list + session/run/dispatch subsections
3. When a project is selected, set `client.WithProject()` and reload all data
4. Keep "All Projects" as the default selection (shows aggregated data like today)
5. Runs/dispatches: group by `ProjectDir`/`RunID` when in "All" mode

## Open Questions

1. Should Coldwine and Pollard also get project switching? Or only Bigend?
   - Recommendation: Bigend only for now. Coldwine/Pollard operate on the CWD project naturally.
2. How to handle Intercore data? `ic.RunList()` returns all runs. Filter by `ProjectDir`?
   - Yes — `Run.ProjectDir` can be matched against `discovery.Project.Path`.
3. Scan roots configuration? Where does the user define which directories to scan?
   - Use existing `bigend.yaml` config or `.autarch/config.toml`. Fallback to `$HOME/projects`.
4. What happens when Intermute is offline? Scanner still works (filesystem), but sessions/specs won't load.
   - Show discovered projects from scanner with "no data" placeholder. Existing offline badge handles the rest.
