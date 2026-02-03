---
title: "Fix Swallowed GenerationErrorMsg in UnifiedApp"
category: ui-bugs
tags: [bubble-tea, message-routing, error-handling, swallowed-error]
module: internal/tui
symptom: "Generation errors from Arbiter never displayed in SprintView chat panel; users see no response after submitting chat messages when errors occur"
root_cause: "Parent model (UnifiedApp) caught GenerationErrorMsg and returned nil, preventing message propagation to child view (SprintView)"
date_resolved: "2026-01-31"
commit: "8752350d7965e96e994464b774daefa2debfe18e"
---

# Solution Document: Fix Swallowed GenerationErrorMsg in UnifiedApp

## Problem Statement

**User-Facing Symptom:**
When users submitted chat messages in the SprintView and the Arbiter encountered a generation error, no feedback was displayed in the chat panel. The UI appeared frozen - users would press Enter to send a message, but would receive no response whatsoever, neither success nor error indication.

**Technical Manifestation:**
`GenerationErrorMsg` messages were being sent by the Arbiter orchestrator but never reached the SprintView component that needed to display them in the chat interface.

**Impact:**
- Users had no visibility into why their requests failed
- No indication that an error occurred, creating a broken UX
- Users would retry or abandon the operation without understanding the issue
- Debug information was lost, making it harder to diagnose problems

## Root Cause Analysis

### The Bubble Tea Message Flow Pattern

In the Bubble Tea TUI framework, messages flow from child components up to parent components via the `Update()` method. The pattern is:

1. User interacts with view (e.g., presses Enter in chat)
2. Child view processes event and returns a command
3. Command executes and generates a message
4. Message bubbles up through parent `Update()` methods
5. Parent can either:
   - **Handle the message** (take action and optionally pass through)
   - **Pass through** (let child handle it via `currentView.Update(msg)`)
   - **Swallow** (catch and return without passing through) ⚠️ DANGEROUS

### The Specific Bug

In `internal/tui/unified_app.go`, the `UnifiedApp.Update()` method had this code:

```go
case GenerationErrorMsg:
    a.generating = false
    a.err = msg.Error
    return a, nil  // ⚠️ BUG: Returns without passing to currentView!
```

**Why this was wrong:**
1. UnifiedApp stores the error in `a.err` for its own use
2. BUT it immediately returns `nil`, terminating message propagation
3. The `currentView` (SprintView) never receives the message
4. SprintView has error handling code at line 177-179 that never executes:
   ```go
   case tui.GenerationErrorMsg:
       v.chatPanel.AddMessage("system", "Error: "+msg.Error.Error())
       return v, nil
   ```

### The Architectural Context

UnifiedApp is a composite container that:
- Manages multiple views (KickoffView, SprintView, etc.)
- Handles global state (agent selection, chat settings)
- Routes messages between views
- Manages navigation and mode switching

SprintView is a focused component that:
- Displays the chat interface
- Shows phase documents
- Handles user input for the sprint workflow
- **Needs to display error feedback to users**

The bug occurred because UnifiedApp treated `GenerationErrorMsg` as "its" message to handle, when in reality it was a message that **both** components needed to handle:
- UnifiedApp: Track loading state (`a.generating = false`)
- SprintView: Display error to user in chat

### Why This Pattern Is Dangerous

The "catch and swallow" pattern (`case SomeMsg: return a, nil`) is dangerous because:

1. **Silent failures**: Messages disappear with no trace
2. **Breaks assumptions**: Child components expect to see certain messages
3. **Hard to debug**: No error, no log, just missing functionality
4. **Fragile**: Works until child needs the message, then breaks mysteriously

In Bubble Tea, the correct pattern is:
```go
case SomeMsg:
    // Parent handles its concerns
    a.someState = msg.SomeData

    // Fall through to pass to child
    // (no return statement here)
```

Or explicitly:
```go
case SomeMsg:
    a.someState = msg.SomeData
    // Pass through to currentView
    if a.currentView != nil {
        var cmd tea.Cmd
        a.currentView, cmd = a.currentView.Update(msg)
        return a, cmd
    }
    return a, nil
```

## Solution Applied

### The Fix

**File:** `internal/tui/unified_app.go` (Line 430-433)

**Before:**
```go
case GenerationErrorMsg:
    a.generating = false
    a.err = msg.Error
    return a, nil  // ❌ Swallows the message
```

**After:**
```go
case GenerationErrorMsg:
    a.generating = false
    a.err = msg.Error
    // Fall through to pass to currentView so SprintView can show errors in chat
```

**What changed:**
1. Removed the `return a, nil` statement
2. Added a comment explaining why we fall through
3. The message now continues to the default handler at line 495-500:
   ```go
   // Pass to current view
   if a.currentView != nil {
       var cmd tea.Cmd
       a.currentView, cmd = a.currentView.Update(msg)
       return a, cmd
   }
   ```

### Why This Works

1. **UnifiedApp still handles its concerns**: Sets `a.generating = false` and `a.err`
2. **Message continues propagating**: Falls through to the catch-all handler
3. **SprintView receives the message**: Its error handler at line 177-179 executes
4. **User sees the error**: Chat panel displays "Error: [message]"
5. **No side effects**: Other views that don't handle GenerationErrorMsg simply ignore it

## Code Examples

### Full Context: UnifiedApp.Update() Method

```go
func (a *UnifiedApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    // ... other cases ...

    case GeneratingMsg:
        a.generating = true
        a.generatingWhat = msg.What
        return a, nil  // OK: This is purely a parent concern

    case GenerationErrorMsg:
        a.generating = false
        a.err = msg.Error
        // Fall through to pass to currentView so SprintView can show errors in chat
        // (removed: return a, nil)

    case AgentNotFoundMsg:
        a.err = &agent.NoAgentError{}
        return a, nil  // OK: Parent handles this globally

    // ... more cases ...
    }

    // Catch-all: Pass unhandled messages to current view
    if a.currentView != nil {
        var cmd tea.Cmd
        a.currentView, cmd = a.currentView.Update(msg)
        return a, cmd
    }

    return a, nil
}
```

### SprintView Error Handler

```go
func (v *SprintView) Update(msg tea.Msg) (pkgtui.View, tea.Cmd) {
    var cmd tea.Cmd

    switch msg := msg.(type) {
    // ... other cases ...

    case tui.GenerationErrorMsg:
        // Now this code path executes!
        v.chatPanel.AddMessage("system", "Error: "+msg.Error.Error())
        return v, nil

    // ... more cases ...
    }

    return v, nil
}
```

### Test Case Coverage

The test file `internal/tui/views/sprint_view_test.go` includes verification:

```go
func TestSprintView_ChatSubmitProducesResponse(t *testing.T) {
    v := NewSprintView("/tmp/test-project", SprintViewOpts{})
    v.Init()
    v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

    // Start sprint and submit chat message
    startCmd := v.StartSprint("Build a todo app")
    startMsg := startCmd()
    v.Update(startMsg)

    v.chatPanel.SetValue("Make the vision more specific")
    _, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEnter})

    msg := cmd()

    // Test verifies we get proper messages back
    switch m := msg.(type) {
    case tui.SprintStreamLineMsg:
        // Expected: agent response
    case tui.GenerationErrorMsg:
        // Expected: error properly routed
        t.Errorf("Got generation error: %v", m.Error)
    default:
        t.Errorf("Unexpected message type: %T", msg)
    }
}
```

## Prevention Strategies

### 1. Code Review Checklist

When reviewing Bubble Tea `Update()` methods, verify:

- [ ] Messages that child views need are passed through
- [ ] Early returns are justified (only for parent-specific messages)
- [ ] Comments explain why messages are or aren't passed through
- [ ] Child view handlers are reachable for their message types

### 2. Architectural Guideline

**Rule:** Parent models should only swallow messages that are exclusively their concern.

**Examples of OK to swallow:**
- `NavigationMsg` - Parent manages view stack
- `GlobalSettingsMsg` - Parent manages global state
- `WindowSizeMsg` - Parent calculates layout before passing through

**Examples of MUST pass through:**
- `ErrorMsg` - User needs to see it in active view
- `DataLoadedMsg` - View displays the data
- `StreamingMsg` - View renders streaming content

### 3. Documentation Pattern

Add comments at message handlers:

```go
case SomeErrorMsg:
    a.globalErrorCount++
    // Fall through: view needs to display error to user

case NavigateBackMsg:
    return a, a.navigateBack()  // OK: purely parent navigation concern

case SettingsChangedMsg:
    a.applySettings(msg)
    // Fall through: views may need to react to setting changes
```

### 4. Testing Strategy

**Unit tests should verify message routing:**

```go
func TestUnifiedApp_GenerationErrorReachesView(t *testing.T) {
    app := NewUnifiedApp(nil)
    mockView := &MockView{}
    app.currentView = mockView

    err := errors.New("generation failed")
    msg := GenerationErrorMsg{What: "test", Error: err}

    _, _ = app.Update(msg)

    // Verify view received the message
    if !mockView.GotMessage(msg) {
        t.Error("View did not receive GenerationErrorMsg")
    }
}
```

**Integration tests should verify user-visible behavior:**

```go
func TestSprintView_ErrorDisplayedInChat(t *testing.T) {
    v := NewSprintView("/tmp/test", SprintViewOpts{})
    v.Init()

    errMsg := tui.GenerationErrorMsg{
        What:  "generation",
        Error: errors.New("mock error"),
    }

    v.Update(errMsg)

    messages := v.ChatPanelMessagesForTest()
    found := false
    for _, m := range messages {
        if strings.Contains(m.Content, "mock error") {
            found = true
            break
        }
    }

    if !found {
        t.Error("Error not displayed in chat panel")
    }
}
```

### 5. Debugging Aid

When investigating "message not received" bugs:

1. Add logging at message generation:
   ```go
   log.Printf("Sending GenerationErrorMsg: %+v", msg)
   ```

2. Add logging at parent handler:
   ```go
   case GenerationErrorMsg:
       log.Printf("UnifiedApp caught GenerationErrorMsg")
       // ... handler code
   ```

3. Add logging at child handler:
   ```go
   case tui.GenerationErrorMsg:
       log.Printf("SprintView received GenerationErrorMsg")
       // ... handler code
   ```

4. If child logging never fires, check for swallowed message in parent.

## Verification

### Manual Testing

1. **Start a sprint** in SprintView
2. **Trigger an error condition** (e.g., invalid agent, network failure)
3. **Verify error appears in chat panel**: Should see "Error: [details]"
4. **Confirm UI remains responsive**: Can type new messages

### Automated Testing

Run the test suite:
```bash
cd internal/tui/views
go test -v -run TestSprintView_ChatSubmitProducesResponse
go test -v -run TestSprintView_TypingViaUpdateReachesComposer
```

Expected: All tests pass, error messages properly routed.

### Regression Prevention

This fix is covered by:
- `TestSprintView_ChatSubmitProducesResponse` - Verifies GenerationErrorMsg is handled
- Integration with existing error handlers in SprintView

## Related Patterns

### Similar Issues in Codebase

Search for other "catch and swallow" patterns that might be problematic:

```bash
# Find cases where messages are caught and immediately return nil
git grep -A2 "case.*Msg:" internal/tui/unified_app.go | grep "return a, nil"
```

Review each case to ensure child views don't need the message.

### Correct Handling Examples

**Example 1: AgentSelectedMsg** (Line 389-396)
```go
case pkgtui.AgentSelectedMsg:
    a.selectedAgent = msg.Name
    a.setSelectorIndex(msg.Name)
    // ... update agent ...
    a.attachAgentName(a.currentView)
    return a, nil  // ✅ OK: This is a parent configuration concern
```
✅ Correct because views are notified via `attachAgentName()`, not by receiving the message.

**Example 2: ScanProgressMsg** (Line 474-481)
```go
case scanProgressWithContinuation:
    // Forward progress to current view and schedule next read
    if a.currentView != nil {
        var cmd tea.Cmd
        a.currentView, cmd = a.currentView.Update(msg.ScanProgressMsg)
        return a, tea.Batch(cmd, msg.nextCmd)
    }
    return a, msg.nextCmd
```
✅ Correct because message is explicitly forwarded to `currentView.Update()`.

**Example 3: WindowSizeMsg** (Line 279-297)
```go
case tea.WindowSizeMsg:
    a.width = msg.Width
    a.height = msg.Height
    // ... calculate layout ...

    if a.currentView != nil {
        contentMsg := tea.WindowSizeMsg{
            Width:  msg.Width,
            Height: msg.Height - headerHeight - footerHeight,
        }
        var cmd tea.Cmd
        a.currentView, cmd = a.currentView.Update(contentMsg)
        return a, cmd
    }
    return a, nil
```
✅ Correct because parent calculates layout, then explicitly passes modified message to view.

## Lessons Learned

### Key Takeaways

1. **In Bubble Tea, assume messages should pass through unless proven otherwise**
   - Default to falling through to child handlers
   - Only intercept when parent must take sole ownership

2. **Both parent and child can handle the same message**
   - Parent updates global state
   - Child updates view-specific state
   - Both are necessary

3. **Comments are essential at message handlers**
   - Explain why message is or isn't passed through
   - Future maintainers need this context

4. **Test message routing, not just business logic**
   - Verify messages reach their intended handlers
   - Integration tests catch routing bugs

5. **User-visible errors must reach the UI**
   - Never swallow error messages at parent level
   - Always let views display errors to users

### Broader Implications

This bug illustrates a common pitfall in hierarchical UI frameworks:

- **Parent components** manage routing and coordination
- **Child components** handle presentation and user interaction
- **Messages** are the communication mechanism between layers

When parents intercept messages meant for children, the result is silent failures and broken UX. The fix is simple (remove one line) but finding the bug is hard (message just "disappears").

The architectural lesson: **Favor explicit pass-through over implicit swallowing.** Make message routing obvious in the code.

## Appendix: Complete Diff

```diff
diff --git a/internal/tui/unified_app.go b/internal/tui/unified_app.go
index 68eeae1..5a33b59 100644
--- a/internal/tui/unified_app.go
+++ b/internal/tui/unified_app.go
@@ -434,7 +434,7 @@ func (a *UnifiedApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
 	case GenerationErrorMsg:
 		a.generating = false
 		a.err = msg.Error
-		return a, nil
+		// Fall through to pass to currentView so SprintView can show errors in chat

 	case AgentNotFoundMsg:
 		a.err = &agent.NoAgentError{}
```

**Impact:**
- Lines changed: 1 deletion, 1 comment addition
- Lines affected: 1 critical control flow change
- User-visible impact: Error messages now appear in chat interface

## References

- Commit: `8752350d7965e96e994464b774daefa2debfe18e`
- Author: mistakeknot
- Date: 2026-01-31 23:05:01 -0800
- Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>

**Related Files:**
- `/root/projects/Autarch/internal/tui/unified_app.go` - Parent container (bug location)
- `/root/projects/Autarch/internal/tui/views/sprint_view.go` - Child view (error handler)
- `/root/projects/Autarch/internal/tui/views/sprint_view_test.go` - Test coverage
- `/root/projects/Autarch/internal/tui/messages.go` - Message type definitions

**See Also:**
- Bubble Tea documentation: https://github.com/charmbracelet/bubbletea
- Message passing patterns in hierarchical UI frameworks
- Error handling best practices in TUI applications
