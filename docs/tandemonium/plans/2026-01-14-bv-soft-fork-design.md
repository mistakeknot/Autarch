# Beads Viewer Soft-Fork UI/UX Design

**Date:** 2026-01-14

## Overview
Tandemonium will adopt a beads_viewer-inspired TUI via a soft fork: we will selectively reuse UI/UX patterns and (where appropriate) code under MIT while keeping Tandemonium's task model (SQLite + YAML specs) and CLI intact. The UI goal is a full-screen, dense two-pane layout that mirrors beads_viewer ergonomics: a compact list on the left and a rich detail pane on the right. We will preserve TUI-first behavior and add CLI "robot" outputs in later phases. This design is phased to reduce risk and keep the current Go TUI stable.

## Phase 1: TUI Parity (Layout + Search + Markdown + Live Reload)
The Phase 1 TUI will render full-screen by tracking `WindowSizeMsg` and padding/trim to terminal height. The left pane becomes a compact, columnar list (type/priority/status/id/title/age/assignee when available). The right pane starts with a small header grid (id/status/priority/assignee/created/labels) and then renders the rest of the detail as markdown (summary, AC, review notes, last activity). Search (`/`) opens a prompt with fuzzy matching across id/title/labels. Filters are single-key toggles (e.g., `o` open/active, `r` review, `d` done, `a` all). Live reload refreshes on a timer and, when possible, a file watcher for spec changes.

## Phase 2: Dependency Model + Insights + Graph + CLI Robots
We add dependency capture (from spec YAML + runtime signals) and compute insights: critical path, cycles, betweenness/centrality, priority recommendations, and quick wins. A graph view and insights panel appear in the TUI, and CLI "robot" commands output structured summaries (triage, plan, insights, next, priority). These outputs are deterministic and AI-friendly.

## Phase 3: Kanban + History + Exports
We add a kanban view, time-travel/history, and export formats (JSON/CSV). This phase focuses on workflow polish, data portability, and more tooling-friendly output.

## Licensing / Attribution
Any reused beads_viewer source will retain MIT headers, and we will add a third-party notice with the MIT text and attribution.
