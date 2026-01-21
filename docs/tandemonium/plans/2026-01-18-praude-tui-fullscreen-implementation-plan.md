# Praude Fullscreen TUI Implementation Plan

## Scope
Implement a fullscreen, document-first TUI with a two-pane layout and always-visible secondary tabs (Research / Tasks / Drift). This plan targets Praude’s UI layer only and does not change storage schemas unless noted.

## Phase 0: Recon & Baseline
- Identify current TUI entry points, view model, and layout helpers.
- Capture minimal snapshots for current layout to prevent regressions.

## Phase 1: Layout Framework
1) **Add full-width top bar + bottom status bar**
   - Create render helpers for top/bottom bars.
   - Expose fields in model: `CurrentPRD`, `RepoPath`, `Mode`, `Focus`, `StatusBadges`.
2) **Two-pane layout**
   - Implement `renderTwoPane` variant that accepts pane ratios (e.g., 70/30).
   - Keep layout stable at small widths (min widths for both panes).

## Phase 2: Primary Pane (Document)
3) **Section-aware document rendering**
   - Define section model: `Section{ID, Title, Body}`.
   - Implement renderer with hierarchy (headers, subheaders, evidence chips).
4) **Document navigation**
   - `j/k` or Up/Down scroll.
   - `[`/`]` or Left/Right (DOC focus) jump to next/previous section.
5) **Inline editing**
   - `enter` toggles section edit.
   - `[enter]` save, `[esc]` cancel.
   - Mark document dirty + update status bar.

## Phase 3: Secondary Pane (Tabs)
6) **Tab bar (always-visible)**
   - Tabs: Research / Tasks / Drift with counts.
   - `1/2/3` select tab; Left/Right when SIDE focus.
7) **Research tab**
   - List evidence refs + claims + suggestions.
   - Preview panel below list (selected item).
8) **Tasks tab**
   - Minimal task list with status badges.
   - Optional selection preview.
9) **Drift tab**
   - Show alignment flags + diff signals (stub initially ok).

## Phase 4: Input & Focus
10) **Focus model**
   - `Tab` toggles DOC/SIDE focus; highlight focused pane header.
   - Status bar shows focus + selection index.
11) **Arrow keys**
   - Up/Down = scroll or selection in focused pane.
   - Left/Right = section jump (DOC) or tab switch (SIDE).
12) **Search**
   - `/` initiates pane-scoped search; query shown in status bar.

## Phase 5: Polishing
13) **Visual hierarchy**
   - Accent color for badges/focus.
   - Quiet headers for panes and tabs.
14) **Empty states**
   - Doc empty prompts.
   - Research/Tasks/Drift empty messaging.

## Tests (TDD)
- Add unit tests for:
  - Top/bottom bar rendering.
  - Pane ratio calculation.
  - Section jump navigation.
  - Tab switching behavior.
  - Focus highlights in header.
  - Research tab list + preview rendering.

## Suggested Execution Order
1. Layout framework + bars
2. Focus model + arrow behavior
3. Document rendering + section jumps
4. Secondary tabs + counts
5. Research tab list + preview
6. Tasks/Drift minimal views
7. Editing + search
8. Visual polish + empty states

## Verification
- Run targeted TUI tests after each phase.
- Full suite: `go test ./internal/tui -v`
