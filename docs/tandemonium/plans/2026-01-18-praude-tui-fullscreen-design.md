# Praude Fullscreen TUI Design (Document-First)

## Goals
- Fullscreen, calm, and document-centric TUI for high-quality PRDs/specs.
- Two-pane layout: primary document reading/writing, secondary operational context.
- Always-visible secondary tabs (Research / Tasks / Drift) with Research as default.
- Minimal modes and predictable navigation.

## Layout
- **Top bar (full width):** app name, current PRD ID/title, repo, status badges (Draft/Review/Locked).
- **Primary pane (left, ~65-70%):** PRD document view with section headers, short line length, and inline evidence chips (e.g., [EV-12]).
- **Secondary pane (right, ~30-35%):** tabbed context with counts (Research 12 / Tasks 5 / Drift 2). Compact lists + preview.
- **Bottom bar (full width):** mode (VIEW/EDIT/RESEARCH), focus (DOC/SIDE), selection index, key hints.

## Interaction Model
- **Focus:** `Tab` toggles focus between DOC and SIDE. Focus indicated in header and footer.
- **Document:**
  - `j/k` or Up/Down scroll.
  - `[` and `]` (or Left/Right when DOC focused) jump between sections.
  - `enter` toggles inline edit for the current section; `[enter] save, [esc] cancel`.
- **Side pane:**
  - `1/2/3` select Research/Tasks/Drift.
  - `j/k` or Up/Down move selection.
  - `enter` opens preview or accept/reject (for suggestions).
  - Left/Right switches tabs when SIDE focused.
- **Search:** `/` searches within the focused pane; query shown in status bar.

## Secondary Pane Tabs
- **Research (default):** sources, claims, evidence refs, suggestions; encourages evidence-based edits.
- **Tasks:** execution status, assignments, blockers; keeps coordination available without stealing focus.
- **Drift:** spec vs implementation signals, unresolved changes; quick triage for PM review.

## Visual Language
- Dark, low-glare background; high-contrast text; single accent color for focus/badges.
- Subtle pane headers; focused pane slightly brighter.
- Section headers are primary hierarchy cue; compact separators/underlines.
- Secondary pane remains compact and readable; previews expand within pane without modal takeover.

## Navigation Summary
- Up/Down: scroll/move selection in focused pane.
- Left/Right:
  - SIDE focus -> switch tabs.
  - DOC focus -> section jump.
- Tab: toggle focus DOC/SIDE.

## Notes
- Document-first prioritizes coherent narrative and evidence quality.
- Always-visible tabs improve discoverability while remaining visually quiet.
