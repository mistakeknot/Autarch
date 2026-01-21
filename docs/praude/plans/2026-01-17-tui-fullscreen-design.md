# Praude Full-Screen TUI Design (Beadsviewer-Inspired)

Date: 2026-01-17

## Goal
Deliver a beautiful, full-screen TUI with a split list/detail layout and
Markdown-rendered detail pane. Match beadsviewer-level polish for navigation,
help, and discoverability without sacrificing speed or clarity.

## Design Principles
- Full-frame UI: header, body, footer on every screen.
- Fast list navigation with visible focus and selection.
- Markdown detail rendering with caching for performance.
- Overlays for help/tutorial instead of external docs.
- Consistent keymap and context-aware key hints.

## Architecture (View Router)
- Introduce a router that manages shared state and active screen.
- Each screen implements:
  - `Update(msg) (Screen, Intent)`
  - `View(snapshot) string`
  - `Title() string`
- Router applies intents to shared state:
  - Launch actions (research/suggestions)
  - Filter/search updates
  - Screen transitions
  - Status messages
- Shared state includes:
  - Summaries list
  - Selected index
  - Active filter/search
  - Last action + severity
  - Render cache keyed by spec ID + modtime

## Screens
1) List/Detail (default)
   - Split view with list and Markdown detail pane.
   - Single-column fallback when width is narrow.
   - Status line in footer for errors/warnings.

2) Interview (guided PRD creation)
   - Full-screen prompt flow.
   - Clear input mode and escape/cancel path.

3) Suggestions Review
   - Toggle per-section acceptance.
   - Confirm/apply with summary.

4) Help Overlay
   - `?` shows keymap and quick actions.
   - Dims background and is dismissible with `Esc`.

5) Tutorial Overlay
   - Backtick `` ` `` opens 3-5 short pages.
   - Focused on core actions (interview, research, suggestions).

## Layout + Visual System
- Header bar: project name + current screen + filter/search.
- Body: split view (35/65 list/detail); single-column on narrow widths.
- Footer bar: key hints, focus indicator, last action status.
- High-contrast theme with accent for selection and warnings.
- Visible focus indicator: `[LIST]` or `[DETAIL]` in header.

## Markdown Detail Pane
- Render PRD details via a Markdown renderer (glamour or equivalent).
- Cache rendered output per spec ID + modtime to avoid re-rendering.
- Fallback to plain text if rendering fails.

## Keymap (Beadsviewer-Style)
- Navigation: `j/k`, arrows; `gg/G` to top/bottom.
- Search: `/` open filter; `Esc` exit filter.
- Help: `?` help overlay; backtick `` ` `` tutorial overlay.
- Actions:
  - `g` interview
  - `r` research + launch
  - `p` suggestions + launch
  - `s` review/apply suggestions
- Focus: `Tab` cycles list/detail; `Enter` toggles detail in single-column mode.
- Quit: `q` or `Ctrl+C`.

Keymap table:

| Key | Action |
| --- | ------ |
| `j` / `k` | Move selection |
| `G` | Jump to bottom |
| `Tab` | Toggle focus list/detail |
| `g` | Guided interview |
| `r` | Research + agent launch |
| `p` | Suggestions + agent launch |
| `s` | Review/apply suggestions |
| `?` | Help overlay |
| `` ` `` | Tutorial overlay |
| `q` | Quit |

## Error Handling
- Status bar shows `info/warn/error` with brief message.
- Overlay for blocking errors (invalid YAML, corrupted spec).
- Non-fatal errors keep current screen active.

## Testing
- Unit: router intents, filter parsing, markdown cache invalidation.
- View snapshots: header/footer, list rendering, markdown detail output.
- Integration: key sequences for interview, research, suggestions.
- Layout: narrow vs wide terminal snapshots.

## Open Questions
- Exact theme palette (default + optional custom theme).
- Markdown renderer choice and performance limits.
- Whether to show a right-side stats gutter in ultra-wide mode.
