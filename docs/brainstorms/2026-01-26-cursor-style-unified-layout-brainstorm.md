# Cursor-Style Unified Layout

**Date:** 2026-01-26
**Status:** Ready for Planning

## What We're Building

A unified shell layout for all Autarch TUI views that mirrors the Cursor/VS Code interaction pattern:

```
┌──────────┬─────────────────────────┬──────────────┐
│ Sidebar  │   Main Document Pane    │  Chat Panel  │
│ (toggle) │   (2/3 width)           │  (1/3 width) │
│          │                         │              │
│ • Specs  │   - Selected item       │  Contextual  │
│ • Tasks  │   - Onboarding steps    │  AI chat     │
│ • Epics  │   - Generated content   │              │
│          │                         │              │
└──────────┴─────────────────────────┴──────────────┘
     ↑              ↑                       ↑
  Collapsible   View-specific           Shared across
  navigation    content                 all views
```

### Core Principles

1. **Left pane = Main document** - Shows the primary content (spec details, interview questions, scan results)
2. **Right pane = Chat** - Contextual AI assistant that can answer questions and take actions
3. **Sidebar = Navigation** - Collapsible list of items (specs, tasks, epics) for quick switching
4. **Breadcrumb for flows** - Guided processes (onboarding) show progress in header instead of sidebar

## Why This Approach

### Chosen: Unified Shell with View Adapters

Each view implements a `ViewAdapter` interface providing:
- `SidebarItems()` - List of navigable items for the sidebar
- `MainContent()` - The document pane content
- `ChatContext()` - Context for the AI assistant
- `OnChatAction()` - Handler for chat-triggered actions

**Benefits:**
- Single source of truth for layout behavior
- Consistent keyboard shortcuts across all views
- Shared chat panel instance with persistent state
- Easy to add new views that automatically get the layout

### Rejected Alternatives

- **Gradual Migration**: Risk of inconsistency, more maintenance burden
- **Layout Manager**: Over-engineered for Bubble Tea's component model

## Key Decisions

### 1. Chat State: Per-View with Context Carryover
Each view maintains its own conversation history. When switching views, the AI retains awareness of recent cross-view context (last N messages from other views).

### 2. Sidebar Behavior
- **Toggle:** `Ctrl+B` to show/hide (matches VS Code)
- **Width:** Narrow (~20 chars) when visible
- **Content:** View-specific items (specs for Gurgeh, insights for Pollard, etc.)
- **Hidden during onboarding:** Breadcrumb navigation instead

### 3. Chat Panel Purpose
- Answer questions about selected items ("explain this spec")
- Execute actions ("create a new task for this epic")
- Context-aware based on current view and selection

### 4. Main Document Pane
- Shows selected item detail (full content)
- During onboarding: interview questions, scan progress, generated content
- Scrollable with keyboard navigation (j/k or arrows)

## Views to Update

| View | Current Layout | Change Required |
|------|---------------|-----------------|
| KickoffView | Split (doc+chat) | Add sidebar (hidden during onboarding) |
| InterviewView | Split (doc+chat) | Add sidebar (hidden, use breadcrumb) |
| GurgehView | List+Detail | Add chat panel, move list to sidebar |
| PollardView | List+Detail | Add chat panel, move list to sidebar |
| ColdwineView | List+Detail | Add chat panel, move list to sidebar |
| EpicReviewView | Expandable list | Add chat panel for Q&A during review |
| TaskReviewView | Expandable list | Add chat panel for Q&A during review |
| SpecSummaryView | Full-width | Add chat panel for refinement |
| BigendView | Dual-pane | Adapt to shell pattern (sessions sidebar) |

## Open Questions

1. **Chat backend integration** - Which AI agent powers the chat? Claude via existing agent detection?
2. **Keyboard shortcuts** - Full keybinding spec needed for sidebar toggle, chat focus, etc.
3. **Mobile/narrow terminals** - Behavior when width < 100 chars? Current stacking works but sidebar unclear.
4. **Offline mode** - What happens when AI is unavailable? Disable chat input or show error?

## Success Criteria

- [ ] All views use the unified shell layout
- [ ] Chat panel is contextually aware of current view and selection
- [ ] Sidebar is collapsible and consistent across views
- [ ] Onboarding flows use breadcrumb instead of sidebar
- [ ] Keyboard navigation works consistently (j/k, Ctrl+B, etc.)
- [ ] Layout remains stable during loading/scanning states

## Next Steps

Run `/workflows:plan` to create the implementation plan with specific tasks and file changes.
