---
module: integration
date: 2026-02-01
problem_type: ui_bug
component: tui/overlay
symptoms:
  - "Help overlay (F1) shows garbled text with fragments like |m*"
  - "Overlay backgrounds bleed through with broken escape codes"
  - "ANSI color codes visible as literal characters in overlay"
root_cause: "insertAt sliced styled text by rune index using visual column offset, cutting through ANSI escape sequences"
severity: medium
tags: [tui, ansi, lipgloss, overlay, charmbracelet, string-width]
---

# ANSI-Aware String Splicing for TUI Overlays

## Problem Statement

The F1 help overlay displayed garbled text. Underlying styled content bled through with fragments like `│m•` where ANSI escape sequences were being cut mid-sequence.

## Investigation

The `insertAt` function spliced an overlay string into a base string at a given visual column. It used `lipgloss.Width()` to compute the overlay width (which ignores ANSI escapes), but then sliced the base string with `[]rune` indexing (which counts ANSI escape characters as regular runes).

## Root Cause

**Visual width != rune count** when ANSI escape codes are present.

`lipgloss.Width("abc")` returns 3, but `len([]rune("\x1b[1mabc\x1b[0m"))` returns 11 (8 runes of escape codes + 3 visible). Slicing `baseRunes[:col]` at visual column 15 might actually cut at rune 8 in the middle of `\x1b[38;2;...m`, producing broken output.

## Solution

```go
// BEFORE (incorrect): rune-based slicing with visual column
func insertAt(base string, col int, overlay string) string {
    baseRunes := []rune(base)
    // ... pads with spaces ...
    overlayWidth := lipgloss.Width(overlay)
    result.WriteString(string(baseRunes[:col]))     // BREAKS ANSI
    result.WriteString(overlay)
    result.WriteString(string(baseRunes[end:]))      // BREAKS ANSI
}

// AFTER (correct): ANSI-aware truncation
import "github.com/charmbracelet/x/ansi"

func insertAt(base string, col int, overlay string) string {
    baseWidth := ansi.StringWidth(base)
    overlayWidth := lipgloss.Width(overlay)
    var result strings.Builder

    if col > 0 {
        if baseWidth >= col {
            result.WriteString(ansi.Truncate(base, col, ""))
        } else {
            result.WriteString(base)
            for i := baseWidth; i < col; i++ {
                result.WriteByte(' ')
            }
        }
    }
    result.WriteString(overlay)

    end := col + overlayWidth
    if baseWidth > end {
        result.WriteString(ansi.TruncateLeft(base, end, ""))
    }
    return result.String()
}
```

## Files Changed

- `internal/tui/unified_app.go` — `insertAt` free function
- `internal/tui/app.go` — `(*App).insertAt` method (same pattern)

## Prevention

### Detection - Catch Early
- Any time you position styled text at a visual column, you need ANSI-aware operations
- Grep for `[]rune` combined with `lipgloss.Width` or `ansi.StringWidth` — that's always a bug

### Best Practices
- **Never use `[]rune` slicing on ANSI-styled strings** for visual-column operations
- Use `ansi.Truncate(s, width, "")` for left portion (keeps first N visible chars)
- Use `ansi.TruncateLeft(s, n, "")` for right portion (drops first N visible chars)
- Use `ansi.StringWidth(s)` instead of `len([]rune(s))` for visual width
- The `charmbracelet/x/ansi` package is already a dependency via lipgloss

### Testing
- Test overlay rendering with styled base content (bold, colored text)
- Visual inspection with `./dev gurgeh` + F1

## Key Insight

In Bubble Tea / lipgloss TUI code, **never mix visual-width offsets with rune-index slicing**. The `charmbracelet/x/ansi` package provides `Truncate`, `TruncateLeft`, and `Cut` that handle escape sequences correctly.

## Related

- `pkg/tui/splitlayout.go:181` — correctly uses `ansi.Truncate` for line clamping
- `pkg/tui/sidebar.go:165` — correctly uses `ansi.Truncate` for label clamping
