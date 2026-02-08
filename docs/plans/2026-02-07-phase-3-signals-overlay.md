# Phase 3: Signals Overlay Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Goal:** Convert Signals from a removed tab into a toggleable overlay accessible via `/sig` (or `/signals`), rendered on top of the current tool content.

**Architecture:** Create a new `SignalsOverlay` component in `internal/tui/` that wraps the existing `SignalsView` but renders it as a bordered overlay box (like the help overlay and palette) instead of a full-screen 3-pane layout. Add toggle state + rendering + key interception in `UnifiedApp`. The existing `SignalsView` (541 lines, 3-pane ShellLayout) is too heavy for an overlay — we'll create a simplified single-panel renderer that reuses the same data loading and filtering logic.

**Tech Stack:** Go, Bubble Tea v1, lipgloss (Tokyo Night colors from `pkg/tui`)

**Bead:** Autarch-n3e

---

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Reuse existing `SignalsView`? | No — new `SignalsOverlay` struct | `SignalsView` uses `ShellLayout` (3-pane). Overlay needs a single-panel box. Data loading can be shared via composition. |
| Overlay sizing | ~60% width, ~70% height, centered | Consistent with help overlay visual weight but larger since signals is content-rich |
| Keybinding to toggle | `/sig` and `/signals` only (no Ctrl+Shift+S) | BT v1 can't distinguish Ctrl+Shift+S from Ctrl+S (XOFF). Already wired as no-op in `UnifiedApp`. |
| Key interception when open | Esc closes. ↑/↓ navigates. Tab cycles category. Ctrl+R refreshes. All other keys blocked. | Same pattern as palette/help overlays — overlay captures all input until dismissed. |
| Where does overlay live? | `internal/tui/signals_overlay.go` (new file) | It's a shell-level component owned by `UnifiedApp`, not a `views/` component. Similar to how palette lives in `internal/tui/palette.go`. |
| Overlay render order | After palette, before help | Signals is more important than help but less transient than palette |
| Data loading | Lazy — load on first show, refresh on `/sig` if already loaded | Don't load signals at startup; only when user requests. Refresh if they toggle off and on. |

## Existing Code Reference

- **Overlay infrastructure**: `unified_app.go:788` — `overlay(base, overlay string)` method + `insertAt()` helper. Used by palette, chat settings, and help.
- **Key interception pattern**: `unified_app.go:356-370` — help overlay intercepts keys; palette intercepts keys. Each overlay returns early from `Update()` when visible.
- **`/sig` slash command**: `unified_app.go:326-328` — already wired, currently returns nil (no-op placeholder).
- **`SignalsView`**: `views/signals.go` — 541 lines. Has `loadData()` (line 312) that queries the event store. Has `filteredSignals()`, `filteredEvents()`, rendering helpers. Uses `ShellLayout` (3-pane) which we won't use.
- **Signal types**: `pkg/signals/signal.go` — `Signal` struct with `Type`, `Severity`, `Source`, `Title`, `Detail`, `CreatedAt`.
- **Event store**: `pkg/events/` — `OpenStore("")` opens SQLite, `Query(filter)` fetches events.
- **Help overlay box style**: `unified_app.go:768-774` — `lipgloss.NewStyle()` with `Background(ColorBgLight)`, `Border(RoundedBorder)`, `BorderForeground(ColorPrimary)`, `Padding(1,3)`, `Width(50)`.

---

### Task 1: Create SignalsOverlay component with rendering

**Files:**
- Create: `internal/tui/signals_overlay.go`
- Test: `internal/tui/signals_overlay_test.go`

This task creates the overlay component that loads and renders signals data in a single-panel overlay box. No `UnifiedApp` integration yet — just the component and its tests.

**Step 1: Write the failing test**

Create `internal/tui/signals_overlay_test.go`:

```go
package tui

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/signals"
)

func TestSignalsOverlayToggle(t *testing.T) {
	o := NewSignalsOverlay()
	if o.Visible() {
		t.Fatal("expected overlay to start hidden")
	}
	o.Toggle()
	if !o.Visible() {
		t.Fatal("expected overlay to be visible after toggle")
	}
	o.Toggle()
	if o.Visible() {
		t.Fatal("expected overlay to be hidden after second toggle")
	}
}

func TestSignalsOverlayRenderEmpty(t *testing.T) {
	o := NewSignalsOverlay()
	o.SetSize(80, 24)
	o.Toggle()
	view := o.View()
	if view == "" {
		t.Fatal("expected non-empty view when overlay is visible")
	}
}

func TestSignalsOverlayRenderWithSignals(t *testing.T) {
	o := NewSignalsOverlay()
	o.SetSize(80, 24)
	o.Toggle()
	o.signals = []signals.Signal{
		{
			ID:        "sig-001",
			Type:      signals.SignalCompetitorShipped,
			Source:    "pollard",
			Severity:  signals.SeverityWarning,
			Title:     "Competitor launched feature X",
			CreatedAt: time.Now(),
		},
	}
	view := o.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestSignalsOverlayClose(t *testing.T) {
	o := NewSignalsOverlay()
	o.Toggle()
	o.Close()
	if o.Visible() {
		t.Fatal("expected overlay to be hidden after Close()")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestSignalsOverlay -v`
Expected: FAIL — `NewSignalsOverlay` undefined.

**Step 3: Write minimal implementation**

Create `internal/tui/signals_overlay.go`:

```go
package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/pkg/events"
	"github.com/mistakeknot/autarch/pkg/signals"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// SignalsOverlay renders signals and events as a toggleable overlay panel.
type SignalsOverlay struct {
	visible bool
	width   int
	height  int
	loaded  bool

	signals []signals.Signal
	events  []*events.Event

	selected int
	category int // 0=signals, 1=events
}

// NewSignalsOverlay creates a new signals overlay.
func NewSignalsOverlay() *SignalsOverlay {
	return &SignalsOverlay{}
}

// Visible returns whether the overlay is currently shown.
func (o *SignalsOverlay) Visible() bool {
	return o.visible
}

// Toggle toggles overlay visibility. Returns a load command if becoming visible.
func (o *SignalsOverlay) Toggle() tea.Cmd {
	o.visible = !o.visible
	if o.visible {
		o.selected = 0
		return o.loadData()
	}
	return nil
}

// Close hides the overlay.
func (o *SignalsOverlay) Close() {
	o.visible = false
}

// SetSize sets the available terminal dimensions.
func (o *SignalsOverlay) SetSize(w, h int) {
	o.width = w
	o.height = h
}

type signalsOverlayLoadedMsg struct {
	signals []signals.Signal
	events  []*events.Event
	err     error
}

// Update processes messages when the overlay is visible.
// Returns true if the message was consumed (caller should not propagate).
func (o *SignalsOverlay) Update(msg tea.Msg) (consumed bool, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case signalsOverlayLoadedMsg:
		if msg.err == nil {
			o.signals = msg.signals
			o.events = msg.events
			o.loaded = true
		}
		return true, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			o.Close()
			return true, nil
		case "up", "k":
			if o.selected > 0 {
				o.selected--
			}
			return true, nil
		case "down", "j":
			o.selected = clampOverlay(o.selected+1, 0, o.currentListLen()-1)
			return true, nil
		case "tab":
			o.category = (o.category + 1) % 2
			o.selected = 0
			return true, nil
		case "ctrl+r":
			return true, o.loadData()
		}
		// Consume all other keys while overlay is visible
		return true, nil
	}
	return false, nil
}

// View renders the overlay box.
func (o *SignalsOverlay) View() string {
	// Overlay dimensions: ~60% width, ~60% height
	overlayWidth := max(40, o.width*60/100)
	overlayHeight := max(10, o.height*60/100)

	var lines []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)

	categoryLabels := []string{"Signals", "Events"}
	var titleParts []string
	for i, label := range categoryLabels {
		if i == o.category {
			titleParts = append(titleParts, titleStyle.Render(fmt.Sprintf("[%s]", label)))
		} else {
			titleParts = append(titleParts, pkgtui.LabelStyle.Render(fmt.Sprintf(" %s ", label)))
		}
	}
	lines = append(lines, strings.Join(titleParts, "  "))
	lines = append(lines, "")

	// Content
	if !o.loaded {
		lines = append(lines, pkgtui.LabelStyle.Render("Loading..."))
	} else {
		switch o.category {
		case 0:
			lines = append(lines, o.renderSignalsList(overlayWidth-8)...)
		case 1:
			lines = append(lines, o.renderEventsList(overlayWidth-8)...)
		}
	}

	// Footer help
	lines = append(lines, "")
	lines = append(lines, pkgtui.LabelStyle.Render("tab category  ↑/↓ navigate  ctrl+r refresh  esc close"))

	// Truncate to fit
	maxContentLines := overlayHeight - 4 // account for box padding
	if len(lines) > maxContentLines {
		lines = lines[:maxContentLines]
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	boxStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBgLight).
		Foreground(pkgtui.ColorFg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pkgtui.ColorPrimary).
		Padding(1, 3).
		Width(overlayWidth).
		Height(overlayHeight)

	return boxStyle.Render(content)
}

func (o *SignalsOverlay) renderSignalsList(width int) []string {
	if len(o.signals) == 0 {
		return []string{pkgtui.LabelStyle.Render("No signals.")}
	}
	var lines []string
	for i, sig := range o.signals {
		severity := signalSeverityIcon(sig.Severity)
		title := sig.Title
		if title == "" {
			title = sig.Detail
		}
		if title == "" {
			title = sig.ID
		}
		line := fmt.Sprintf("%s %s  %s  %s", severity, sig.CreatedAt.Format("Jan02 15:04"), sig.Source, title)
		if i == o.selected {
			line = lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary).Bold(true).Render("› " + line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (o *SignalsOverlay) renderEventsList(width int) []string {
	if len(o.events) == 0 {
		return []string{pkgtui.LabelStyle.Render("No events.")}
	}
	var lines []string
	for i, evt := range o.events {
		line := fmt.Sprintf("• %s  %s  %s", evt.CreatedAt.Format("Jan02 15:04"), evt.EventType, evt.EntityID)
		if i == o.selected {
			line = lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary).Bold(true).Render("› " + line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (o *SignalsOverlay) currentListLen() int {
	if o.category == 0 {
		return len(o.signals)
	}
	return len(o.events)
}

func (o *SignalsOverlay) loadData() tea.Cmd {
	return func() tea.Msg {
		store, err := events.OpenStore("")
		if err != nil {
			return signalsOverlayLoadedMsg{err: err}
		}
		defer store.Close()

		filter := events.NewEventFilter().WithLimit(100)
		evs, err := store.Query(filter)
		if err != nil {
			return signalsOverlayLoadedMsg{err: err}
		}

		// newest first
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].CreatedAt.After(evs[j].CreatedAt) })

		var sigs []signals.Signal
		var otherEvents []*events.Event
		for _, evt := range evs {
			if evt.EventType == events.EventSignalRaised || evt.EventType == events.EventSignalDismissed {
				var sig signals.Signal
				if err := json.Unmarshal(evt.Payload, &sig); err == nil && sig.ID != "" {
					sigs = append(sigs, sig)
					continue
				}
			}
			otherEvents = append(otherEvents, evt)
		}

		return signalsOverlayLoadedMsg{signals: sigs, events: otherEvents}
	}
}

func signalSeverityIcon(s signals.Severity) string {
	switch s {
	case signals.SeverityCritical:
		return "!!"
	case signals.SeverityWarning:
		return "! "
	default:
		return "  "
	}
}

func clampOverlay(val, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestSignalsOverlay -v`
Expected: PASS (4 tests)

**Step 5: Commit**

```bash
git add internal/tui/signals_overlay.go internal/tui/signals_overlay_test.go
git commit -m "feat(tui): add SignalsOverlay component for Phase 3"
```

---

### Task 2: Wire SignalsOverlay into UnifiedApp

**Files:**
- Modify: `internal/tui/unified_app.go` (struct, NewUnifiedApp, Update, View)
- Test: `internal/tui/unified_app_test.go` (add overlay tests)

This task integrates the overlay into the shell — adding toggle state, key interception, rendering, and the `/sig` slash command.

**Step 1: Write the failing tests**

Append to `internal/tui/unified_app_test.go`:

```go
func TestSignalsOverlayToggleViaSig(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.currentView = &noopDashboardView{name: "test"}

	// /sig should toggle overlay on
	updated, _ := app.Update(pkgtui.SlashCommandMsg{Command: "sig"})
	app = updated.(*UnifiedApp)
	if !app.signalsOverlay.Visible() {
		t.Fatal("expected signals overlay to be visible after /sig")
	}

	// /sig again should toggle off
	updated, _ = app.Update(pkgtui.SlashCommandMsg{Command: "sig"})
	app = updated.(*UnifiedApp)
	if app.signalsOverlay.Visible() {
		t.Fatal("expected signals overlay to be hidden after second /sig")
	}
}

func TestSignalsOverlayEscCloses(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.signalsOverlay.Toggle()

	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*UnifiedApp)
	if app.signalsOverlay.Visible() {
		t.Fatal("expected signals overlay to close on Esc")
	}
}

func TestSignalsOverlayBlocksKeysToView(t *testing.T) {
	app := NewUnifiedApp(nil)
	view := &inputFocusView{}
	app.currentView = view
	app.signalsOverlay.Toggle()

	// Type a key — should NOT reach the view
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if view.seen {
		t.Fatal("expected overlay to consume key, but view saw it")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run TestSignalsOverlay -v`
Expected: FAIL — `app.signalsOverlay` undefined.

**Step 3: Modify UnifiedApp**

In `internal/tui/unified_app.go`, make these changes:

**3a.** Add field to `UnifiedApp` struct (after `showHelp`):

```go
	signalsOverlay *SignalsOverlay
```

**3b.** Initialize in `NewUnifiedApp`:

```go
	signalsOverlay: NewSignalsOverlay(),
```

**3c.** Wire `/sig` and `/signals` slash commands (replace the no-op at line ~326):

```go
		case "signals", "sig":
			return a, a.signalsOverlay.Toggle()
```

**3d.** Add key interception in `Update()` for tea.KeyMsg — after chatSettings block, before help overlay:

```go
		if a.signalsOverlay.Visible() {
			consumed, cmd := a.signalsOverlay.Update(msg)
			if consumed {
				return a, cmd
			}
		}
```

**3e.** Add message routing for `signalsOverlayLoadedMsg` in the outer switch:

```go
	case signalsOverlayLoadedMsg:
		_, cmd := a.signalsOverlay.Update(msg)
		return a, cmd
```

**3f.** Add rendering in `View()` — after chatSettings overlay, before help overlay:

```go
	if a.signalsOverlay.Visible() {
		a.signalsOverlay.SetSize(a.width, a.height)
		return a.overlay(result, a.signalsOverlay.View())
	}
```

**3g.** Pass window size to overlay in WindowSizeMsg handler (after palette.SetSize):

```go
		a.signalsOverlay.SetSize(msg.Width, msg.Height)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestSignalsOverlay -v`
Expected: PASS (all 7 tests — 4 from Task 1 + 3 new)

Also run: `go test ./internal/tui/... -v`
Expected: All existing tests still pass.

**Step 5: Commit**

```bash
git add internal/tui/unified_app.go internal/tui/unified_app_test.go
git commit -m "feat(tui): wire SignalsOverlay into UnifiedApp via /sig toggle"
```

---

### Task 3: Add signals overlay to help text and palette

**Files:**
- Modify: `internal/tui/unified_app.go` (footer, help overlay, palette commands)

Small polish task — make the overlay discoverable.

**Step 1: Update footer help text**

In `renderFooterContent()`, add `/sig` to the help string. Change:
```
/big /gur /cold /pol
```
to:
```
/big /gur /cold /pol /sig
```

**Step 2: Add to help overlay global bindings**

In `renderHelpOverlay()`, add to `globalBindings`:
```go
{Key: "/sig", Description: "Toggle signals overlay"},
```

**Step 3: Add palette command**

In `initPaletteCommands()`, add:
```go
cmds = append(cmds, Command{
	Name:        "Signals overlay",
	Description: "Toggle signals and events overlay",
	Action: func() tea.Cmd {
		return a.signalsOverlay.Toggle()
	},
})
```

Also add the same in `updateCommands()`.

**Step 4: Build and test**

Run: `go build ./cmd/... ./internal/... ./pkg/...`
Run: `go test ./internal/tui/... -v`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/tui/unified_app.go
git commit -m "feat(tui): add signals overlay to help text and palette"
```

---

## Verification Checklist

After all tasks:

- [ ] `go build ./cmd/... ./internal/... ./pkg/...` — clean build
- [ ] `go test ./internal/tui/... -v` — all tests pass
- [ ] Manual: `./dev autarch tui` → type `/sig` → overlay appears with signals/events
- [ ] Manual: `esc` closes overlay → returns to previous tool
- [ ] Manual: `/sig` toggles off when already visible
- [ ] Manual: `↑/↓` navigates signals list within overlay
- [ ] Manual: `tab` switches between Signals and Events categories
- [ ] Manual: palette (Ctrl+P) shows "Signals overlay" command
- [ ] Manual: `/help` shows `/sig` in global bindings

## Line Count Estimate

| File | Change |
|------|--------|
| `internal/tui/signals_overlay.go` | +220 new |
| `internal/tui/signals_overlay_test.go` | +60 new |
| `internal/tui/unified_app.go` | +25 modified |
| `internal/tui/unified_app_test.go` | +30 modified |
| **Total** | ~335 lines |
