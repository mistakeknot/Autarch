# TUI Keyboard Shortcuts Guide

Use this doc whenever adding or editing keyboard shortcuts. The goal is to
avoid terminal/OS collisions, preserve common TUI conventions, and keep
behavior consistent across tools.

## Scope
- Applies to all Autarch TUIs (Bubble Tea + lipgloss).
- Defines safe defaults, common conventions, and keys to avoid.

## Safe Defaults (Low Collision Risk)
- Navigation: arrows, PageUp/PageDown, Home/End
- Actions: Enter (activate), Esc (cancel/back), Tab/Shift+Tab (focus)
- Function keys: F1–F12 (e.g., F2 for Dispatch agent)

## Printable Key Policy (Unified UI)
- Printable keys (letters, digits, symbols, space) are **not** bound anywhere
  in the unified three-pane UI.
- Use function keys and modifier combos for view-specific actions.

## Avoid or Handle Carefully
- `ctrl+z`, `ctrl+\`: terminal signals
- `ctrl+d`: EOF
- `ctrl+s`, `ctrl+q`: flow control (XON/XOFF) unless disabled
- `ctrl+l`: clear screen
- `ctrl+v`, `shift+insert`: paste (should be left to terminal)
- `alt`/`meta` combos: sent as Esc prefix; may collide with Esc handling

## Text Input Rules
- Do not capture printable characters when an input is focused.
- In the unified UI, printable keys are never bound globally or per-view.
- Keep standard editing keys working (arrows, backspace, delete).
- Enter should submit/accept; Esc should cancel/blur.
- `ctrl+c` should always quit from any screen.

## Checklist for New Shortcuts
- Consistent with existing [TUI Keybindings](../../agents/tui-keybindings.md).
- No conflicts with terminal-reserved keys.
- Documented in the tool help overlay and [TUI Keybindings](../../agents/tui-keybindings.md).
- Manually tested in a real terminal (not just IDE terminal).
