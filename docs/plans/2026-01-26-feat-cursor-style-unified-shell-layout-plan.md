---
title: "feat: Cursor-Style Unified Shell Layout"
type: feat
date: 2026-01-26
brainstorm: docs/brainstorms/2026-01-26-cursor-style-unified-layout-brainstorm.md
deepened: 2026-01-26
reviewed: 2026-01-26
---

# feat: Cursor-Style Unified Shell Layout

## Review Summary

**Reviewed on:** 2026-01-26
**Reviewers:** DHH, Kieran, Code-Simplicity
**Result:** Plan simplified from 730 LOC → ~500 LOC (32% reduction)

### Cuts Applied
- ❌ **Phase 5 (Agent-Native API)** - Removed entirely (scope creep, no user demand)
- ❌ **Phase 6 (Performance optimizations)** - Removed (ring buffer, render caching are YAGNI)
- ❌ **Responsive < 100 width** - Require minimum 100 chars, show error otherwise
- ❌ **IsHeader, Status fields** - Removed from SidebarItem (unused)
- ❌ **Multi-width testing** - Test at 120 only
- ✏️ **ChatService location** - Moved to `internal/tui/services/chat.go`
- ✏️ **Phases consolidated** - 6 phases → 2 phases

---

## Overview

Implement a unified shell layout for all Autarch TUI views that mirrors the Cursor/VS Code interaction pattern: collapsible sidebar (navigation), main document pane (2/3), and chat panel (1/3).

```
┌──────────┬─────────────────────────┬──────────────┐
│ Sidebar  │   Main Document Pane    │  Chat Panel  │
│ (toggle) │   (2/3 width)           │  (1/3 width) │
│ Ctrl+B   │                         │              │
│          │   - Selected item       │  Contextual  │
│ • Specs  │   - Onboarding steps    │  AI chat     │
│ • Tasks  │   - Generated content   │              │
│          │                         │              │
└──────────┴─────────────────────────┴──────────────┘
```

## Problem Statement

Currently, different views use inconsistent layouts:
- **InterviewView/KickoffView**: Use SplitLayout (doc + chat) ✓
- **GurgehView/PollardView/ColdwineView**: Use list + detail (no chat)
- **Review views**: Expandable lists (no chat)

This creates inconsistent user experience and no contextual AI assistance in browse/review views.

---

## Phase 1: Shell + Chat + GurgehView

Build the infrastructure and prove it works in one view.

### 1.1 Create Sidebar Component

**File:** `pkg/tui/sidebar.go` (~80 lines)

```go
// Sidebar provides collapsible navigation for the unified shell.
type Sidebar struct {
    items     []SidebarItem
    selected  int
    collapsed bool
    width     int  // Fixed 20 chars when expanded, 0 when collapsed
    height    int
    focused   bool
}

type SidebarItem struct {
    ID    string
    Label string
    Icon  string  // e.g., "●", "◐", "✓"
}

// Key methods:
func NewSidebar() *Sidebar
func (s *Sidebar) Toggle()
func (s *Sidebar) SetItems(items []SidebarItem)
func (s *Sidebar) Selected() (SidebarItem, bool)
func (s *Sidebar) IsCollapsed() bool
func (s *Sidebar) SetSize(width, height int)
func (s *Sidebar) Update(msg tea.Msg) (*Sidebar, tea.Cmd)
func (s *Sidebar) View() string
func (s *Sidebar) Focus() tea.Cmd
func (s *Sidebar) Blur()
```

**Design notes:**
- Fixed 20-char width (no responsive calculations)
- Truncate labels at 17 chars with unicode ellipsis `…`
- Flat list (no hierarchical expand/collapse)
- Focus indicator: bright border when focused, dim when not

### 1.2 Create ShellLayout Component

**File:** `pkg/tui/shelllayout.go` (~150 lines)

```go
type FocusTarget int
const (
    FocusSidebar FocusTarget = iota
    FocusDocument
    FocusChat
)

// ShellLayout provides the unified 3-pane Cursor-style layout.
type ShellLayout struct {
    sidebar     *Sidebar
    splitLayout *SplitLayout  // Reuse existing for doc + chat
    width       int
    height      int
    showSidebar bool
    focus       FocusTarget
}

func NewShellLayout() *ShellLayout {
    return &ShellLayout{
        sidebar:     NewSidebar(),
        splitLayout: NewSplitLayout(0.66),
        showSidebar: true,
        focus:       FocusDocument,
    }
}

// Dimension handling - apply the documented learning
func (l *ShellLayout) SetSize(width, height int) {
    // Require minimum 100 chars
    if width < 100 {
        l.width = 100
        l.height = height
        return  // Will show error in View()
    }

    l.width = width
    l.height = height

    sidebarW := 0
    if l.showSidebar && !l.sidebar.IsCollapsed() {
        sidebarW = 20
    }

    // Content area = width - sidebar - separator (if sidebar visible)
    contentWidth := width - sidebarW
    if sidebarW > 0 {
        contentWidth -= 1  // separator
    }

    l.sidebar.SetSize(sidebarW, height)
    l.splitLayout.SetSize(contentWidth, height)
}

// Focus cycling - simple, no helper function
func (l *ShellLayout) NextFocus() {
    switch l.focus {
    case FocusSidebar:
        l.focus = FocusDocument
    case FocusDocument:
        l.focus = FocusChat
    case FocusChat:
        if l.showSidebar && !l.sidebar.IsCollapsed() {
            l.focus = FocusSidebar
        } else {
            l.focus = FocusDocument
        }
    }
}

// Toggle sidebar with focus recovery
func (l *ShellLayout) ToggleSidebar() {
    l.sidebar.Toggle()
    if l.sidebar.IsCollapsed() && l.focus == FocusSidebar {
        l.focus = FocusDocument
    }
}
```

### 1.3 Create ChatService

**File:** `internal/tui/services/chat.go` (~40 lines)

```go
package services

// AgentInterface abstracts the AI agent for testability
type AgentInterface interface {
    Generate(ctx context.Context, prompt string) (string, error)
    Available() bool
}

type ChatService struct {
    agent AgentInterface
}

func NewChatService(agent AgentInterface) *ChatService {
    return &ChatService{agent: agent}
}

func (s *ChatService) Send(viewID, msg string) tea.Cmd {
    // Handle agent unavailable
    if s.agent == nil || !s.agent.Available() {
        return func() tea.Msg {
            return ChatResponseMsg{
                ViewID: viewID,
                Error:  errors.New("agent unavailable"),
            }
        }
    }

    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        resp, err := s.agent.Generate(ctx, msg)
        return ChatResponseMsg{
            ViewID:   viewID,
            Response: resp,
            Error:    err,
        }
    }
}

// Message types
type ChatResponseMsg struct {
    ViewID   string
    Response string
    Error    error
}

type ChatThinkingMsg struct {
    ViewID string
}

type SidebarSelectMsg struct {
    ItemID string
}
```

### 1.4 Migrate GurgehView

**File:** `internal/tui/views/gurgeh.go` (~30 lines changed)

```go
type GurgehView struct {
    // ... existing fields ...
    shell       *pkgtui.ShellLayout
    chatService *services.ChatService
}

func (v *GurgehView) View() string {
    sidebarItems := v.sidebarItems()
    return v.shell.Render(sidebarItems, v.docPanel.View(), v.chatPanel.View())
}

func (v *GurgehView) sidebarItems() []pkgtui.SidebarItem {
    var items []pkgtui.SidebarItem
    for _, spec := range v.specs {
        items = append(items, pkgtui.SidebarItem{
            ID:    spec.ID,
            Label: spec.Title,
            Icon:  statusIcon(spec.Status),
        })
    }
    return items
}

// Update() handles sidebar selection
case pkgtui.SidebarSelectMsg:
    v.selectSpec(msg.ItemID)
    return v, nil

// Compile-time interface assertion
var _ pkgtui.SidebarProvider = (*GurgehView)(nil)
```

**Optional interface (views implement if they have sidebar):**

```go
// pkg/tui/interfaces.go
type SidebarProvider interface {
    SidebarItems() []SidebarItem
}
```

---

## Phase 2: Remaining Views

Mechanical migration applying the same pattern.

### Views with Sidebar (implement SidebarProvider)

| View | Sidebar Content |
|------|-----------------|
| GurgehView | Specs list |
| PollardView | Insights list |
| ColdwineView | Epics/Tasks list |

### Views without Sidebar (chat only)

| View | Notes |
|------|-------|
| KickoffView | Onboarding, keep breadcrumb |
| InterviewView | Onboarding, keep breadcrumb |
| EpicReviewView | Chat for Q&A during review |
| TaskReviewView | Chat for Q&A during review |
| SpecSummaryView | Chat for refinement |

---

## Keyboard Shortcuts

### Shell-Level (Global)

| Key | Action |
|-----|--------|
| `Ctrl+B` | Toggle sidebar |
| `Tab` | Cycle focus: sidebar → doc → chat (skips collapsed sidebar) |

### Sidebar (when focused)

| Key | Action |
|-----|--------|
| `j/k` or `↑/↓` | Navigate items |
| `Enter` | Select item |

### Chat (when focused)

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Ctrl+J` | Newline in message |
| `Ctrl+C` | Cancel current generation |

---

## Acceptance Criteria

### Functional Requirements

- [ ] 9 views migrated: Gurgeh, Pollard, Coldwine, EpicReview, TaskReview, SpecSummary, Kickoff, Interview, TaskDetail
- [ ] Sidebar collapsible via `Ctrl+B`
- [ ] Tab cycles focus correctly (skips sidebar when collapsed)
- [ ] Browse views (Gurgeh, Pollard, Coldwine) show sidebar with items
- [ ] Onboarding/review views show chat but no sidebar

### Non-Functional Requirements

- [ ] Minimum terminal width: 100 chars (show error if smaller)
- [ ] ANSI-aware width calculations (use `ansi.StringWidth`)
- [ ] No layout flicker during loading states

### Test Cases

**Sidebar tests:**
- [ ] Toggle collapsed/expanded state
- [ ] Selection navigation (j/k moves cursor)
- [ ] Selection wraps or clamps at boundaries
- [ ] Truncation at 17 chars with ellipsis
- [ ] Empty state renders "No items yet"

**ShellLayout tests:**
- [ ] Dimension invariant: `sidebar + separator + left + separator + right == width`
- [ ] Focus cycling skips collapsed sidebar
- [ ] Focus recovery when sidebar collapses while focused
- [ ] Error shown when width < 100

**ChatService tests:**
- [ ] Agent nil returns error message
- [ ] Timeout handling (30s context)
- [ ] ChatResponseMsg contains viewID for routing

**Compile-time assertions:**
```go
var _ SidebarProvider = (*GurgehView)(nil)
var _ SidebarProvider = (*PollardView)(nil)
var _ SidebarProvider = (*ColdwineView)(nil)
```

---

## Dependencies & Risks

### Dependencies

1. **Existing SplitLayout** - Reuse for doc + chat split
2. **Agent detection** - Required for chat backend
3. **DocPanel/ChatPanel** - Already production-ready

### Risks

| Risk | Mitigation |
|------|------------|
| Breaking existing view tests | Use optional SidebarProvider interface |
| Agent unavailable | Check `Available()`, show error in chat |
| Dimension mismatch | Follow documented learning, add validation |

---

## Estimated LOC

| Component | Lines |
|-----------|-------|
| sidebar.go | ~80 |
| shelllayout.go | ~150 |
| services/chat.go | ~40 |
| View updates (9 views × ~25) | ~225 |
| **Total** | **~495** |

---

## Implementation Order

1. **Create `pkg/tui/sidebar.go`** - Can be tested in isolation
2. **Create `pkg/tui/shelllayout.go`** - Composes sidebar + splitlayout
3. **Create `internal/tui/services/chat.go`** - Simple service with timeout
4. **Migrate GurgehView** - Prove the pattern works
5. **Migrate remaining 8 views** - Mechanical application
6. **Add tests** - Per the test cases above

---

## Success Metrics

- All 9 views updated to use unified layout
- Zero visual regression in existing flows
- Chat response latency < 2s for simple queries (30s timeout)

---

## References

### Internal

- Existing SplitLayout: `pkg/tui/splitlayout.go`
- View interface: `pkg/tui/view.go`
- Padding gotcha: `docs/solutions/ui-bugs/tui-dimension-mismatch-splitlayout-20260126.md`

### Brainstorm

- Design decisions: `docs/brainstorms/2026-01-26-cursor-style-unified-layout-brainstorm.md`
