# TUI Polish Design (Empty State + Table Styling + Detail Framing)

**Date:** 2026-01-14

## Overview
This design makes the TUI feel less barebones by improving three areas at once: (1) a richer empty state that guides first-time users, (2) denser, clearer table styling for the task list, and (3) a framed detail pane with structured sections even when no task is selected. The goal is to preserve the current two-pane layout while adding the visual scaffolding seen in beads_viewer: a strong header strip, compact column headers, and consistent section titles. All changes are text-only, ANSI-friendly, and fit within the current Bubble Tea view rendering.

## Empty State
When there are zero tasks (or no matches), the left pane should show a short "Quick start" block with numbered steps (init project, create a task, start/review). This provides immediate guidance without requiring a separate help modal. The right pane should show placeholder metadata (ID/Status/Priority/Assignee/Created/Labels) and placeholder sections (Summary, Acceptance Criteria, Recent Activity) with muted dashes. This maintains a stable layout and helps new users understand what appears once tasks exist.

## Table Styling
The task list should render as a compact table with a consistent header row and thin divider, using fixed-width columns and status badges. We will add subtle spacing and separators so the list feels intentional rather than "dumped". Alternating row dimming can be simulated with light ANSI gray on non-selected rows. The selected row keeps a stronger indicator.

## Detail Framing
The right pane starts with a compact header grid (ID/Status/Priority/Assignee/Created/Labels). Below that, the markdown-rendered detail is organized under section headers (Summary, Acceptance Criteria, Recent Activity). If content is missing, placeholders are shown. This makes the pane feel structured regardless of whether a task is selected, and aligns with beads_viewer style without copying its code.
