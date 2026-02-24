# Solutions Documentation

This directory contains solutions to specific problems encountered during Autarch development. Each solution document is self-contained but cross-referenced with others.

## Terminal state restoration (February 2026)

Complete research and implementation guide for terminal state management, signal handling, and panic recovery in Go TUI applications.

### Documents in this series

| Document | Purpose | Audience | Length |
|----------|---------|----------|--------|
| **TERMINAL_RECOVERY_SUMMARY.md** | Overview and key findings | Everyone | 6 KB |
| **TERMINAL_RECOVERY_QUICKREF.md** | One-page cheat sheet | Developers | 8 KB |
| **TERMINAL_STATE_RECOVERY.md** | Comprehensive guide | Architects/Leads | 25 KB |
| **TERMINAL_RECOVERY_EXAMPLES.go** | Code examples | Developers | 14 KB |
| **TERMINAL_TESTING_GUIDE.md** | Testing strategies | QA/Developers | 17 KB |

### Quick start

**Implementing terminal state restoration in Autarch:**

1. Read: `TERMINAL_RECOVERY_SUMMARY.md` (5 min)
2. Reference: `TERMINAL_RECOVERY_QUICKREF.md` (during coding)
3. Copy patterns from: `TERMINAL_RECOVERY_EXAMPLES.go`
4. Test using: `TERMINAL_TESTING_GUIDE.md`

### Key takeaways

**The most critical pattern (copy this):**

```go
// Signal handling
ctx, stop := signal.NotifyContext(context.Background(),
    syscall.SIGINT, syscall.SIGTERM)
defer stop()

// Raw mode
oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
if err != nil { return err }
defer term.Restore(int(os.Stdin.Fd()), oldState)

// Panic recovery
defer func() {
    if r := recover(); r != nil {
        term.Restore(int(os.Stdin.Fd()), oldState)
        fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
        os.Exit(1)
    }
}()

// TUI
p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
_, err = p.Run()
```

### Status

- **Research:** Complete ✓
- **Documentation:** Complete ✓
- **Code Examples:** Complete ✓
- **Testing Guide:** Complete ✓
- **Implementation in Autarch:** Pending

### For Autarch maintainers

Recommended implementation priority:

1. **HIGH** - Add signal context to main runners (30 min impact)
2. **HIGH** - Add panic recovery handler (20 min impact)
3. **MEDIUM** - Test on macOS/Windows (1-2 hours)
4. **MEDIUM** - Update user documentation (30 min)
5. **LOW** - Refactor to use TerminalState wrapper (1 hour)

### References

All information current as of February 2026:

- **Bubble Tea:** v2.0.0-beta+ (official docs: 82.15/100 quality score, 370+ examples)
- **golang.org/x/term:** Standard library, high reliability
- **Go Version:** 1.22+
- **Platforms:** Linux, macOS, Windows

### How to use these documents

**If you have 5 minutes:**
→ Read `TERMINAL_RECOVERY_SUMMARY.md`

**If you have 15 minutes:**
→ Read `TERMINAL_RECOVERY_QUICKREF.md`

**If you're implementing:**
→ Keep `TERMINAL_RECOVERY_QUICKREF.md` open + copy from `TERMINAL_RECOVERY_EXAMPLES.go`

**If you're debugging terminal issues:**
→ See troubleshooting section in `TERMINAL_STATE_RECOVERY.md`

**If you're testing:**
→ Follow `TERMINAL_TESTING_GUIDE.md`

**If you're writing documentation for users:**
→ See "Emergency Terminal Recovery" in `TERMINAL_STATE_RECOVERY.md`

---

## Other solutions

(Future solutions can be added here as they're completed)

---

## Document maintenance

Last verified: February 4, 2026
Next review: August 2026 (or when Bubble Tea v2.1 released)

### How to update

If any of these conditions occur:
- Bubble Tea API changes
- Go releases new standard library features
- Community discovers better patterns
- Platform-specific issues discovered

Update priority order:
1. `TERMINAL_STATE_RECOVERY.md` (primary source)
2. `TERMINAL_RECOVERY_QUICKREF.md` (quick reference)
3. `TERMINAL_RECOVERY_EXAMPLES.go` (code examples)
4. All others (cross-references)

---

**Total Documentation:** ~70 KB | ~2900 lines | 5 documents
**Estimated Implementation Time:** 1-2 hours for Autarch
**Status:** Ready for implementation
