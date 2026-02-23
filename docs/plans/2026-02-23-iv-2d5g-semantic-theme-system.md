# Plan: Semantic Color Palette and Theme System

**Bead:** iv-2d5g

## Problem

Autarch's TUI uses flat package-level `var` constants in `pkg/tui/colors.go` (17 colors) and `pkg/tui/styles.go` (30+ styles). This has three problems:

1. **No theme switching** — hardcoded to Tokyo Night dark. No light mode, no NO_COLOR, no accessible plain mode.
2. **No semantic layer** — components use `ColorMuted` for 3+ distinct purposes (dimmed text, borders, disabled state). Changing one meaning changes all.
3. **No agent-color standardization** — agent badge colors are scattered across individual style vars.

## Solution

Port the two-tier theme system from `research/ntm/internal/tui/theme/` into `apps/autarch/pkg/tui/theme/`. Create backwards-compatible bridge constants in `pkg/tui/colors.go` so the 38 consuming files don't need immediate migration.

### Key design decisions

- **Preserve Autarch's agent colors** — Claude=`#e07353` (coral), Codex=`#00D4AA` (teal), Aider=`#14B8A6`, Cursor=`#0066FF`. These match brand identities better than NTM's Catppuccin-mapped colors.
- **Default theme = Tokyo Night** (current) mapped to Catppuccin Mocha structure. Autarch users get the same look until they opt in.
- **Bridge pattern** — `pkg/tui/colors.go` becomes thin aliases into `theme.Current().Semantic()`, so `tui.ColorPrimary` still works everywhere.
- **`NO_COLOR` and `AUTARCH_THEME` env vars** — respect the NO_COLOR standard. Theme names: `mocha` (default), `macchiato`, `latte`, `nord`, `plain`.

## Architecture

```
pkg/tui/theme/
  theme.go          — Theme struct, 5 palette presets, Current(), FromName(), NewStyles()
  semantic.go        — SemanticPalette, AgentColor(), StatusColor(), NewSemanticStyles()
  theme_test.go      — Tests for palettes, env var, NO_COLOR, factory
  semantic_test.go   — Tests for semantic mappings, agent/status dispatchers

pkg/tui/colors.go   — Bridge: var ColorPrimary = theme.Current().Semantic().Interactive
pkg/tui/styles.go   — Bridge: var BaseStyle = theme.DefaultStyles().App (or keep manual for now)
```

## Tasks

- [x] **1. Create `pkg/tui/theme/theme.go`** — port from ntm
  - `Theme` struct with 23 base colors + 6 semantic + 4 agent
  - Presets: `CatppuccinMocha` (mapped from current Tokyo Night values), `CatppuccinMacchiato`, `CatppuccinLatte`, `Nord`, `Plain`
  - `NoColorEnabled()` — respects `NO_COLOR` + `AUTARCH_NO_COLOR`
  - `FromName(name string) Theme` — string → palette
  - `Current() Theme` — reads `AUTARCH_THEME` env var, default `mocha`
  - `NewStyles(t Theme) Styles` — factory for pre-built lipgloss styles
  - `DefaultStyles() Styles` — convenience wrapper
  - `(t Theme) Gradient(steps int) []lipgloss.Color`
  - Keep Autarch agent colors: Claude=`#e07353`, Codex=`#00D4AA`, Gemini=`#f9e2af`, User=`#9ece6a`

- [x] **2. Create `pkg/tui/theme/semantic.go`** — port from ntm
  - `SemanticPalette` struct with ~38 role-based colors
  - `(t Theme) Semantic() SemanticPalette`
  - `Semantic()` global convenience
  - `(p SemanticPalette) AgentColor(agentType string) lipgloss.Color` — accepts "claude"|"cc", "codex"|"cod", "gemini"|"gmi", "aider", "cursor", "user"
  - `(p SemanticPalette) StatusColor(status string) lipgloss.Color` — accepts success/warning/error/info/pending/idle/disabled
  - `NewSemanticStyles(t Theme) SemanticStyles`
  - `DefaultSemanticStyles() SemanticStyles`

- [x] **3. Update `pkg/tui/colors.go`** — bridge to theme system
  - Change flat constants to call `theme.Current().Semantic()` for semantic colors
  - Keep same variable names for backwards compatibility
  - Add `ColorInfo` (missing — maps to `StatusInfo`)
  - Map: `ColorPrimary` → `Interactive`, `ColorSecondary` → `Accent2`, `ColorSuccess` → `StatusSuccess`, `ColorWarning` → `StatusWarning`, `ColorError` → `StatusError`, `ColorMuted` → `FgTertiary`, `ColorBg` → `BgPrimary`, etc.
  - Agent colors: `ColorClaude` → `AgentClaude`, `ColorCodex` → `AgentCodex`, etc.

- [x] **4. Tests** — port from ntm, adapted for Autarch values
  - `theme_test.go`: Current(), FromName(), NoColorEnabled(), NewStyles(), Plain theme guard rails
  - `semantic_test.go`: SemanticPalette mappings, AgentColor(), StatusColor(), NewSemanticStyles()
  - Verify bridge: `pkg/tui/colors_test.go` — confirm `tui.ColorPrimary` matches `theme.Current().Semantic().Interactive`

- [x] **5. Verify build** — `go test -race ./pkg/tui/... ./pkg/tui/theme/...`

## Non-Goals

- **Migrating 38 consumer files** to use `theme.Semantic()` directly — that's a follow-up. The bridge in `colors.go` makes this non-blocking.
- **Migrating `styles.go`** to use `theme.NewStyles()` — can happen incrementally.
- **Runtime theme switching** — env var at startup is sufficient for now.
- **SSH/terminal dark/light auto-detection** — port the `detectDarkBackground` logic but don't make it the default (Autarch defaults to dark).

## Files Changed

| File | Change |
|------|--------|
| `pkg/tui/theme/theme.go` | New — Theme struct, palettes, factory |
| `pkg/tui/theme/semantic.go` | New — SemanticPalette, dispatchers |
| `pkg/tui/theme/theme_test.go` | New — palette/factory tests |
| `pkg/tui/theme/semantic_test.go` | New — semantic mapping tests |
| `pkg/tui/colors.go` | Modified — bridge to theme system |

## Color Mapping (Tokyo Night → Catppuccin Mocha structure)

| Autarch current | Hex | → Theme field | → Semantic role |
|-----------------|-----|---------------|-----------------|
| `ColorPrimary` | `#7aa2f7` | `Blue` | `Interactive` |
| `ColorSecondary` | `#bb9af7` | `Mauve` | `Accent2` |
| `ColorSuccess` | `#9ece6a` | `Green` | `StatusSuccess` |
| `ColorWarning` | `#e0af68` | `Yellow` | `StatusWarning` |
| `ColorError` | `#f7768e` | `Red` | `StatusError` |
| `ColorMuted` | `#565f89` | `Overlay` | `FgTertiary` |
| `ColorBg` | `#1a1b26` | `Base` | `BgPrimary` |
| `ColorBgDark` | `#16161e` | `Mantle` | `BgSecondary` |
| `ColorBgLight` | `#24283b` | `Surface0` | `BgTertiary` |
| `ColorBgLighter` | `#292e42` | `Surface1` | `BgHighlight` |
| `ColorFg` | `#c0caf5` | `Text` | `FgPrimary` |
| `ColorFgDim` | `#a9b1d6` | `Subtext` | `FgSecondary` |
| `ColorBorder` | `#3b4261` | `Surface2` | `BorderDefault` |
| `ColorClaude` | `#e07353` | `Claude` | `AgentClaude` |
| `ColorCodex` | `#00D4AA` | `Codex` | `AgentCodex` |
| `ColorAider` | `#14B8A6` | (keep) | `AgentAider` (new) |
| `ColorCursor` | `#0066FF` | (keep) | `AgentCursor` (new) |
