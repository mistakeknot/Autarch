# Plan: Bigend Multi-Project Support (iv-o0955)

## Overview

Add project discovery and project-scoped navigation to BigendView in the unified TUI. 4 tasks, estimated ~400 lines of new/modified code.

## Task 1: Wire Scanner + Project Loading

**Files:** `internal/tui/views/bigend.go`, `cmd/autarch/main.go`

### Changes

1. Add fields to `BigendView`:
   ```go
   scanner       *discovery.Scanner
   projects      []discovery.Project
   activeProject int // -1 = all, 0..N = specific project
   ```

2. Add `SetScanner(s *discovery.Scanner)` setter.

3. Add `projectsLoadedMsg` type and `loadProjects()` method:
   ```go
   type projectsLoadedMsg struct { projects []discovery.Project }

   func (v *BigendView) loadProjects() tea.Cmd {
       if v.scanner == nil { return nil }
       s := v.scanner
       return func() tea.Msg {
           projects, _ := s.Scan()
           return projectsLoadedMsg{projects: projects}
       }
   }
   ```

4. Handle `projectsLoadedMsg` in `Update()`: store projects, do NOT auto-select.

5. Add `loadProjects()` to `Init()`, `Focus()`, and refresh key handler.

6. In `main.go`: create `discovery.Scanner` with scan roots `[cwd, $HOME/projects]` and call `bigend.SetScanner(scanner)`.

### Tests
- Verify `projectsLoadedMsg` handler stores projects correctly.

---

## Task 2: Project Sidebar

**Files:** `internal/tui/views/bigend.go`

### Changes

1. Modify `SidebarItems()` to return project entries:
   ```go
   func (v *BigendView) SidebarItems() []pkgtui.SidebarItem {
       var items []pkgtui.SidebarItem
       // "All Projects" sentinel
       items = append(items, pkgtui.SidebarItem{
           ID: "__all_projects", Label: "All Projects", Icon: allIcon,
       })
       for _, p := range v.projects {
           items = append(items, pkgtui.SidebarItem{
               ID: p.Path, Label: p.Name, Icon: projectIcon(p),
           })
       }
       return items
   }
   ```

2. Add `projectIcon(p)` helper: shows tooling indicators (e.g., "IC" if HasIntercore, "CW" if HasColdwine).

3. Handle `SidebarSelectMsg` in `Update()`:
   - `__all_projects` → set `activeProject = -1`
   - Project path → find in `projects` slice, set `activeProject = index`
   - Reload data: `loadSessions() + loadRuns() + loadDispatches()`

4. Replace `projectID`/`projectName` scalar fields with `activeProject` index.

### Tests
- Verify sidebar items include "All Projects" + discovered projects.
- Verify `SidebarSelectMsg` switches project and triggers reload.

---

## Task 3: Project-Scoped Data Loading

**Files:** `internal/tui/views/bigend.go`

### Changes

1. Modify `loadSessions()`: if `activeProject >= 0`, use `client.WithProject(projects[activeProject].Name)` to scope the query. Otherwise load all.
   - Note: `WithProject()` mutates the client. To avoid races, create a shallow copy or pass project as parameter to the loader.
   - Simpler: pass project filter directly in URL query params via a new helper method, or filter client-side after loading.

2. Modify `loadRuns()`: if `activeProject >= 0`, filter returned runs by `Run.ProjectDir == projects[activeProject].Path`.

3. Modify `loadDispatches()`: filter dispatches whose `RunID` matches a run belonging to the active project. Since dispatches have `RunID`, match against the loaded `runs` slice.

4. For "All Projects" mode (`activeProject == -1`): no filtering (today's behavior).

5. Update `renderSessionsPane()`: show active project name in the pane title.

### Tests
- Verify project-scoped loading filters correctly.
- Verify "All" mode returns all data.

---

## Task 4: Client Project Scoping (Safe Pattern)

**Files:** `pkg/autarch/client.go`, `internal/tui/views/bigend.go`

### Changes

The `WithProject()` method mutates `c.project` which is unsafe if called from multiple goroutines. Instead:

1. Add `ProjectClient(project string) *Client` method that returns a shallow copy with the project set:
   ```go
   func (c *Client) ProjectClient(project string) *Client {
       clone := *c // shallow copy — shares httpClient
       clone.project = project
       return &clone
   }
   ```

2. In `loadSessions()`, `loadRuns()`, etc.: if active project is set, use `v.client.ProjectClient(name)` for the query. The clone is GC'd after the goroutine completes.

3. Remove the `projectID`/`projectName` fields from BigendView — they're replaced by `activeProject` index into `projects`.

4. Update `SetProjectContext()` callers to set `activeProject` instead.

### Tests
- Verify `ProjectClient` returns isolated copy.
- Verify concurrent calls don't race on `c.project`.

---

## Execution Order

Tasks 1-4 are sequential: Task 1 (scanner wiring) → Task 2 (sidebar) → Task 3 (scoped loading) → Task 4 (safe client pattern).

Tasks 1+4 could be parallelized (client safety is independent), but the total scope is small enough to do sequentially.

## Risk Assessment

- **Low risk**: All changes are within BigendView. No cross-view or data model changes.
- **Scanner latency**: Mitigated by running in background goroutine on Focus().
- **Client mutation**: Task 4 addresses the thread-safety concern with ProjectClient().
- **Backward compatibility**: "All Projects" mode is the default, matching today's behavior exactly.
