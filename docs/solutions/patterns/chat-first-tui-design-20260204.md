---
title: "Chat-First TUI Design: Keybindings, Slash Commands, and Layout"
category: patterns
tags: [tui, chat-first, keybindings, slash-commands, layout, accessibility, bubble-tea]
module: pkg/tui, internal/gurgeh/tui
symptom: "Function keys conflict with MacBook; single-letter shortcuts conflict with chat typing"
root_cause: "TUI designed as content-first (IDE pattern) rather than chat-first (Claude Code pattern)"
date_resolved: "2026-02-04"
commits:
  - 2e80594  # chat-friendly keybindings
  - 970a8bc  # fuzzy command picker
  - 899ad15  # 50/50 split layout
  - 336aeb1  # remove edit mode
  - 22ff962  # log pane visual consistency
---

# Chat-First TUI Design

This document captures the design philosophy and implementation patterns for Autarch's chat-focused TUI, established during the Gurgeh dogfooding session on 2026-02-04.

## Problem

The original TUI used:
- **Function keys** (F1-F12) — Require Fn modifier on MacBooks
- **Single-letter shortcuts** (`?`, `q`, `j/k`) — Conflict with typing in chat composer
- **66/34 layout split** — Document pane dominates, minimizing chat importance
- **Edit mode** — Modal switching between viewing and editing

These patterns work for content-first applications (IDEs) but conflict with chat-centric workflows like Claude Code.

## Solution: Chat-First Philosophy

### 1. Ctrl+ Keybindings

All shortcuts use `Ctrl+` combinations to avoid conflicts with natural typing:

```go
// pkg/tui/keys.go
Quit: key.NewBinding(
    key.WithKeys("ctrl+c"),
    key.WithHelp("ctrl+c×2", "quit"),  // Double-press to quit
),
```

| Old | New | Rationale |
|-----|-----|-----------|
| `?` | `F1` or `/help` | `?` is a typeable character |
| `q` | `Ctrl+C` twice | `q` conflicts with typing |
| `j/k` | `↑/↓` | Arrow keys don't conflict |
| `F2` | `Ctrl+G` | MacBook-friendly |
| `Ctrl+D` | `Ctrl+X` | `Ctrl+D` is EOF in terminals |

### 2. Slash Commands with Fuzzy Finder

Type `/` to open a command picker with fuzzy search:

```go
// pkg/tui/command_picker.go
type SlashCommandDef struct {
    Command     string   // "help"
    Aliases     []string // ["h"]
    Description string   // "Show help overlay"
    Category    string   // "global", "kickoff", "sprint"
}

// Fuzzy matching on command names, aliases, and descriptions
func fuzzyMatch(target, query string) bool {
    // Substring match first (fast path)
    if strings.Contains(target, query) {
        return true
    }
    // Character-by-character sequence match
    // "stg" matches "settings"
    // ...
}
```

**Key behaviors:**
- Picker appears when input starts with `/`
- Real-time filtering as user types
- `↑/↓` to navigate, `Tab/Enter` to select, `Esc` to dismiss
- Prefix matches sort before fuzzy matches
- View-specific commands overlay global commands

### 3. 50/50 Split Layout

Changed from 66/34 to equal split:

```go
// pkg/tui/shelllayout.go
func NewShellLayout() *ShellLayout {
    return &ShellLayout{
        splitLayout: NewSplitLayout(0.5),  // Was 0.66
        // ...
    }
}
```

**Rationale:** In a chat-first TUI, the chat pane is the primary interaction surface, not secondary to the document view. Equal weighting reflects equal importance.

### 4. No Edit Mode

Removed `/edit` command entirely. Users refine content via:
1. **Chat feedback** — "Make it more focused on developers"
2. **Direct file editing** — Open spec files in vim/VS Code

```go
// Removed from SprintCommands:
// {Command: "edit", Aliases: []string{"e"}, ...}
```

**Rationale:** Eliminates modal complexity. The chat IS the editing interface.

### 5. Visual Consistency

All panes use consistent styling:

```go
// pkg/tui/logpane.go - matches SplitLayout panel headers
header := lipgloss.NewStyle().
    Foreground(ColorPrimary).
    Bold(true).
    Background(ColorBgDark).  // Dark header background
    Padding(0, 1).            // Consistent padding
    Render("Logs")
```

No borders on content panes — the layout provides visual separation.

## Implementation Patterns

### ChatPanel Slash Command Integration

```go
// pkg/tui/chatpanel.go
func (p *ChatPanel) Update(msg tea.Msg) (*ChatPanel, tea.Cmd) {
    if keyMsg, ok := msg.(tea.KeyMsg); ok {
        // Handle picker FIRST (higher priority than composer)
        if p.commandPicker != nil && p.commandPicker.Visible() {
            selectedCmd, consumed := p.commandPicker.Update(keyMsg)
            if selectedCmd != "" {
                p.composer.SetValue("/" + selectedCmd + " ")
                return p, nil
            }
            if consumed {
                return p, nil  // Don't pass to composer
            }
        }
    }
    // Then pass to composer...
}

// Auto-show picker when typing /
func (p *ChatPanel) updateCommandPicker() {
    value := p.composer.Value()
    if strings.HasPrefix(value, "/") {
        query := strings.TrimPrefix(value, "/")
        if !strings.Contains(query, " ") {  // Still typing command
            p.commandPicker.Show(query)
        }
    } else {
        p.commandPicker.Hide()
    }
}
```

### View-Specific Commands

Each view registers its commands:

```go
// internal/tui/views/kickoff.go
chatPanel := pkgtui.NewChatPanel()
chatPanel.SetViewCommands(pkgtui.KickoffCommands())  // Adds /scan, /new, /delete

// internal/tui/views/sprint_view.go
chatPanel.SetViewCommands(pkgtui.SprintCommands())   // Adds /accept, /1, /2, /3
```

### Double Ctrl+C to Quit

First press clears input, second press quits:

```go
// internal/tui/unified_app.go
if keyMsg.Type == tea.KeyCtrlC {
    if view, ok := a.activeView().(inputClearer); ok {
        if view.HasInput() {
            view.ClearInput()
            return a, nil  // First press: clear
        }
    }
    if time.Since(a.lastCtrlC) < 500*time.Millisecond {
        return a, tea.Quit  // Second press: quit
    }
    a.lastCtrlC = time.Now()
}
```

## Design Principles

1. **Chat is primary** — Layout, keybindings, and workflow center on conversation
2. **Typing safety** — No shortcut should conflict with natural text input
3. **Discoverability** — Slash commands + fuzzy finder replace memorization
4. **Platform consistency** — Ctrl+ works everywhere; F-keys are fallbacks
5. **Minimal modes** — No edit mode, no modal dialogs when avoidable

## Related Documentation

- [docs/tui/SHORTCUTS.md](../tui/SHORTCUTS.md) — Keybinding safety rules
- [docs/QUICK_REFERENCE.md](../QUICK_REFERENCE.md) — Full command reference
- [AUTARCH_TUI_PATTERNS_REFERENCE.md](AUTARCH_TUI_PATTERNS_REFERENCE.md) — Bubble Tea patterns
- [INLINE_MODE_ARCHITECTURE.md](INLINE_MODE_ARCHITECTURE.md) — Log pane architecture

## Commits

- `2e80594` feat(tui): chat-friendly keybindings and slash commands
- `970a8bc` feat(tui): add fuzzy command picker for slash commands
- `899ad15` feat(tui): use 50/50 split for document and chat panes
- `336aeb1` refactor(tui): remove edit mode in favor of chat-first workflow
- `22ff962` fix(tui): remove border from log pane to match other panes
