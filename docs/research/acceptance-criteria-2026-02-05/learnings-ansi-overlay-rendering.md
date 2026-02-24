# Learnings: ANSI overlay rendering

**Source:** Agent result from session b817bfb0
**Task ID:** a981f47
**Date:** 2026-02-05

---

Now I have everything I need to provide a thorough analysis.

---

## Analysis: ANSI-Aware String Splicing Learning vs. Acceptance Criteria Plan

### Relevance: YES, this learning is relevant in two distinct ways

The ANSI-aware string splicing learning from `/root/projects/Autarch/docs/solutions/ui-bugs/ansi-aware-string-splicing-for-overlays.md` documents a class of TUI rendering bug where visual-width column offsets are incorrectly mixed with rune-index or byte-length string operations on styled text. This applies to the acceptance criteria plan in two ways:

---

### 1. Overlay Rendering Correctness (Direct Hit)

The acceptance criteria plan has multiple overlays that go through the `insertAt` function in `unified_app.go` (line 2086) and `app.go` (line 387):

- **Help overlay** (F1 / `/help`) -- AC-X.3 mentions "3-pane layout renders correctly" but has no criterion for overlay rendering correctness on top of styled content.
- **Command picker** (`/` slash commands) -- The picker renders via `CommandPicker.View()` in `/root/projects/Autarch/pkg/tui/command_picker.go` and is composited using the `overlay()` method which calls `insertAt`.
- **Chat settings overlay** -- Same `overlay()` path (line 1928 of `unified_app.go`).
- **Edit preview modal** (AC-1.7) -- "Accept finding, verify preview modal with editable diff" -- this will be a new overlay rendered on top of styled content.

The fix has already been applied to `insertAt` (both copies now use `ansi.Truncate` and `ansi.TruncateLeft`), but there is **no regression test** for this. The learning document explicitly notes under "Testing": only "visual inspection with `./dev gurgeh` + F1" is listed. If someone introduces a new overlay rendering path or refactors `insertAt`, the bug can silently return.

**Specific plan gap:** The plan has no acceptance criterion verifying that overlays render correctly when positioned on top of ANSI-styled base content. AC-X.3 only checks "3-pane layout renders correctly at terminal width >= 120 columns" -- this is about the base layout, not overlay compositing.

**Suggested criteria additions:**

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-X.11 | Help overlay (F1) renders without garbled text when base content contains styled (bold, colored) text | Unit test: call `insertAt` with ANSI-styled base string and overlay; assert no broken escape sequences in output |
| AC-X.12 | Command picker overlay renders correctly when positioned over styled sidebar items | Unit test: render command picker over lipgloss-styled content; verify output contains no partial ANSI escape codes |
| AC-X.13 | No `[]rune` slicing combined with `lipgloss.Width` or `ansi.StringWidth` exists in rendering code paths | Grep-based CI check (detection rule from the learning doc's "Prevention" section) |

---

### 2. Emoji/Multi-Byte Character Width in Pollard Views (Related but Different Bug Class)

The Pollard view in `/root/projects/Autarch/internal/tui/views/pollard.go` uses emoji icons:

```go
func categoryIcon(category string) string {
    switch category {
    case "competitor": return "\u2694"  // (sword)
    case "market":     return "\U0001F4CA"  // (chart, 2 cells wide)
    case "user":       return "\U0001F464"  // (bust, 2 cells wide)
    ...
    }
}
```

These icons participate in two rendering operations that have width-correctness issues:

**a) Sidebar label rendering:** Icons are passed as `SidebarItem.Icon` to the sidebar component. The sidebar in `/root/projects/Autarch/pkg/tui/sidebar.go` already uses `ansi.Truncate` correctly for label clamping (the learning doc notes this at line 100-101). So the sidebar itself is safe.

**b) `wordWrap` function (lines 274-301 of `pollard.go`):** This function uses `current.Len()` (byte length) and `len(word)` (byte length) to measure against `width` (visual columns). This is the **same class of bug** as the original `insertAt` issue -- mixing byte counts with visual width:

```go
// CURRENT (pollard.go:284) -- byte length, not visual width
if current.Len()+len(word)+1 > width {
```

For ASCII text this works. But if research finding body text contains emoji (which it will -- the PRD mentions icons like `\u2694\U0001F4CA\U0001F464` appearing in research findings), `len(word)` returns 3-4 bytes for a single-cell emoji like `\u2694` (which is 1 visual column) and 4 bytes for a 2-cell emoji like `\U0001F4CA`. The `current.Len()` accumulates byte lengths. This means lines will wrap too early when content contains emoji or non-ASCII characters.

This is not catastrophic (lines are shorter than expected, not garbled), but it is the same fundamental confusion between byte/rune counts and visual width that the learning document warns about.

**Specific plan gap:** AC-1.5 mentions "Pollard 3-pane layout renders: sidebar (Inbox/Accepted/Rejected/Deferred + hunter status), doc pane (finding detail)..." but has no criterion for correct rendering of findings that contain emoji icons or non-ASCII characters. The icons `\u2694\U0001F4CA\U0001F464` are specified in the PRD itself (CUJ-1) as part of the research findings display.

**Suggested criteria additions:**

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-1.5a | Research findings with category icons render with correct column alignment in sidebar and doc pane | Visual test: verify findings with all icon types display without column misalignment |
| AC-1.5b | Word-wrapped research finding body text containing emoji/non-ASCII characters wraps at correct visual column width | Unit test: call `wordWrap` with text containing 2-cell-wide emoji, verify wrap point matches visual width not byte length |

---

### 3. Broader Pattern: Future Overlay Work (Phase 3 Signals Overlay)

The project memory in `/root/.claude/projects/-root-projects-Autarch/memory/MEMORY.md` notes that Phase 3 of the navigation plan adds a **Signals overlay**. Any new overlay implementation will go through the same `overlay()` / `insertAt` compositing path. Without regression tests, the ANSI splicing bug could recur if someone writes a new overlay positioning function rather than reusing the existing corrected one.

**Suggested criteria addition:**

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-X.14 | All overlay rendering paths use `ansi.Truncate`/`ansi.TruncateLeft` for visual-column string operations; no `[]rune` slicing on styled strings | Code review gate + automated grep in CI (`[]rune` + visual-width function in same file = flag) |

---

### Summary of Recommended Additions

The ANSI-aware string splicing learning is relevant to the acceptance criteria plan in three areas:

1. **Overlay compositing regression** -- The fix exists but has zero automated tests. The plan defines overlays (help, command picker, edit preview) without any rendering correctness criteria. Add AC-X.11 through AC-X.13.

2. **Emoji width in Pollard content** -- The `wordWrap` function in `pollard.go` uses byte length instead of visual width. This is the same bug class (mixing measurement systems) applied to multi-byte characters instead of ANSI escapes. Add AC-1.5a and AC-1.5b.

3. **Future-proofing for Signals overlay** -- Phase 3 adds another overlay. A CI-level detection rule (the learning doc's grep pattern: `[]rune` near `lipgloss.Width`/`ansi.StringWidth`) would catch regressions before they ship. Add AC-X.14.