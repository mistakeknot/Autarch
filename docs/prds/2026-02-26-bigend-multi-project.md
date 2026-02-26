# PRD: Bigend Multi-Project Support (iv-o0955)

## Problem

Bigend in the unified TUI is single-project: `projectID`/`projectName` are scalar fields set once during onboarding. In a monorepo like Demarch with 40+ subprojects, all sessions, runs, and dispatches are mixed together with no project grouping or switching capability. The vision describes Bigend as "multi-project agent mission control" — the implementation contradicts this.

## Solution

Add a project sidebar to BigendView using the existing `discovery.Scanner` to find projects on disk. Users select a project to scope all data (sessions, runs, dispatches) to that project. An "All" mode aggregates data across projects (today's behavior).

## Features

### F1: Project Discovery Integration
Wire `discovery.Scanner` into BigendView. Scanner runs on `Focus()` (tab switch) in a background goroutine. Cache results — only re-scan when user presses refresh.

### F2: Project Sidebar
Replace the flat sessions/runs/dispatches sidebar with a two-level hierarchy:
- Top section: project list (from scanner)
- Project selection reloads data in the document pane

Sidebar items: project name + active agent count badge. Selected project highlighted. "All Projects" sentinel at top for aggregated view.

### F3: Project-Scoped Data Loading
When a project is selected:
- Set `client.WithProject(project.Name)` and reload sessions/specs
- Filter `runs` by `Run.ProjectDir` matching `project.Path`
- Filter `dispatches` by their parent run's project

When "All Projects" is selected (default), show all data (today's behavior).

### F4: Project Health Indicators
Each project sidebar entry shows: name + icon for tooling presence (IC = Intercore, CW = Coldwine, PR = Pollard).

## Non-Goals

- Multi-project for Coldwine/Pollard (they operate on CWD naturally)
- Project creation UI (use onboarding / `coldwine init`)
- Remote project discovery (local filesystem only)
- Cross-project dispatching (single-project dispatches only)

## Technical Approach

### Data Flow
```
Focus() → Scanner.Scan() → projectsLoadedMsg → sidebar update
                                                ↓
                                   SidebarSelectMsg → setActiveProject()
                                                ↓
                                   client.WithProject() + loadSessions() + loadRuns() + loadDispatches()
```

### Key Types
```go
// In BigendView
scanner  *discovery.Scanner  // injected via SetScanner()
projects []discovery.Project // cached scan results
activeProject int            // index into projects (-1 = all)

// Messages
type projectsLoadedMsg struct { projects []discovery.Project }
```

### Scanner Configuration
Default scan roots: `$HOME/projects`, CWD. Configurable via `.autarch/config.toml` or `bigend.yaml`.

## Success Criteria

1. Bigend sidebar shows discovered projects with tooling indicators
2. Selecting a project scopes sessions/runs/dispatches to that project
3. "All Projects" shows aggregated view (backward-compatible with today)
4. Scanner runs in background — no startup latency
5. Works offline (scanner is filesystem-based, Intermute fallback already exists)
