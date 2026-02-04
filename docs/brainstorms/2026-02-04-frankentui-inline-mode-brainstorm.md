# Brainstorm: FrankenTUI-Inspired Inline Mode for Autarch TUIs

**Date:** 2026-02-04
**Status:** Ready for Planning
**Origin:** Deep investigation of [FrankenTUI](https://github.com/Dicklesworthstone/frankentui)

## What We're Building

A **message-based log routing system** for Autarch's Bubble Tea TUIs that provides:

1. **Real-time log visibility** - Logs appear in a dedicated pane while TUI runs
2. **Scrollback preservation** - Terminal history survives after TUI exits
3. **One-writer rule** - Centralized terminal output prevents corruption from concurrent writes

### Scope

| In Scope | Out of Scope |
|----------|--------------|
| Custom slog.Handler emitting LogMsg | Bayesian diff strategy selection |
| LogPane component (viewport-based) | BOCPD resize coalescing |
| TerminalWriter (mutex-protected) | True inline mode (scroll-above-UI) |
| `--inline` flag for opt-in | Changing default behavior |
| Panic recovery with terminal reset | Full FrankenTUI port |

## Why This Approach

### Primary Use Cases (Both Equally Important)
1. **Agent workflow visibility** - See Coldwine/Gurgeh agent output while TUI runs
2. **Debugging** - Capture logs during development; preserve after exit

### Why Message-Based (Approach A)?

We evaluated three approaches:

| Approach | Complexity | Integration | Real-time | Scrollback |
|----------|------------|-------------|-----------|------------|
| **A: Message-Based** | Medium | Native | Yes | Yes |
| B: io.MultiWriter | Low | Hacky | No | Yes |
| C: True Inline | High | Custom | Yes | Yes |

**Approach A wins because:**
- Follows established Autarch patterns (SprintStreamLineMsg, TerminalPane)
- Non-blocking: logs buffer in channel, UI renders at own pace
- Easy to test: mock handler, assert messages
- One-writer rule fits naturally into message architecture

### Why Not True Inline Mode?

FrankenTUI's scroll-above-UI is elegant but requires:
- Abandoning `tea.WithAltScreen()` entirely
- Complex cursor management (DECSTBM, save/restore)
- Terminal compatibility testing (tmux, screen, etc.)

We can add this as a future enhancement if needed.

## Key Decisions

### 1. Opt-in via `--inline` flag
- Safer rollout; users choose when they want the feature
- Default behavior unchanged (alt-screen mode)
- Per-tool: `gurgeh --inline`, `coldwine execute --inline`

### 2. Dedicated log pane (not scroll-above-UI)
- Split layout: main UI + scrollable log viewport
- Reuses existing TerminalPane viewport pattern
- Takes screen real estate but simpler to implement

### 3. Full one-writer rule implementation
- TerminalWriter serializes all stdout through mutex
- LogSink routes logs through writer
- Prevents concurrent write corruption (41 files with log calls!)

### 4. Skip Bayesian algorithms (YAGNI)
- FrankenTUI's diff strategy selection is impressive but Bubble Tea handles diffing
- Resize coalescing: Bubble Tea's handling is adequate
- Document concepts for future reference if perf issues arise

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Bubble Tea Program                    │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐ │
│  │  Main View  │    │   LogPane   │    │   Footer    │ │
│  │  (Gurgeh/   │    │  (viewport) │    │   (help)    │ │
│  │  Coldwine)  │    │             │    │             │ │
│  └─────────────┘    └──────▲──────┘    └─────────────┘ │
│                            │                            │
│                       LogMsg{...}                       │
│                            │                            │
├────────────────────────────┼────────────────────────────┤
│                   slog.Handler                          │
│                   (TUIHandler)                          │
│                            │                            │
│              ┌─────────────┴─────────────┐              │
│              │      TerminalWriter       │              │
│              │    (mutex + channel)      │              │
│              └───────────────────────────┘              │
└─────────────────────────────────────────────────────────┘
```

### Data Flow

1. Code calls `slog.Info("message")`
2. TUIHandler formats log, creates `LogMsg`
3. LogMsg sent to Bubble Tea program via `program.Send()`
4. App.Update() receives LogMsg, appends to LogPane buffer
5. LogPane.View() renders visible logs in viewport
6. On exit: buffer written to stdout (scrollback preserved)

## Components to Create

| Component | Location | Lines (est) | Description |
|-----------|----------|-------------|-------------|
| LogMsg | `internal/tui/messages.go` | ~10 | Message type with level, time, text |
| TUIHandler | `pkg/tui/loghandler/handler.go` | ~80 | slog.Handler implementation |
| LogPane | `pkg/tui/logpane/pane.go` | ~150 | Viewport-based log display |
| TerminalWriter | `pkg/tui/writer/writer.go` | ~100 | Mutex-protected output |
| Recovery | `pkg/tui/recovery/recovery.go` | ~30 | Panic recovery + terminal reset |

**Total: ~370 lines of new code**

## Open Questions

1. **Log buffer size** - How many entries before rotation? (Suggest: 1000)
2. **Log level filtering** - UI toggle for debug/info/warn/error?
3. **Color scheme** - Match Tokyo Night? Level-based colors?
4. **Pane toggle** - Hotkey to show/hide log pane? (Suggest: `L`)

## Success Criteria

- [ ] `gurgeh --inline` shows logs in dedicated pane
- [ ] Scrollback visible after exit (grep-able)
- [ ] No log corruption during heavy async operations
- [ ] Panic leaves terminal in usable state
- [ ] Existing behavior unchanged without `--inline`

## References

- [FrankenTUI Repository](https://github.com/Dicklesworthstone/frankentui)
- [Bubble Tea Framework](https://github.com/charmbracelet/bubbletea)
- Autarch patterns: `internal/tui/messages.go`, `internal/bigend/tui/terminal.go`
- Research docs: `docs/solutions/INLINE_MODE_*.md`

## Next Steps

Run `/workflows:plan` to create implementation plan with specific tasks.
