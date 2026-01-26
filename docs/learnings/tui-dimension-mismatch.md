# TUI Dimension Mismatch: Parent Padding vs Child Sizing

## Problem

Visual artifacts in TUI split layouts: stray vertical bars (`│`), misaligned borders, and content overflow.

## Root Cause

**Dimension mismatch between parent container and child views.**

The `unified_app.View()` wraps view content with padding:
```go
contentStyle := lipgloss.NewStyle().
    Padding(1, 3).  // 1 line top/bottom, 3 columns left/right
    Width(a.width).
    Height(contentHeight)
```

But views received raw terminal dimensions via `tea.WindowSizeMsg`:
```go
case tea.WindowSizeMsg:
    v.width = msg.Width      // Full terminal width!
    v.height = msg.Height - 4
```

When a `SplitLayout` padded content to the "full" width, lines became longer than the padded container could display, causing visual overflow.

## Solution

Account for parent container padding in child views:
```go
case tea.WindowSizeMsg:
    // Account for unified_app's content padding (Padding(1, 3) = 6 horizontal, 2 vertical)
    v.width = msg.Width - 6
    v.height = msg.Height - 4 - 2
```

## Related Issue: ANSI Width Calculation

When padding/truncating styled text, use ANSI-aware width functions:

**Wrong** (counts escape codes as visible):
```go
import "github.com/mattn/go-runewidth"
width := runewidth.StringWidth(styledLine)  // Overcounts!
```

**Correct** (ignores escape codes):
```go
import "github.com/charmbracelet/x/ansi"
width := ansi.StringWidth(styledLine)  // Accurate
```

## Files Affected

- `internal/tui/views/kickoff.go` - Added padding compensation
- `internal/tui/views/interview.go` - Added padding compensation
- `pkg/tui/splitlayout.go` - Uses `ansi.StringWidth()` for accurate ANSI handling

## Prevention

1. When wrapping content with padding/margins, communicate actual available dimensions to children
2. Use `charmbracelet/x/ansi` for any width calculations on styled text
3. Test TUI layouts at multiple terminal sizes, especially narrow widths

## Tags

`tui`, `lipgloss`, `bubble-tea`, `layout`, `visual-bug`
