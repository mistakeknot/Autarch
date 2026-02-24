# Analysis: Writing kieran-go-reviewer Agent

## Task

Create `/root/projects/Clavain/agents/review/kieran-go-reviewer.md` following the exact pattern established by `kieran-python-reviewer.md` and `kieran-typescript-reviewer.md`.

## Source Files Analyzed

- `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/agents/review/kieran-python-reviewer.md` (105 lines)
- `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/agents/review/kieran-typescript-reviewer.md` (96 lines)
- `/root/projects/Clavain/agents/review/kieran-python-reviewer.md` (local copy, identical to cached)
- `/root/projects/Clavain/agents/review/kieran-typescript-reviewer.md` (local copy, identical to cached)

## Pattern Analysis

### Frontmatter Structure

Both files use YAML frontmatter with:
- `name`: kebab-case agent name
- `description`: Long string with role description, when to invoke, and 3 `<example>` blocks showing context/user/assistant/commentary interaction patterns
- `model: inherit`

### Body Structure

The Python version has 11 sections; the TypeScript version has 10 (it merged some). Both follow this general flow:

1. **Opening paragraph**: "You are Kieran, a super senior [language] developer with impeccable taste..."
2. **Numbered principles**: Each a `## N. TITLE` heading with bullet points
3. **FAIL/PASS examples**: Inline code examples showing bad vs. good patterns
4. **Closing review steps**: Numbered list of how to approach a review
5. **Final sentence**: "Remember: you're not just finding problems, you're teaching [language] excellence."

### Shared Sections (Present in Both)

1. EXISTING CODE MODIFICATIONS - BE VERY STRICT (identical in both)
2. NEW CODE - BE PRAGMATIC (identical in both)
3. Language-specific convention (Type Hints for Python, Type Safety for TypeScript)
4. TESTING AS QUALITY INDICATOR (identical in both)
5. CRITICAL DELETIONS & REGRESSIONS (identical in both)
6. NAMING & CLARITY - THE 5-SECOND RULE (nearly identical, examples differ by naming convention)
7. MODULE EXTRACTION SIGNALS (nearly identical)
8. Language-specific patterns/idioms
9. IMPORT ORGANIZATION (language-specific grouping rules)
10. MODERN FEATURES (language-specific)
11. CORE PHILOSOPHY (shared "Duplication > Complexity" theme, language-specific additions)

## Go-Specific Adaptations Made

### Section 3: Error Handling Convention (replaces Type Hints / Type Safety)
- Error checking obligation (never discard with `_`)
- Error wrapping with `%w` verb
- Sentinel errors pattern
- No panic in library code
- `errors.Is`/`errors.As` for comparison
- Early return pattern for happy path

### Section 4: Testing
- Table-driven tests with `t.Run` subtests
- `t.Helper()` for test helpers
- `testify/assert` or stdlib consistency

### Section 6: Naming
- Go-specific: short lowercase package names, MixedCaps exports, receiver naming, anti-stutter rules
- Package-qualified naming (e.g., `user.New` not `user.NewUser`)

### Section 8: Go Idioms (replaces Pythonic Patterns / Modern TS Patterns)
- Accept interfaces, return structs
- Embedding for composition
- Channel vs mutex guidance
- Goroutine lifecycle management
- `context.Context` first param
- Functional options pattern
- `defer` evaluation rules
- Useful zero values

### Section 9: Import Organization
- Three-group stdlib/external/internal pattern
- `goimports` formatting
- Dot import and blank import warnings

### Section 10: Modern Go Features
- Generics (Go 1.18+)
- `log/slog` structured logging
- `any` replacing `interface{}`
- Range-over-func (Go 1.23+)
- `slices` and `maps` packages

### Section 11: Core Philosophy
- Go proverbs: "A little copying is better than a little dependency"
- "Clear is better than clever"
- "The bigger the interface, the weaker the abstraction"
- "Don't just check errors, handle them gracefully"
- Happy path left-alignment

### Frontmatter Examples
Three Go-specific contexts:
1. HTTP handler with middleware (parallel to FastAPI/React component)
2. CLI tool with cobra commands (parallel to service refactoring)
3. Worker pool with goroutines (parallel to utility creation)

## Closing Review Steps
Adapted from the Python/TypeScript versions:
1. Critical issues first (regressions, deletions, breaking changes)
2. **Go-specific**: Unhandled errors, goroutine leaks, race conditions (replaces "type hints" / "any usage")
3-6. Same structure as other versions

## Output

Written to: `/root/projects/Clavain/agents/review/kieran-go-reviewer.md` (134 lines)

Comparable to the Python version (105 lines) and TypeScript version (96 lines) — the Go version is longer because Go idioms section (section 8) covers more ground (concurrency, interfaces, context, functional options, defer) than the equivalent Python/TypeScript sections.
