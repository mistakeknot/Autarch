# Broadcast Confirmation Flow Implementation Plan
**Phase:** executing (as of 2026-02-24T01:55:37Z)

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Goal:** Add a 3-phase confirmation flow (Command → Target → Confirm → Execute) to the Autarch command palette for broadcast actions that send to multiple agent panes.

**Architecture:** Extend the existing `internal/tui/palette.go` with Phase/Target enums and phase-aware Update/View methods. Add `GetAgentPanes()` to the Bigend tmux client for live pane enumeration. Broadcast commands are registered at the unified palette level — any view can trigger them.

**Tech Stack:** Go, Bubble Tea, lipgloss, Bigend tmux client

---

## Review Findings (flux-drive, 3 agents)

Fixes incorporated from plan review (P0/P1 issues):

1. **P0: `pendingCmd` pointer → value copy** — Store `Command` value + `hasPendingCmd bool`, not a pointer into a replaceable slice.
2. **P0: `PaneCountMsg` location** — Place in `internal/tui/messages.go` (canonical message location), not `palette_types.go`.
3. **P0: Missing `tmuxClient` on UnifiedApp** — Create tmux client inside `FetchPaneCounts` closure (no new field needed on UnifiedApp).
4. **P1: Data race in action closures** — Snapshot `BroadcastAction` before `Hide()`, never read palette fields from `tea.Cmd` goroutine.
5. **P1: Duplicate agent-type detection** — Reuse existing `AgentType` from `detector.go`, add `AgentGemini`/`AgentUser`/`AgentUnknown` constants.
6. **P1: Test naming** — Use `fakeRunner` (project convention), not `mockRunner`. Consolidate test helper.
7. **P2: Colon delimiter** — Use tab delimiter in `GetAgentPanes` format string, matching `RefreshCache` pattern.
8. **P2: `exec.ExitError{}` in test** — Replace with `errors.New("exit status 1")`.
9. **P3: `FetchPaneCounts` encapsulation** — Make unexported, add `SetPaneCountFetcher()` setter method.

---

### Task 1: Add Phase/Target Types and Broadcast Field

Extend the Command struct and add new types for the multi-phase palette flow.

**Files:**
- Modify: `pkg/tui/view.go:46-50` (add Broadcast field to Command)
- Create: `internal/tui/palette_types.go` (Phase/Target enums, PaneCounts, BroadcastAction)
- Test: `internal/tui/palette_types_test.go`

**Step 1: Write the failing test**

Create `internal/tui/palette_types_test.go`:

```go
package tui

import "testing"

func TestPhaseString(t *testing.T) {
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseCommand, "command"},
		{PhaseTarget, "target"},
		{PhaseConfirm, "confirm"},
	}
	for _, tt := range tests {
		if got := tt.phase.String(); got != tt.want {
			t.Errorf("Phase(%d).String() = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestTargetLabel(t *testing.T) {
	tests := []struct {
		target Target
		want   string
	}{
		{TargetAll, "All agents"},
		{TargetClaude, "Claude"},
		{TargetCodex, "Codex"},
		{TargetGemini, "Gemini"},
	}
	for _, tt := range tests {
		if got := tt.target.Label(); got != tt.want {
			t.Errorf("Target(%d).Label() = %q, want %q", tt.target, got, tt.want)
		}
	}
}

func TestPaneCountsTotal(t *testing.T) {
	pc := PaneCounts{Claude: 2, Codex: 1, Gemini: 0}
	if got := pc.Total(); got != 3 {
		t.Errorf("Total() = %d, want 3", got)
	}
}

func TestPaneCountsForTarget(t *testing.T) {
	pc := PaneCounts{Claude: 2, Codex: 1, Gemini: 3}
	tests := []struct {
		target Target
		want   int
	}{
		{TargetAll, 6},
		{TargetClaude, 2},
		{TargetCodex, 1},
		{TargetGemini, 3},
	}
	for _, tt := range tests {
		if got := pc.ForTarget(tt.target); got != tt.want {
			t.Errorf("ForTarget(%v) = %d, want %d", tt.target, got, tt.want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/ -run TestPhase -v`
Expected: FAIL — types not defined yet.

**Step 3: Write the types**

Create `internal/tui/palette_types.go`:

```go
package tui

// Phase represents the current palette interaction phase.
type Phase int

const (
	PhaseCommand Phase = iota // Fuzzy command search (existing behavior)
	PhaseTarget               // Select broadcast target (All/Claude/Codex/Gemini)
	PhaseConfirm              // Confirm before executing
)

func (p Phase) String() string {
	switch p {
	case PhaseCommand:
		return "command"
	case PhaseTarget:
		return "target"
	case PhaseConfirm:
		return "confirm"
	default:
		return "unknown"
	}
}

// Target represents the broadcast target group.
type Target int

const (
	TargetAll    Target = iota // All agent panes
	TargetClaude               // Claude panes only
	TargetCodex                // Codex panes only
	TargetGemini               // Gemini panes only
)

func (t Target) Label() string {
	switch t {
	case TargetAll:
		return "All agents"
	case TargetClaude:
		return "Claude"
	case TargetCodex:
		return "Codex"
	case TargetGemini:
		return "Gemini"
	default:
		return "Unknown"
	}
}

// PaneCounts holds live counts of agent panes by type.
type PaneCounts struct {
	Claude int
	Codex  int
	Gemini int
}

// Total returns the total number of agent panes.
func (pc PaneCounts) Total() int {
	return pc.Claude + pc.Codex + pc.Gemini
}

// ForTarget returns the pane count for a specific target.
func (pc PaneCounts) ForTarget(t Target) int {
	switch t {
	case TargetAll:
		return pc.Total()
	case TargetClaude:
		return pc.Claude
	case TargetCodex:
		return pc.Codex
	case TargetGemini:
		return pc.Gemini
	default:
		return 0
	}
}

// BroadcastAction holds the resolved broadcast context passed to the action.
type BroadcastAction struct {
	Target     Target
	PaneCounts PaneCounts
}
```

**Step 4: Add Broadcast field to Command**

In `pkg/tui/view.go`, add to the Command struct:

```go
type Command struct {
	Name        string
	Description string
	Action      func() tea.Cmd
	Broadcast   bool // When true, palette enters target+confirm phases before executing
}
```

**Step 5: Run tests to verify they pass**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/ -run "TestPhase|TestTarget|TestPaneCounts" -v -race`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/tui/palette_types.go internal/tui/palette_types_test.go pkg/tui/view.go
git commit -m "feat(palette): add Phase/Target types and Broadcast field for confirmation flow"
```

---

### Task 2: Add AgentPane and GetAgentPanes to Tmux Client

Add pane enumeration by agent type to the Bigend tmux client.

**Files:**
- Modify: `internal/bigend/tmux/client.go` (add AgentPane type, GetAgentPanes method)
- Create: `internal/bigend/tmux/agent_panes_test.go`

**Step 1: Write the failing test**

Create `internal/bigend/tmux/agent_panes_test.go`:

```go
package tmux

import (
	"errors"
	"testing"
)

// fakeRunner follows project convention (see client_actions_test.go, coldwine/tmux/session_test.go)
type fakeRunnerPanes struct {
	stdout string
	stderr string
	err    error
}

func (f *fakeRunnerPanes) Run(name string, args ...string) (string, string, error) {
	return f.stdout, f.stderr, f.err
}

func TestGetAgentPanes_ParsesOutput(t *testing.T) {
	// Tab-delimited (matching RefreshCache pattern) — safe for pane titles with colons
	runner := &fakeRunnerPanes{
		stdout: "%0\tclaude-agent\tdev\n%1\tcodex-agent\tdev\n%2\tgemini-agent\tdev\n%3\tuser-shell\tdev\n",
	}
	client := NewClientWithRunner(runner)

	panes, err := client.GetAgentPanes("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(panes) != 4 {
		t.Fatalf("got %d panes, want 4", len(panes))
	}

	// Verify agent type detection uses existing AgentType constants
	expected := []struct {
		id        string
		agentType AgentType
	}{
		{"%0", AgentClaude},
		{"%1", AgentCodex},
		{"%2", AgentGemini},
		{"%3", AgentUser},
	}
	for i, e := range expected {
		if panes[i].ID != e.id {
			t.Errorf("panes[%d].ID = %q, want %q", i, panes[i].ID, e.id)
		}
		if panes[i].AgentType != e.agentType {
			t.Errorf("panes[%d].AgentType = %q, want %q", i, panes[i].AgentType, e.agentType)
		}
	}
}

func TestGetAgentPanes_FiltersBySession(t *testing.T) {
	runner := &fakeRunnerPanes{
		stdout: "%0\tclaude-agent\tdev\n%1\tcodex-agent\tother\n%2\tgemini-agent\tdev\n",
	}
	client := NewClientWithRunner(runner)

	panes, err := client.GetAgentPanes("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(panes) != 2 {
		t.Fatalf("got %d panes, want 2 (filtered to 'dev' session)", len(panes))
	}
}

func TestGetAgentPanes_EmptyOnNoServer(t *testing.T) {
	runner := &fakeRunnerPanes{
		stderr: "no server running on /tmp/tmux-1000/default",
		err:    errors.New("exit status 1"),
	}
	client := NewClientWithRunner(runner)

	panes, err := client.GetAgentPanes("dev")
	if err != nil {
		t.Fatalf("expected nil error for no-server, got: %v", err)
	}
	if len(panes) != 0 {
		t.Errorf("expected empty panes, got %d", len(panes))
	}
}

func TestDetectAgentType(t *testing.T) {
	tests := []struct {
		title string
		want  AgentType
	}{
		{"claude-agent", AgentClaude},
		{"Claude Code", AgentClaude},
		{"codex-agent", AgentCodex},
		{"Codex CLI", AgentCodex},
		{"gemini-agent", AgentGemini},
		{"Gemini Pro", AgentGemini},
		{"user-shell", AgentUser},
		{"bash", AgentUser},
		{"zsh", AgentUser},
		{"something-else", AgentUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := detectAgentType(tt.title)
			if got != tt.want {
				t.Errorf("detectAgentType(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/bigend/tmux/ -run "TestGetAgentPanes|TestDetectAgentType" -v`
Expected: FAIL — `GetAgentPanes` and `detectAgentType` not defined.

**Step 3: Write the implementation**

First, add missing agent type constants to `internal/bigend/tmux/detector.go`:

```go
const (
	AgentClaude  AgentType = "claude"
	AgentCodex   AgentType = "codex"
	AgentGemini  AgentType = "gemini"  // NEW
	AgentAider   AgentType = "aider"
	AgentCursor  AgentType = "cursor"
	AgentUser    AgentType = "user"    // NEW
	AgentUnknown AgentType = "unknown" // NEW
)
```

Then add to `internal/bigend/tmux/client.go` (append after `AttachSession`):

```go
// AgentPane represents a tmux pane running an agent.
type AgentPane struct {
	ID        string    // Tmux pane ID (e.g., "%0")
	AgentType AgentType // Reuses AgentType from detector.go
	Title     string    // Raw pane title
}

// GetAgentPanes enumerates panes in the given session, classifying each by agent type.
// Returns empty list (not error) when tmux is unavailable.
func (c *Client) GetAgentPanes(session string) ([]AgentPane, error) {
	// Tab-delimited format — safe for pane titles containing colons.
	// Matches the delimiter pattern used by RefreshCache.
	format := "#{pane_id}\t#{pane_title}\t#{session_name}"
	stdout, stderr, err := c.run("list-panes", "-a", "-F", format)
	if err != nil {
		// Graceful degradation: no server → empty list
		if strings.Contains(stderr, "no server running") ||
			strings.Contains(stderr, "no sessions") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list panes: %w: %s", err, stderr)
	}

	var panes []AgentPane
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		paneID := parts[0]
		title := parts[1]
		sessName := parts[2]

		// Filter to requested session
		if sessName != session {
			continue
		}

		panes = append(panes, AgentPane{
			ID:        paneID,
			AgentType: detectAgentType(title),
			Title:     title,
		})
	}
	return panes, nil
}

// detectAgentType classifies an agent from its pane title.
// Uses AgentType constants from detector.go for type safety.
func detectAgentType(title string) AgentType {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "claude"):
		return AgentClaude
	case strings.Contains(lower, "codex"):
		return AgentCodex
	case strings.Contains(lower, "gemini"):
		return AgentGemini
	case strings.Contains(lower, "user") ||
		strings.Contains(lower, "bash") ||
		strings.Contains(lower, "zsh"):
		return AgentUser
	default:
		return AgentUnknown
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/bigend/tmux/ -run "TestGetAgentPanes|TestDetectAgentType" -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/bigend/tmux/client.go internal/bigend/tmux/agent_panes_test.go
git commit -m "feat(tmux): add GetAgentPanes for pane enumeration by agent type"
```

---

### Task 3: Add Phase-Aware Update Logic to Palette

Extend the palette's Update method to handle PhaseTarget (key 1-4) and PhaseConfirm (Enter/Esc).

**Files:**
- Modify: `internal/tui/palette.go` (add phase fields, phase-aware Update)
- Test: `internal/tui/palette_test.go`

**Step 1: Write the failing test**

Create `internal/tui/palette_test.go`:

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPalette_NonBroadcastSkipsPhases(t *testing.T) {
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Normal", Description: "non-broadcast", Action: func() tea.Cmd { return nil }},
	})
	p.Show()

	// Enter on a non-broadcast command should hide and execute directly
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.Visible() {
		t.Error("palette should hide after entering non-broadcast command")
	}
	if p.phase != PhaseCommand {
		t.Errorf("phase = %v, want PhaseCommand", p.phase)
	}
}

func TestPalette_BroadcastEnterGoesToTarget(t *testing.T) {
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Send Prompt", Description: "broadcast", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()

	// Enter on a broadcast command should transition to target phase
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !p.Visible() {
		t.Error("palette should remain visible in target phase")
	}
	if p.phase != PhaseTarget {
		t.Errorf("phase = %v, want PhaseTarget", p.phase)
	}
}

func TestPalette_TargetPhaseKeySelectsTarget(t *testing.T) {
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()

	// Enter to get to target phase
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Press "2" to select Claude
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if p.phase != PhaseConfirm {
		t.Errorf("phase = %v, want PhaseConfirm", p.phase)
	}
	if p.target != TargetClaude {
		t.Errorf("target = %v, want TargetClaude", p.target)
	}
}

func TestPalette_ConfirmPhaseEnterExecutes(t *testing.T) {
	executed := false
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd {
			executed = true
			return nil
		}},
	})
	p.Show()

	// Command -> Target -> Confirm -> Execute
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})   // -> Target
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}) // -> Confirm (All)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})   // -> Execute

	if !executed {
		t.Error("action should have been executed")
	}
	if p.Visible() {
		t.Error("palette should hide after execution")
	}
	if p.phase != PhaseCommand {
		t.Errorf("phase should reset to PhaseCommand, got %v", p.phase)
	}
}

func TestPalette_EscGoesBackOnePhase(t *testing.T) {
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()

	// Command -> Target
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.phase != PhaseTarget {
		t.Fatalf("expected PhaseTarget, got %v", p.phase)
	}

	// Target -> Confirm
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if p.phase != PhaseConfirm {
		t.Fatalf("expected PhaseConfirm, got %v", p.phase)
	}

	// Confirm -> Target (Esc)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.phase != PhaseTarget {
		t.Errorf("expected PhaseTarget after esc from confirm, got %v", p.phase)
	}

	// Target -> Command (Esc)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.phase != PhaseCommand {
		t.Errorf("expected PhaseCommand after esc from target, got %v", p.phase)
	}
	if !p.Visible() {
		t.Error("palette should still be visible in command phase")
	}

	// Command -> Close (Esc)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.Visible() {
		t.Error("palette should be hidden after esc from command")
	}
}

func TestPalette_CtrlCClosesFromAnyPhase(t *testing.T) {
	for _, startPhase := range []Phase{PhaseCommand, PhaseTarget, PhaseConfirm} {
		t.Run(startPhase.String(), func(t *testing.T) {
			p := NewPalette()
			p.SetCommands([]Command{
				{Name: "Send", Broadcast: true, Action: func() tea.Cmd { return nil }},
			})
			p.Show()

			// Navigate to the target phase
			if startPhase >= PhaseTarget {
				p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
			}
			if startPhase >= PhaseConfirm {
				p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
			}

			p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			if p.Visible() {
				t.Error("ctrl+c should close palette from any phase")
			}
			if p.phase != PhaseCommand {
				t.Errorf("phase should reset to PhaseCommand, got %v", p.phase)
			}
		})
	}
}

func TestPalette_ShowResetsPhasesToCommand(t *testing.T) {
	p := NewPalette()
	p.phase = PhaseConfirm
	p.target = TargetClaude
	p.Show()

	if p.phase != PhaseCommand {
		t.Errorf("Show should reset phase to PhaseCommand, got %v", p.phase)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/ -run "TestPalette_" -v`
Expected: FAIL — `phase`, `target` fields don't exist on Palette.

**Step 3: Modify palette.go — add fields and phase-aware Update**

Add new fields to the `Palette` struct (after `visible bool`):

```go
type Palette struct {
	input    textinput.Model
	commands []Command
	matches  []fuzzy.Match
	selected int
	width    int
	height   int
	visible  bool

	// Broadcast confirmation phases
	phase         Phase
	target        Target
	paneCounts    PaneCounts
	pendingCmd    Command // Value copy of the broadcast command waiting for confirmation
	hasPendingCmd bool    // True when pendingCmd is valid

	// fetchPaneCounts is set by the parent to provide async pane count fetching.
	// When set, it's called as a tea.Cmd when entering PhaseTarget.
	fetchPaneCounts func() tea.Msg
}
```

Update `Show()` to reset phase state:

```go
func (p *Palette) Show() tea.Cmd {
	p.visible = true
	p.phase = PhaseCommand
	p.target = TargetAll
	p.hasPendingCmd = false
	p.input.Reset()
	p.selected = 0
	p.updateMatches()
	return p.input.Focus()
}

// SetPaneCountFetcher sets the async pane count fetch function.
func (p *Palette) SetPaneCountFetcher(fn func() tea.Msg) {
	p.fetchPaneCounts = fn
}
```

Update `Hide()` to reset phase:

```go
func (p *Palette) Hide() {
	p.visible = false
	p.phase = PhaseCommand
	p.hasPendingCmd = false
}
```

Replace the `Update` method's key handling with phase-aware logic:

```go
func (p *Palette) Update(msg tea.Msg) (*Palette, tea.Cmd) {
	if !p.visible {
		return p, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// ctrl+c and q close from any phase
		switch msg.String() {
		case "ctrl+c":
			p.Hide()
			return p, nil
		}

		switch p.phase {
		case PhaseCommand:
			return p.updateCommandPhase(msg)
		case PhaseTarget:
			return p.updateTargetPhase(msg)
		case PhaseConfirm:
			return p.updateConfirmPhase(msg)
		}
	}

	// Non-key messages pass through to text input (command phase only)
	if p.phase == PhaseCommand {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		p.updateMatches()
		p.selected = 0
		return p, cmd
	}

	return p, nil
}

func (p *Palette) updateCommandPhase(msg tea.KeyMsg) (*Palette, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.Hide()
		return p, nil

	case "enter":
		cmd := p.Selected()
		if cmd == nil {
			return p, nil
		}
		if cmd.Broadcast {
			p.pendingCmd = *cmd // Value copy — safe even if SetCommands replaces the slice
			p.hasPendingCmd = true
			p.phase = PhaseTarget
			if p.fetchPaneCounts != nil {
				return p, p.fetchPaneCounts
			}
			return p, nil
		}
		p.Hide()
		return p, cmd.Action()

	case "up", "ctrl+p":
		if p.selected > 0 {
			p.selected--
		}
		return p, nil

	case "down", "ctrl+n":
		if p.selected < len(p.matches)-1 {
			p.selected++
		}
		return p, nil
	}

	// Text input for fuzzy search
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.updateMatches()
	p.selected = 0
	return p, cmd
}

func (p *Palette) updateTargetPhase(msg tea.KeyMsg) (*Palette, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.phase = PhaseCommand
		return p, nil
	case "1":
		p.target = TargetAll
		p.phase = PhaseConfirm
		return p, nil
	case "2":
		p.target = TargetClaude
		p.phase = PhaseConfirm
		return p, nil
	case "3":
		p.target = TargetCodex
		p.phase = PhaseConfirm
		return p, nil
	case "4":
		p.target = TargetGemini
		p.phase = PhaseConfirm
		return p, nil
	}
	return p, nil
}

func (p *Palette) updateConfirmPhase(msg tea.KeyMsg) (*Palette, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.phase = PhaseTarget
		return p, nil
	case "enter":
		if p.hasPendingCmd {
			// Snapshot target context BEFORE Hide() modifies state.
			// Action closures must not read palette fields — they run on a
			// separate goroutine (tea.Cmd), creating a data race.
			action := p.pendingCmd.Action
			_ = BroadcastAction{Target: p.target, PaneCounts: p.paneCounts}
			p.Hide()
			return p, action()
		}
		p.Hide()
		return p, nil
	}
	return p, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/ -run "TestPalette_" -v -race`
Expected: PASS

**Step 5: Build to verify no compile errors**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go build ./cmd/autarch/`
Expected: Clean build (the Broadcast field has zero value `false`, so all existing Command literals continue to work).

**Step 6: Commit**

```bash
git add internal/tui/palette.go internal/tui/palette_test.go
git commit -m "feat(palette): add phase-aware Update with Command→Target→Confirm flow"
```

---

### Task 4: Add Phase-Aware View Rendering

Render different UI for each palette phase — target selection with counts, confirmation screen.

**Files:**
- Modify: `internal/tui/palette.go` (replace View with phase-aware rendering)
- Modify: `internal/tui/palette_test.go` (add view rendering tests)

**Step 1: Write the failing test**

Add to `internal/tui/palette_test.go`:

```go
func TestPalette_ViewTargetPhaseShowsCounts(t *testing.T) {
	p := NewPalette()
	p.SetSize(80, 30)
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()
	p.paneCounts = PaneCounts{Claude: 2, Codex: 1, Gemini: 0}

	// Navigate to target phase
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := p.View()
	if !containsStr(view, "All agents (3)") {
		t.Errorf("target view should show 'All agents (3)', got:\n%s", view)
	}
	if !containsStr(view, "Claude (2)") {
		t.Errorf("target view should show 'Claude (2)', got:\n%s", view)
	}
	if !containsStr(view, "Codex (1)") {
		t.Errorf("target view should show 'Codex (1)', got:\n%s", view)
	}
	if !containsStr(view, "Gemini (0)") {
		t.Errorf("target view should show 'Gemini (0)', got:\n%s", view)
	}
}

func TestPalette_ViewConfirmPhaseShowsDetails(t *testing.T) {
	p := NewPalette()
	p.SetSize(80, 30)
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()
	p.paneCounts = PaneCounts{Claude: 2, Codex: 1, Gemini: 0}

	// Navigate to confirm phase
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter}) // -> Target
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}) // -> Confirm (Claude)

	view := p.View()
	if !containsStr(view, "Send Prompt") {
		t.Errorf("confirm view should show command name 'Send Prompt', got:\n%s", view)
	}
	if !containsStr(view, "Claude") {
		t.Errorf("confirm view should show target 'Claude', got:\n%s", view)
	}
	if !containsStr(view, "enter") {
		t.Errorf("confirm view should show 'enter' key hint, got:\n%s", view)
	}
}

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}
```

Add `"strings"` to the test file imports.

**Step 2: Run tests to verify they fail**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/ -run "TestPalette_View" -v`
Expected: FAIL — view still renders command phase content for all phases.

**Step 3: Replace View method with phase-aware rendering**

Replace the `View()` method in `palette.go`:

```go
func (p *Palette) View() string {
	if !p.visible {
		return ""
	}

	width := p.width
	if width > 60 {
		width = 60
	}

	var content string
	switch p.phase {
	case PhaseTarget:
		content = p.viewTargetPhase(width)
	case PhaseConfirm:
		content = p.viewConfirmPhase(width)
	default:
		content = p.viewCommandPhase(width)
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pkgtui.ColorPrimary).
		Padding(1, 2).
		Width(width)

	return style.Render(content)
}

func (p *Palette) viewCommandPhase(width int) string {
	var b strings.Builder

	title := pkgtui.TitleStyle.Render("Command Palette")
	b.WriteString(title + "\n")
	b.WriteString(p.input.View() + "\n")
	b.WriteString(strings.Repeat("─", width-4) + "\n")

	maxResults := 8
	if p.height > 0 {
		maxResults = min(maxResults, p.height-6)
	}

	for i, match := range p.matches {
		if i >= maxResults {
			break
		}

		cmd := p.commands[match.Index]
		name := cmd.Name
		desc := cmd.Description

		if i == p.selected {
			name = pkgtui.SelectedStyle.Render(name)
		} else {
			name = pkgtui.UnselectedStyle.Render(name)
		}

		desc = pkgtui.LabelStyle.Render(desc)

		line := "  " + name
		if desc != "" {
			line += "  " + desc
		}
		b.WriteString(line + "\n")
	}

	if len(p.matches) == 0 {
		b.WriteString(pkgtui.LabelStyle.Render("  No matching commands\n"))
	}

	return b.String()
}

func (p *Palette) viewTargetPhase(width int) string {
	var b strings.Builder

	cmdName := ""
	if p.hasPendingCmd {
		cmdName = p.pendingCmd.Name
	}
	title := pkgtui.TitleStyle.Render("Select Target: " + cmdName)
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("─", width-4) + "\n")

	targets := []struct {
		key    string
		target Target
	}{
		{"1", TargetAll},
		{"2", TargetClaude},
		{"3", TargetCodex},
		{"4", TargetGemini},
	}

	for _, t := range targets {
		count := p.paneCounts.ForTarget(t.target)
		label := fmt.Sprintf("  %s. %s (%d)", t.key, t.target.Label(), count)
		b.WriteString(label + "\n")
	}

	b.WriteString("\n")
	b.WriteString(pkgtui.LabelStyle.Render("  esc back"))

	return b.String()
}

func (p *Palette) viewConfirmPhase(width int) string {
	var b strings.Builder

	title := pkgtui.TitleStyle.Render("Confirm Broadcast")
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("─", width-4) + "\n")

	cmdName := ""
	if p.hasPendingCmd {
		cmdName = p.pendingCmd.Name
	}

	count := p.paneCounts.ForTarget(p.target)
	b.WriteString(fmt.Sprintf("  Command: %s\n", cmdName))
	b.WriteString(fmt.Sprintf("  Target:  %s (%d)\n", p.target.Label(), count))

	if p.target == TargetAll && p.paneCounts.Total() > 0 {
		b.WriteString(fmt.Sprintf("           Claude(%d) Codex(%d) Gemini(%d)\n",
			p.paneCounts.Claude, p.paneCounts.Codex, p.paneCounts.Gemini))
	}

	b.WriteString("\n")
	b.WriteString(pkgtui.LabelStyle.Render("  enter confirm  esc back"))

	return b.String()
}
```

Add `"fmt"` to the palette.go imports.

**Step 4: Run tests to verify they pass**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/ -run "TestPalette_" -v -race`
Expected: PASS (all palette tests including new view tests).

**Step 5: Build to verify compile**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go build ./cmd/autarch/`

**Step 6: Commit**

```bash
git add internal/tui/palette.go internal/tui/palette_test.go
git commit -m "feat(palette): add phase-aware View rendering for target and confirm screens"
```

---

### Task 5: Wire Async Pane Count Fetching

When the palette enters PhaseTarget, fetch pane counts asynchronously via a Bubble Tea command.

**Files:**
- Modify: `internal/tui/palette.go` (add SetPaneCounts, paneCountMsg, fetch tea.Cmd)
- Modify: `internal/tui/unified_app.go` (wire tmux client to palette, handle paneCountMsg)
- Test: `internal/tui/palette_test.go` (add pane count message tests)

**Step 1: Write the failing test**

Add to `internal/tui/palette_test.go`:

```go
func TestPalette_TargetPhaseReturnsFetchCmd(t *testing.T) {
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()

	// Entering target phase should return a command (for fetching pane counts)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("entering target phase should return a tea.Cmd for fetching pane counts")
	}
}

func TestPalette_PaneCountMsgUpdatesCounts(t *testing.T) {
	p := NewPalette()
	p.phase = PhaseTarget
	p.visible = true

	p, _ = p.Update(PaneCountMsg{Counts: PaneCounts{Claude: 3, Codex: 1, Gemini: 2}})

	if p.paneCounts.Claude != 3 {
		t.Errorf("Claude count = %d, want 3", p.paneCounts.Claude)
	}
	if p.paneCounts.Codex != 1 {
		t.Errorf("Codex count = %d, want 1", p.paneCounts.Codex)
	}
	if p.paneCounts.Gemini != 2 {
		t.Errorf("Gemini count = %d, want 2", p.paneCounts.Gemini)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/ -run "TestPalette_TargetPhaseReturns|TestPalette_PaneCount" -v`
Expected: FAIL — `PaneCountMsg` not defined, target phase doesn't return a cmd.

**Step 3: Add PaneCountMsg and fetch wiring**

Add `PaneCountMsg` to `internal/tui/messages.go` (canonical message location):

```go
// PaneCountMsg carries fetched pane counts back to the palette.
type PaneCountMsg struct {
	Counts PaneCounts
	Err    error
}
```

Note: `PaneCounts` stays in `palette_types.go` since it's a data type, not a message.

The `fetchPaneCounts` field and `SetPaneCountFetcher` setter were already added to the Palette struct in Task 3. The broadcast branch in `updateCommandPhase` already calls `p.fetchPaneCounts`.

Add PaneCountMsg handling in `Update` (before the key switch):

```go
	switch msg := msg.(type) {
	case PaneCountMsg:
		if msg.Err == nil {
			p.paneCounts = msg.Counts
		}
		return p, nil
	case tea.KeyMsg:
		// ... existing key handling
	}
```

**Step 4: Wire in unified_app.go**

`UnifiedApp` has no `tmuxClient` field — the tmux client is used in `internal/bigend/tui/model.go` and `internal/bigend/aggregator/`. Create a fresh tmux client inside the closure (it's lightweight — just finds the tmux binary path):

```go
	// In initPaletteCommands or after palette creation in NewUnifiedApp:
	a.palette.SetPaneCountFetcher(func() tea.Msg {
		client := tmux.NewClient()
		if !client.IsAvailable() {
			return PaneCountMsg{}
		}
		// Use the first session found — in production, Bigend manages a single primary session.
		sessions, err := client.ListSessions()
		if err != nil || len(sessions) == 0 {
			return PaneCountMsg{Err: err}
		}
		panes, err := client.GetAgentPanes(sessions[0].Name)
		if err != nil {
			return PaneCountMsg{Err: err}
		}
		var counts PaneCounts
		for _, pane := range panes {
			switch pane.AgentType {
			case tmux.AgentClaude:
				counts.Claude++
			case tmux.AgentCodex:
				counts.Codex++
			case tmux.AgentGemini:
				counts.Gemini++
			}
		}
		return PaneCountMsg{Counts: counts}
	})
```

Add the import `"github.com/mistakeknot/autarch/internal/bigend/tmux"` to unified_app.go.

**Step 5: Run tests to verify they pass**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/ -run "TestPalette_" -v -race`
Expected: PASS

**Step 6: Build to verify compile**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go build ./cmd/autarch/`

**Step 7: Commit**

```bash
git add internal/tui/palette.go internal/tui/palette_types.go internal/tui/palette_test.go internal/tui/unified_app.go
git commit -m "feat(palette): wire async pane count fetching on target phase entry"
```

---

### Task 6: Register Broadcast Commands in Unified App

Add the initial broadcast commands ("Send Prompt to Agents", "Stop All Agents") to the palette as broadcast-enabled commands.

**Files:**
- Modify: `internal/tui/unified_app.go` (add broadcast commands to updateCommands)

**Step 1: Add broadcast commands**

In `unified_app.go`, inside `updateCommands()`, add broadcast commands before the final `SetCommands` call:

```go
	// Broadcast commands (available from any view)
	cmds = append(cmds, Command{
		Name:        "Send Prompt to Agents",
		Description: "Broadcast a prompt to agent panes",
		Broadcast:   true,
		Action: func() tea.Cmd {
			// TODO: implement actual send-to-panes via tmux SendKeys
			return nil
		},
	})
	cmds = append(cmds, Command{
		Name:        "Stop All Agents",
		Description: "Send interrupt to agent panes",
		Broadcast:   true,
		Action: func() tea.Cmd {
			// TODO: implement actual ctrl-c to panes via tmux SendKeys
			return nil
		},
	})
```

The Action implementations are stubs for now — the actual tmux SendKeys logic depends on the BroadcastAction context (which target was selected). That wiring is deferred per the PRD non-goals ("Custom prompt input — deferred").

**Step 2: Build to verify compile**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go build ./cmd/autarch/`

**Step 3: Run all tests**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/ -v -race`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/tui/unified_app.go
git commit -m "feat(palette): register broadcast commands in unified palette"
```

---

### Task 7: Integration Test — Full Phase Flow

End-to-end test verifying the complete Command → Target → Confirm → Execute flow with pane counts.

**Files:**
- Modify: `internal/tui/palette_test.go` (add integration test)

**Step 1: Write the integration test**

Add to `internal/tui/palette_test.go`:

```go
func TestPalette_FullBroadcastFlow(t *testing.T) {
	executed := false

	p := NewPalette()
	p.SetSize(80, 30)
	p.paneCounts = PaneCounts{Claude: 2, Codex: 1, Gemini: 3}
	p.SetPaneCountFetcher(func() tea.Msg {
		return PaneCountMsg{Counts: PaneCounts{Claude: 2, Codex: 1, Gemini: 3}}
	})

	p.SetCommands([]Command{
		{Name: "Normal Cmd", Action: func() tea.Cmd { return nil }},
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd {
			// Do NOT read p.target or p.paneCounts here — this runs as tea.Cmd
			// on a goroutine, which would race with Update() writing these fields.
			executed = true
			return nil
		}},
	})

	// Open palette
	p.Show()

	// Type to filter to broadcast command
	for _, r := range "Send" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify "Send Prompt" is top match
	sel := p.Selected()
	if sel == nil || sel.Name != "Send Prompt" {
		t.Fatal("expected 'Send Prompt' to be selected after fuzzy search")
	}

	// Enter → Target phase
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.phase != PhaseTarget {
		t.Fatalf("expected PhaseTarget, got %v", p.phase)
	}
	if cmd == nil {
		t.Error("expected fetch cmd from target phase entry")
	}

	// Verify target view shows counts
	view := p.View()
	if !containsStr(view, "All agents (6)") {
		t.Errorf("target view should show total count 6, got:\n%s", view)
	}

	// Select "3" → Codex → Confirm
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if p.phase != PhaseConfirm {
		t.Fatalf("expected PhaseConfirm, got %v", p.phase)
	}
	if p.target != TargetCodex {
		t.Fatalf("expected TargetCodex, got %v", p.target)
	}

	// Verify confirm view
	view = p.View()
	if !containsStr(view, "Send Prompt") {
		t.Errorf("confirm view should show 'Send Prompt'")
	}
	if !containsStr(view, "Codex (1)") {
		t.Errorf("confirm view should show 'Codex (1)'")
	}

	// Enter → Execute
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.Visible() {
		t.Error("palette should hide after execution")
	}
	if !executed {
		t.Error("action should have been executed")
	}
}
```

**Step 2: Run the integration test**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/ -run "TestPalette_FullBroadcastFlow" -v -race`
Expected: PASS

**Step 3: Run ALL tests to verify no regressions**

Run: `cd /home/mk/projects/Demarch/apps/autarch && go test ./internal/tui/... -v -race`
Expected: PASS (all existing + new tests)

**Step 4: Commit**

```bash
git add internal/tui/palette_test.go
git commit -m "test(palette): add full broadcast flow integration test"
```

---

## Summary

| Task | What | Files | Tests |
|------|------|-------|-------|
| 1 | Phase/Target types + Broadcast field | `palette_types.go`, `view.go` | 4 test functions |
| 2 | AgentPane + GetAgentPanes | `client.go`, `agent_panes_test.go` | 4 test functions |
| 3 | Phase-aware Update logic | `palette.go`, `palette_test.go` | 7 test functions |
| 4 | Phase-aware View rendering | `palette.go`, `palette_test.go` | 2 test functions |
| 5 | Async pane count fetching | `palette.go`, `palette_types.go`, `unified_app.go` | 2 test functions |
| 6 | Register broadcast commands | `unified_app.go` | build verify |
| 7 | Full-flow integration test | `palette_test.go` | 1 integration test |

**Total: 7 tasks, 7 commits, ~20 test functions**
