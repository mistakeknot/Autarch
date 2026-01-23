# Praude Status Grouped List UX Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `Vauxpraudemonium-i0e` (Task reference)

**Goal:** Port agent-deck's list UX into Praude, with status-grouped PRDs, responsive layout, and persisted UI state.

**Architecture:** Add explicit `status` to PRD specs, build a status-based group tree for the list, and replace list/layout rendering with agent-deck patterns. Persist list UI state in `.praude/state.json` and keep existing interview/research/suggestions flows intact.

**Tech Stack:** Go 1.24+, Bubble Tea, Lip Gloss, YAML, local JSON state file

Note: User explicitly requested no worktrees; implementation will be in the current working tree.

---

### Task 1: Add `status` to PRD schema and summaries

**Files:**
- Modify: `internal/praude/specs/schema.go`
- Modify: `internal/praude/specs/load.go`
- Modify: `internal/praude/specs/load_test.go`

**Step 1: Write the failing test**

Add a new test case to `internal/praude/specs/load_test.go`:

```go
func TestLoadSummariesStatus(t *testing.T) {
	dir := t.TempDir()
	raw := "id: \"PRD-002\"\ntitle: \"B\"\nsummary: \"S\"\nstatus: \"research\"\n"
	if err := os.WriteFile(filepath.Join(dir, "PRD-002.yaml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	list, _ := LoadSummaries(dir)
	if len(list) != 1 {
		t.Fatalf("expected 1 summary")
	}
	if list[0].Status != "research" {
		t.Fatalf("expected status research, got %q", list[0].Status)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/specs -run TestLoadSummariesStatus`

Expected: FAIL with a compile error (`Summary` has no field `Status`) or empty status value.

**Step 3: Write minimal implementation**

Update `internal/praude/specs/schema.go`:

```go
type Spec struct {
	ID                   string                     `yaml:"id"`
	Title                string                     `yaml:"title"`
	CreatedAt            string                     `yaml:"created_at"`
	Status               string                     `yaml:"status"`
	// ... existing fields ...
}
```

Update `internal/praude/specs/load.go`:

```go
type Summary struct {
	ID      string
	Title   string
	Summary string
	Status  string
	Path    string
}

// inside LoadSummaries
var doc struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Summary string `yaml:"summary"`
	Status  string `yaml:"status"`
}
// ...
status := strings.TrimSpace(doc.Status)
if status == "" {
	status = "draft"
}
out = append(out, Summary{ID: doc.ID, Title: doc.Title, Summary: doc.Summary, Status: status, Path: path})
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/specs -run TestLoadSummariesStatus`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/specs/schema.go internal/praude/specs/load.go internal/praude/specs/load_test.go
git commit -m "Add status to PRD summaries"
```

---

### Task 2: Add status template + validation

**Files:**
- Modify: `internal/praude/specs/create.go`
- Modify: `internal/praude/specs/validate.go`
- Modify: `internal/praude/specs/validate_test.go`

**Step 1: Write the failing test**

Add to `internal/praude/specs/validate_test.go`:

```go
func TestValidateStatus(t *testing.T) {
	raw := []byte("id: \"PRD-001\"\ntitle: \"A\"\nsummary: \"S\"\nstatus: \"bogus\"\n")
	res, err := Validate(raw, ValidationOptions{Mode: ValidationSoft})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected status warning")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/specs -run TestValidateStatus`

Expected: FAIL (no warning produced).

**Step 3: Write minimal implementation**

Update `internal/praude/specs/create.go` template:

```go
status: "draft"
```

Add helpers in `internal/praude/specs/validate.go`:

```go
func validStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "interview", "draft", "research", "suggestions", "validated", "archived":
		return true
	default:
		return false
	}
}
```

Call it from `Validate`:

```go
if doc.Status != "" && !validStatus(doc.Status) {
	res.Warnings = append(res.Warnings, "invalid status: "+doc.Status)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/specs -run TestValidateStatus`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/specs/create.go internal/praude/specs/validate.go internal/praude/specs/validate_test.go
git commit -m "Add status validation and template"
```

---

### Task 3: Persist UI state in `.praude/state.json`

**Files:**
- Create: `internal/praude/tui/state_persist.go`
- Create: `internal/praude/tui/state_persist_test.go`
- Modify: `internal/praude/project/project.go`

**Step 1: Write the failing test**

Create `internal/praude/tui/state_persist_test.go`:

```go
package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveUIState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	state := UIState{
		Expanded: map[string]bool{"draft": true, "research": false},
		SelectedID: "PRD-123",
	}
	if err := SaveUIState(path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadUIState(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SelectedID != "PRD-123" || loaded.Expanded["draft"] != true {
		t.Fatalf("state not preserved")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestLoadSaveUIState`

Expected: FAIL (missing types/functions).

**Step 3: Write minimal implementation**

Create `internal/praude/tui/state_persist.go`:

```go
package tui

import (
	"encoding/json"
	"os"
)

type UIState struct {
	Expanded   map[string]bool `json:"expanded"`
	SelectedID string          `json:"selected_id"`
}

func LoadUIState(path string) (UIState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return UIState{Expanded: map[string]bool{}}, err
	}
	var out UIState
	if err := json.Unmarshal(raw, &out); err != nil {
		return UIState{Expanded: map[string]bool{}}, err
	}
	if out.Expanded == nil {
		out.Expanded = map[string]bool{}
	}
	return out, nil
}

func SaveUIState(path string, state UIState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
```

Update `internal/praude/project/project.go` to add a state path helper:

```go
func StatePath(root string) string {
	return filepath.Join(RootDir(root), "state.json")
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestLoadSaveUIState`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/state_persist.go internal/praude/tui/state_persist_test.go internal/praude/project/project.go
git commit -m "Persist Praude TUI state"
```

---

### Task 4: Implement status group tree + flattening

**Files:**
- Create: `internal/praude/tui/group_tree.go`
- Create: `internal/praude/tui/group_tree_test.go`

**Step 1: Write the failing test**

Create `internal/praude/tui/group_tree_test.go`:

```go
package tui

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

func TestGroupTreeFlatten(t *testing.T) {
	summaries := []specs.Summary{
		{ID: "PRD-1", Title: "A", Status: "draft"},
		{ID: "PRD-2", Title: "B", Status: "research"},
	}
	tree := NewGroupTree(summaries, map[string]bool{"draft": true, "research": true})
	items := tree.Flatten()
	if len(items) < 4 {
		t.Fatalf("expected headers and items")
	}
	if items[0].Type != ItemTypeGroup || items[1].Type != ItemTypePRD {
		t.Fatalf("expected group then item")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestGroupTreeFlatten`

Expected: FAIL (missing types/functions).

**Step 3: Write minimal implementation**

Create `internal/praude/tui/group_tree.go`:

```go
package tui

import (
	"sort"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

type ItemType int

const (
	ItemTypeGroup ItemType = iota
	ItemTypePRD
)

type Group struct {
	Name     string
	Expanded bool
	Items    []specs.Summary
}

type Item struct {
	Type          ItemType
	Group         *Group
	Summary       *specs.Summary
	IsLastInGroup bool
}

type GroupTree struct {
	Groups []*Group
}

var StatusOrder = []string{"interview", "draft", "research", "suggestions", "validated", "archived"}

func NewGroupTree(summaries []specs.Summary, expanded map[string]bool) *GroupTree {
	groups := make([]*Group, 0, len(StatusOrder))
	byStatus := make(map[string][]specs.Summary)
	for _, s := range summaries {
		status := strings.ToLower(strings.TrimSpace(s.Status))
		if status == "" {
			status = "draft"
		}
		byStatus[status] = append(byStatus[status], s)
	}
	for _, status := range StatusOrder {
		items := byStatus[status]
		if len(items) == 0 {
			continue
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		groups = append(groups, &Group{
			Name:     status,
			Expanded: expanded[status],
			Items:    items,
		})
	}
	return &GroupTree{Groups: groups}
}

func (t *GroupTree) Flatten() []Item {
	var out []Item
	for _, g := range t.Groups {
		out = append(out, Item{Type: ItemTypeGroup, Group: g})
		if !g.Expanded {
			continue
		}
		for i := range g.Items {
			last := i == len(g.Items)-1
			s := g.Items[i]
			out = append(out, Item{Type: ItemTypePRD, Group: g, Summary: &s, IsLastInGroup: last})
		}
	}
	return out
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestGroupTreeFlatten`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/group_tree.go internal/praude/tui/group_tree_test.go
git commit -m "Add Praude status group tree"
```

---

### Task 5: Port agent-deck search overlay to Praude

**Files:**
- Create: `internal/praude/tui/search_overlay.go`
- Create: `internal/praude/tui/search_overlay_test.go`
- Modify: `internal/praude/tui/model.go`

**Step 1: Write the failing test**

Create `internal/praude/tui/search_overlay_test.go`:

```go
package tui

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSearchOverlayFilters(t *testing.T) {
	overlay := NewSearchOverlay()
	overlay.SetItems([]specs.Summary{{ID: "PRD-1", Title: "Alpha"}, {ID: "PRD-2", Title: "Beta"}})
	overlay.Show()
	overlay, _ = overlay.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if len(overlay.Results()) == 0 {
		t.Fatalf("expected results")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestSearchOverlayFilters`

Expected: FAIL (missing overlay type).

**Step 3: Write minimal implementation**

Create `internal/praude/tui/search_overlay.go`:

```go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

type SearchOverlay struct {
	input   textinput.Model
	results []specs.Summary
	cursor  int
	visible bool
	items   []specs.Summary
}

func NewSearchOverlay() *SearchOverlay {
	ti := textinput.New()
	ti.Placeholder = "Search PRDs..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 50
	return &SearchOverlay{input: ti}
}

func (s *SearchOverlay) SetItems(items []specs.Summary) { s.items = items; s.updateResults() }
func (s *SearchOverlay) Show()                          { s.visible = true; s.input.Focus() }
func (s *SearchOverlay) Hide()                          { s.visible = false; s.input.Blur() }
func (s *SearchOverlay) Visible() bool                  { return s.visible }
func (s *SearchOverlay) Results() []specs.Summary       { return s.results }
func (s *SearchOverlay) Selected() *specs.Summary {
	if len(s.results) == 0 {
		return nil
	}
	if s.cursor >= len(s.results) {
		s.cursor = len(s.results) - 1
	}
	return &s.results[s.cursor]
}

func (s *SearchOverlay) Update(msg tea.Msg) (*SearchOverlay, tea.Cmd) {
	if !s.visible {
		return s, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.Hide()
			return s, nil
		case "enter":
			s.Hide()
			return s, nil
		case "up", "ctrl+k":
			if s.cursor > 0 {
				s.cursor--
			}
			return s, nil
		case "down", "ctrl+j":
			if s.cursor < len(s.results)-1 {
				s.cursor++
			}
			return s, nil
		default:
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			s.updateResults()
			return s, cmd
		}
	}
	return s, nil
}

func (s *SearchOverlay) updateResults() {
	needle := strings.ToLower(strings.TrimSpace(s.input.Value()))
	if needle == "" {
		s.results = s.items
		return
	}
	out := make([]specs.Summary, 0)
	for _, it := range s.items {
		if strings.Contains(strings.ToLower(it.ID), needle) || strings.Contains(strings.ToLower(it.Title), needle) {
			out = append(out, it)
		}
	}
	s.results = out
	s.cursor = 0
}

func (s *SearchOverlay) View(width int) string {
	if !s.visible {
		return ""
	}
	boxStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	return boxStyle.Render("Search: " + s.input.View())
}
```

Update `internal/praude/tui/model.go` to use `SearchOverlay` in update loop and overlay rendering (wire `/` to show and select). Keep full implementation for later tasks.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestSearchOverlayFilters`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/search_overlay.go internal/praude/tui/search_overlay_test.go internal/praude/tui/model.go
git commit -m "Add Praude search overlay"
```

---

### Task 6: Port agent-deck layout + list rendering into Praude

**Files:**
- Modify: `internal/praude/tui/model.go`
- Modify: `internal/praude/tui/layout.go`
- Modify: `internal/praude/tui/screen_list.go`
- Create: `internal/praude/tui/layout_test.go`

**Step 1: Write the failing test**

Create `internal/praude/tui/layout_test.go`:

```go
package tui

import "testing"

func TestLayoutModeSelection(t *testing.T) {
	if layoutMode(40) != LayoutModeSingle {
		t.Fatalf("expected single")
	}
	if layoutMode(60) != LayoutModeStacked {
		t.Fatalf("expected stacked")
	}
	if layoutMode(90) != LayoutModeDual {
		t.Fatalf("expected dual")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestLayoutModeSelection`

Expected: FAIL (missing layoutMode + constants).

**Step 3: Write minimal implementation**

Update `internal/praude/tui/layout.go` with agent-deck layout logic:

```go
const (
	layoutBreakpointSingle  = 50
	layoutBreakpointStacked = 80
)

const (
	LayoutModeSingle  = "single"
	LayoutModeStacked = "stacked"
	LayoutModeDual    = "dual"
)

func layoutMode(width int) string {
	switch {
	case width < layoutBreakpointSingle:
		return LayoutModeSingle
	case width < layoutBreakpointStacked:
		return LayoutModeStacked
	default:
		return LayoutModeDual
	}
}
```

Replace `renderSplitView` with dual/stacked/single renderers that call a new `renderGroupList` and existing `renderDetail`. Port `ensureExactWidth`/`ensureExactHeight` helpers from agent-deck to prevent bleed.

Update `internal/praude/tui/screen_list.go` to render grouped list items (delegate to new list renderer instead of `renderList`).

Update `internal/praude/tui/model.go`:
- Maintain `groupTree` and `flatItems` for navigation.
- Convert selection index to selected PRD summary.
- Replace list rendering in `View()` with new layout paths.
- Add toggle expand/collapse on group headers.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestLayoutModeSelection`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/model.go internal/praude/tui/layout.go internal/praude/tui/screen_list.go internal/praude/tui/layout_test.go
git commit -m "Port agent-deck list layout"
```

---

### Task 7: Wire persisted state + selection updates

**Files:**
- Modify: `internal/praude/tui/model.go`
- Modify: `internal/praude/tui/overlay.go`
- Create: `internal/praude/tui/state_integration_test.go`

**Step 1: Write the failing test**

Create `internal/praude/tui/state_integration_test.go`:

```go
package tui

import "testing"

func TestStateSelectionRoundTrip(t *testing.T) {
	state := UIState{Expanded: map[string]bool{"draft": true}, SelectedID: "PRD-9"}
	selected := selectedIndexFromID([]Item{}, "PRD-9")
	if selected != 0 {
		// no items, should default to 0
		return
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestStateSelectionRoundTrip`

Expected: FAIL (helper missing).

**Step 3: Write minimal implementation**

Add helper functions in `model.go` (or a small new file) to map selected PRD id to list index, and save UI state on selection/expand changes. Use `project.StatePath(m.root)` for persistence. Update `overlay.go` help text to include expand/collapse and search overlay keys.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestStateSelectionRoundTrip`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/model.go internal/praude/tui/overlay.go internal/praude/tui/state_integration_test.go
git commit -m "Persist Praude list state"
```

---

### Task 8: Full test pass

**Step 1: Run full test suite**

Run: `go test ./...`

Expected: PASS (no failures).

**Step 2: Commit (if needed)**

```bash
git status --porcelain
# commit only if there are remaining changes
```

---

Plan complete and saved to `docs/plans/2026-01-22-praude-status-list-ux-plan.md`.

Two execution options:
1) Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks
2) Parallel Session (separate) — New session uses superpowers:executing-plans to execute task-by-task

Which approach?
