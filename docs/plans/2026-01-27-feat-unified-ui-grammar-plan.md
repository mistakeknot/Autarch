# Unified UI Grammar Across Autarch TUIs

## Problem

The four Autarch TUI tools (Bigend, Gurgeh, Coldwine, Pollard) have divergent keyboard handling patterns despite sharing visual styles via `pkg/tui/`. Key inconsistencies:

1. **Keybinding declaration style**: Bigend uses `charmbracelet/bubbles/key` bindings (`key.NewBinding`, `key.Matches`). Gurgeh and Coldwine use raw `msg.String()` string matching. There is no shared keybinding registry.

2. **Quit behavior**: Bigend accepts `q` or `ctrl+c` equally. Coldwine requires double `ctrl+c` (with a timeout window) and has no `q` to quit. Gurgeh uses `q`/`ctrl+c` directly.

3. **Help toggle**: Bigend binds `?` via the key map. Coldwine binds `?` via inline `msg.String()` check. Gurgeh handles `?` inside overlays only.

4. **Search activation**: Both Bigend and Coldwine use `/` for search, but Bigend uses `key.Matches` while Coldwine uses `msg.String() == "/"`. Gurgeh uses a dedicated `SearchOverlay` component with its own key handling.

5. **Navigation keys**: `h` means "toggle archived" in Gurgeh but is unused in Coldwine/Bigend. Bigend uses `[`/`]` for pane focus; Coldwine uses `tab`. `r` means "rename" in Bigend, "refresh" in the SHORTCUTS.md spec, "review" in Coldwine, and "reject suggestions" in Gurgeh.

6. **Back/cancel**: Gurgeh and Coldwine use `b` for "back" in some views; Bigend uses `esc` exclusively. The SHORTCUTS.md spec lists `esc` for cancel/back.

7. **No shared keymap**: `pkg/tui/` has styles, colors, components, and layouts but zero keybinding infrastructure. Each tool reinvents key handling from scratch.

## Proposed Solution

Add a shared keybinding layer to `pkg/tui/` and migrate each tool to use it.

### 1. `pkg/tui/keys.go` -- Shared keymap

Define a `CommonKeys` struct using `charmbracelet/bubbles/key` with standard bindings matching SHORTCUTS.md:

| Action | Keys | Notes |
|--------|------|-------|
| Quit | `q`, `ctrl+c` | All tools |
| Help | `?` | Toggle help overlay |
| Search | `/` | Activate search |
| Back | `esc` | Cancel/dismiss overlays |
| Navigate | `j`/`k`, arrows | Up/down |
| Top/Bottom | `g`/`G` | Jump to extremes |
| Next/Prev | `n`/`p` | Next/prev match or item |
| Refresh | `r` | Reload data |
| Tab cycle | `tab`/`shift+tab` | Focus next/prev pane |
| Select | `enter` | Activate item |
| Toggle | `space` | Toggle selection |
| Sections | `1`-`9` | Switch tabs/sections |

Provide `NewCommonKeys()` that returns pre-configured bindings, and a `HandleCommon(msg tea.KeyMsg, keys CommonKeys) tea.Cmd` helper that returns `tea.Quit`, help toggle commands, etc.

### 2. `pkg/tui/help.go` -- Shared help overlay

A reusable help overlay component that renders keybindings from any `CommonKeys` plus tool-specific extras, using existing `HelpKeyStyle`/`HelpDescStyle`.

### 3. Migrate tools incrementally

- **Phase 1**: Bigend (already uses `key.Binding`; swap local `keyMap` for shared `CommonKeys` + tool-specific extensions).
- **Phase 2**: Gurgeh (replace `msg.String()` switches with `key.Matches`; resolve `h`/`r` conflicts).
- **Phase 3**: Coldwine (replace inline string matching; adopt shared quit/help/search; keep double-`ctrl+c` as an opt-in variant).
- **Phase 4**: Pollard (new tool; use shared keys from the start).

### 4. Resolve letter conflicts

| Key | Current conflict | Resolution |
|-----|-----------------|------------|
| `r` | refresh (spec) vs. rename (Bigend) vs. review (Coldwine) vs. reject (Gurgeh) | `r` = refresh globally; tool-specific actions get different keys |
| `h` | toggle archived (Gurgeh) vs. back (SHORTCUTS.md) | `h` = back/left (per vim convention); archived toggle moves to `H` |
| `b` | back (Coldwine) vs. unused | `esc` = back globally; `b` available for tool-specific use |

## Acceptance Criteria

- [ ] `pkg/tui/keys.go` exports `CommonKeys` struct and `NewCommonKeys()` constructor
- [ ] `pkg/tui/help.go` exports a reusable help overlay component
- [ ] All four tools use `CommonKeys` for quit, help, search, navigation, and tab cycling
- [ ] Tool-specific bindings extend (not replace) the common set
- [ ] No key conflicts between common bindings and tool-specific bindings within any single tool
- [ ] `docs/tui/SHORTCUTS.md` updated to reflect the canonical shared bindings
- [ ] Existing tests updated to use shared key references instead of hardcoded strings
- [ ] `r` consistently means "refresh" across all tools when no context-specific action applies

## Key Files

| File | Role |
|------|------|
| `pkg/tui/keys.go` | New: shared keybinding definitions |
| `pkg/tui/help.go` | New: shared help overlay component |
| `pkg/tui/styles.go` | Existing: already has `HelpKeyStyle`/`HelpDescStyle` |
| `pkg/tui/components.go` | Existing: shared UI components |
| `docs/tui/SHORTCUTS.md` | Existing: canonical shortcut reference |
| `internal/bigend/tui/model.go` | Migrate local `keyMap` to shared `CommonKeys` |
| `internal/gurgeh/tui/model.go` | Replace `msg.String()` with `key.Matches` |
| `internal/coldwine/tui/model.go` | Replace inline string checks with shared keys |
