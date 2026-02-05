---
title: "TUI Scrolling - Keyboard and Mouse Focus Issues"
category: ui-bugs
tags: [bubble-tea, tui, focus, keyboard, mouse, scrolling]
module: TUI/SprintView
symptom: "Doc panel doesn't scroll with keyboard or mouse regardless of focus"
root_cause: "Multiple issues: key matching method, focus state source, terminal key conflicts"
date: 2026-02-04
---

# TUI Scrolling - Keyboard and Mouse Focus Issues

## Problems

Three related issues with doc panel scrolling in the SprintView:

1. **Keyboard scrolling didn't work** - Arrow keys and page keys had no effect
2. **Mouse scrolling went to wrong panel** - Mousewheel always scrolled chat, not doc
3. **Key conflicts** - Initial key choices conflicted with chat input or terminal

## Root Causes

### 1. Keyboard Matching Method

The code used `key.Matches()` which requires a pre-configured `key.Binding`:

```go
// BEFORE: Required key binding setup, didn't match raw keys
if key.Matches(msg, v.keys.Up) {
    v.docPanel.ScrollUp()
}
```

Changed to explicit `msg.String()` matching:

```go
// AFTER: Direct string matching works with raw key events
switch msg.String() {
case "up", "ctrl+p":
    v.docPanel.ScrollUp()
case "down", "ctrl+n":
    v.docPanel.ScrollDown()
}
```

### 2. Mouse Focus State

The mouse handler checked `v.chatPanel.Focused()` which is the component's **internal** focus state. But when Tab switches shell focus, only the shell's focus state updates - the chat panel's internal state doesn't change.

```go
// BEFORE: chatPanel.Focused() doesn't update on Tab
case tea.MouseWheelUp:
    if v.chatPanel.Focused() {  // BUG: Always true!
        v.chatPanel.ScrollUp()
    } else {
        v.docPanel.ScrollUp()
    }

// AFTER: Use shell as source of truth
case tea.MouseWheelUp:
    if v.shell.Focus() == pkgtui.FocusChat {
        v.chatPanel.ScrollUp()
    } else {
        v.docPanel.ScrollUp()
    }
```

### 3. Terminal Key Conflicts

Initial key choices had problems:

| Keys | Problem |
|------|---------|
| `j/k` | Conflicted with chat input (typing 'j' would scroll, not type) |
| `Ctrl+J/K` | Ctrl+J = LF (newline) in terminals, same as Enter |
| `Ctrl+N/P` | Safe - Emacs-style prev/next, no terminal conflicts |

## Solution Summary

Final keyboard bindings for doc panel (when focused):

```go
case pkgtui.FocusDocument:
    switch msg.String() {
    case "up", "ctrl+p":
        v.docPanel.ScrollUp()
    case "down", "ctrl+n":
        v.docPanel.ScrollDown()
    case "pgup", "ctrl+u":
        for i := 0; i < 5; i++ { v.docPanel.ScrollUp() }
    case "pgdown", "ctrl+d":
        for i := 0; i < 5; i++ { v.docPanel.ScrollDown() }
    case "home":
        v.docPanel.ScrollToTop()
    }
```

Mouse scrolling uses shell focus state:

```go
case tea.MouseWheelUp:
    if v.shell.Focus() == pkgtui.FocusChat {
        v.chatPanel.ScrollUp()
    } else {
        v.docPanel.ScrollUp()
    }
```

## Key Insights

1. **Shell layout owns focus state** - Components have internal focus state, but the shell layout is the authoritative source when using multi-pane layouts.

2. **Emacs bindings are terminal-safe** - Ctrl+N/P (next/prev) work in tmux, terminals, and don't conflict with common shortcuts.

3. **Explicit string matching is clearer** - `msg.String()` is simpler than `key.Matches()` and doesn't require binding configuration.

4. **Chat-based TUIs need careful key design** - Keys that could be typed (j, k, g) shouldn't be shortcuts when focus could be ambiguous.

## Files Changed

- `internal/tui/views/sprint_view.go`: Keyboard and mouse handling
- `internal/tui/views/doc_panel.go`: Added `ScrollToTop()` method

## Testing

When building TUIs with multiple scrollable panels:
1. Test keyboard scrolling in each focus state
2. Test mouse scrolling in each focus state
3. Test key bindings in actual terminal (not just IDE terminal)
4. Test in tmux to catch prefix conflicts
