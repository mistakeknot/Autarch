# Vauxhall Search + Status Filters Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `Vauxpraudemonium-v2z` (Task reference)

**Goal:** Add a search bar with status-token filtering for Sessions and Agents in the Vauxhall TUI.

**Architecture:** Add filter state and parser to the TUI model, apply filtering when building session/agent lists, and render a filter line under the header. Keep parsing tolerant and UI optional when empty.

**Tech Stack:** Go, Bubble Tea (bubbles/list + textinput), lipgloss

---

### Task 1: Add filter parser + state types

**Files:**
- Modify: `internal/vauxhall/tui/model.go`
- Test: `internal/vauxhall/tui/model_filter_test.go`

**Step 1: Write the failing test**

Create `internal/vauxhall/tui/model_filter_test.go` with:
```go
func TestFilterParsesStatusTokens(t *testing.T) {
	state := parseFilter("!waiting codex")
	if !state.Statuses[tmux.StatusWaiting] {
		t.Fatalf("expected waiting status")
	}
	if len(state.Terms) != 1 || state.Terms[0] != "codex" {
		t.Fatalf("expected codex term")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/vauxhall/tui -run TestFilterParsesStatusTokens -v`
Expected: FAIL with “undefined: parseFilter”

**Step 3: Write minimal implementation**

In `internal/vauxhall/tui/model.go`:
- Add `type FilterState struct { Raw string; Terms []string; Statuses map[tmux.Status]bool }`
- Add `func parseFilter(input string) FilterState`:
  - Split on whitespace
  - If token starts with `!`, map to status (`running`, `waiting`, `idle`, `error`)
  - Unknown tokens go to `Terms`
  - Lowercase tokens for comparison

**Step 4: Run test to verify it passes**

Run: `go test ./internal/vauxhall/tui -run TestFilterParsesStatusTokens -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_filter_test.go
git commit -m "feat(vauxhall): add filter parser for tui"
```

---

### Task 2: Apply filters to sessions

**Files:**
- Modify: `internal/vauxhall/tui/model.go`
- Test: `internal/vauxhall/tui/model_filter_test.go`

**Step 1: Write the failing test**

Add to `model_filter_test.go`:
```go
func TestSessionFilterAppliesStatusAndText(t *testing.T) {
	items := []list.Item{
		SessionItem{Session: aggregator.TmuxSession{Name: "codex-a"}, Status: tmux.StatusWaiting},
		SessionItem{Session: aggregator.TmuxSession{Name: "codex-b"}, Status: tmux.StatusRunning},
		SessionItem{Session: aggregator.TmuxSession{Name: "claude"}, Status: tmux.StatusWaiting},
	}
	state := parseFilter("!waiting codex")
	filtered := filterSessionItems(items, state)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 item, got %d", len(filtered))
	}
	got := filtered[0].(SessionItem)
	if got.Session.Name != "codex-a" {
		t.Fatalf("unexpected session: %s", got.Session.Name)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/vauxhall/tui -run TestSessionFilterAppliesStatusAndText -v`
Expected: FAIL with “undefined: filterSessionItems”

**Step 3: Write minimal implementation**

In `model.go`:
- Add `filterSessionItems(items []list.Item, state FilterState) []list.Item`
- Logic:
  - If `state.Raw == ""`, return items
  - For each `SessionItem`, match status if `state.Statuses` non-empty
  - Match terms by substring against session name + description (lowercase)

**Step 4: Run test to verify it passes**

Run: `go test ./internal/vauxhall/tui -run TestSessionFilterAppliesStatusAndText -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_filter_test.go
git commit -m "feat(vauxhall): filter sessions by query"
```

---

### Task 3: Apply filters to agents

**Files:**
- Modify: `internal/vauxhall/tui/model.go`
- Test: `internal/vauxhall/tui/model_filter_test.go`

**Step 1: Write the failing test**

Add to `model_filter_test.go`:
```go
func TestAgentFilterUsesLinkedSessionStatus(t *testing.T) {
	items := []list.Item{
		AgentItem{Agent: aggregator.Agent{Name: "Copper"}},
		AgentItem{Agent: aggregator.Agent{Name: "Rose"}},
	}
	statusByAgent := map[string]tmux.Status{
		"Copper": tmux.StatusWaiting,
		"Rose": tmux.StatusRunning,
	}
	state := parseFilter("!waiting")
	filtered := filterAgentItems(items, state, statusByAgent)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 item, got %d", len(filtered))
	}
	got := filtered[0].(AgentItem)
	if got.Agent.Name != "Copper" {
		t.Fatalf("unexpected agent: %s", got.Agent.Name)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/vauxhall/tui -run TestAgentFilterUsesLinkedSessionStatus -v`
Expected: FAIL with “undefined: filterAgentItems”

**Step 3: Write minimal implementation**

In `model.go`:
- Add `filterAgentItems(items []list.Item, state FilterState, statusByAgent map[string]tmux.Status) []list.Item`
- Determine status from `statusByAgent[name]` if present; if not, treat as no-status match
- Apply status+term matching same as sessions

**Step 4: Run test to verify it passes**

Run: `go test ./internal/vauxhall/tui -run TestAgentFilterUsesLinkedSessionStatus -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_filter_test.go
git commit -m "feat(vauxhall): filter agents by query"
```

---

### Task 4: Wire filter into TUI state + keybindings

**Files:**
- Modify: `internal/vauxhall/tui/model.go`
- Test: `internal/vauxhall/tui/model_filter_ui_test.go`

**Step 1: Write the failing test**

Create `internal/vauxhall/tui/model_filter_ui_test.go` with:
```go
func TestFilterClearsOnEscape(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m = m.withFilterActive("codex")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.filterInput.Value() != "" {
		t.Fatalf("expected empty filter")
	}
	if mm.filterActive {
		t.Fatalf("expected filter inactive")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/vauxhall/tui -run TestFilterClearsOnEscape -v`
Expected: FAIL with “no field or method withFilterActive”

**Step 3: Write minimal implementation**

In `model.go`:
- Add fields: `filterActive bool`, `filterInput textinput.Model`, `filterState FilterState`
- Add helper `withFilterActive(value string) Model` for tests
- Add keybinding for `/` to focus search input
- On `esc`: clear input, set `filterActive=false`, and reset `filterState`
- On input update: set `filterState = parseFilter(filterInput.Value())`
- Call `filterSessionItems` / `filterAgentItems` inside `updateLists()`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/vauxhall/tui -run TestFilterClearsOnEscape -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_filter_ui_test.go
git commit -m "feat(vauxhall): add filter input and key handling"
```

---

### Task 5: Render filter line

**Files:**
- Modify: `internal/vauxhall/tui/model.go`
- Test: `internal/vauxhall/tui/model_filter_ui_test.go`

**Step 1: Write the failing test**

Add to `model_filter_ui_test.go`:
```go
func TestFilterUIHiddenWhenEmpty(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 80
	m.height = 20
	view := m.View()
	if strings.Contains(view, "Filter:") {
		t.Fatalf("did not expect filter line")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/vauxhall/tui -run TestFilterUIHiddenWhenEmpty -v`
Expected: FAIL after adding filter rendering

**Step 3: Write minimal implementation**

In `model.go`:
- Add `renderFilterLine()` that returns empty string when `filterState.Raw == ""`
- Insert it between header and main content in `View()`
- Style with `LabelStyle` (dim) and clamp width to avoid panic

**Step 4: Run test to verify it passes**

Run: `go test ./internal/vauxhall/tui -run TestFilterUIHiddenWhenEmpty -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_filter_ui_test.go
git commit -m "feat(vauxhall): render filter line"
```

---

### Task 6: Full TUI test run

**Files:**
- Test: `internal/vauxhall/tui/...`

**Step 1: Run full tests**

Run: `go test ./internal/vauxhall/tui -v`
Expected: PASS

**Step 2: Commit plan completion note (optional)**

```bash
git status -sb
```

---

## Notes
- If you want `!unknown` support for agents with no session, add it to the parser + agent filter map and add a test in Task 3.
- If we later want filters shared with web, extract parser/filter helpers to a small `internal/vauxhall/filter` package.

