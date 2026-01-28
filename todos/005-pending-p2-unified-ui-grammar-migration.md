---
status: pending
priority: p2
issue_id: "005"
tags: [tui, ux, keybindings]
dependencies: []
---

# Migrate TUIs to CommonKeys + HelpOverlay

## Problem Statement

Unified keybinding grammar is defined in `pkg/tui`, but individual tools still use local keymaps and string matching, leaving key conflicts unresolved.

## Findings

- Plan specifies a shared keybinding layer and tool migrations. (`docs/plans/2026-01-27-feat-unified-ui-grammar-plan.md:23-75`)
- Common keybindings exist in `pkg/tui/keys.go`, but no tool imports or uses them. (`pkg/tui/keys.go:8-81`)
- Bigend still defines its own keyMap and bindings. (`internal/bigend/tui/model.go:487-560`)

## Proposed Solutions

### Option 1: Incremental migration by tool

**Approach:**
- Replace Bigend key map with CommonKeys + tool-specific extras
- Migrate Gurgeh and Coldwine to key.Matches using CommonKeys

**Pros:**
- Aligns with plan and SHORTCUTS.md
- Resolves key conflicts progressively

**Cons:**
- Requires careful regression testing in each tool

**Effort:** 4-8 hours

**Risk:** Medium

---

### Option 2: Defer and document conflicts

**Approach:**
- Add explicit conflicts list to SHORTCUTS.md and postpone code migration

**Pros:**
- Minimal code churn

**Cons:**
- Keeps inconsistent UX

**Effort:** 1-2 hours

**Risk:** Medium

## Recommended Action

**To be filled during triage.**

## Technical Details

**Affected files:**
- `docs/plans/2026-01-27-feat-unified-ui-grammar-plan.md:23-75`
- `pkg/tui/keys.go:8-81`
- `internal/bigend/tui/model.go:487-560`

## Acceptance Criteria

- [ ] All four tools use CommonKeys for shared actions (quit/help/search/nav)
- [ ] Help overlay renders from CommonKeys + tool extras
- [ ] Key conflicts resolved per plan
- [ ] docs/tui/SHORTCUTS.md updated to canonical bindings

## Work Log

### 2026-01-28 - Initial Discovery

**By:** Codex

**Actions:**
- Verified CommonKeys exists but no usage in tool models
- Confirmed Bigend still uses local keyMap

**Learnings:**
- Unified UI grammar is only partially implemented
