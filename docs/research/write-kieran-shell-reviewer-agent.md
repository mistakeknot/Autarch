# Analysis: Writing kieran-shell-reviewer Agent

## Summary

Created a Claude Code plugin agent file for shell script review, following the established pattern from the compound-engineering plugin's `kieran-python-reviewer.md` and `kieran-typescript-reviewer.md`.

## Reference Pattern Analysis

The existing Kieran reviewer agents (Python at `/root/.claude/plugins/cache/every-marketplace/compound-engineering/2.30.0/agents/review/kieran-python-reviewer.md` and TypeScript variant) share a consistent structure:

1. **YAML frontmatter**: `name`, `description` (with embedded XML examples), `model: inherit`
2. **Persona intro**: "You are Kieran, a super senior [role]..."
3. **Numbered review principles**: 10-11 sections covering the language-specific quality bar
4. **FAIL/PASS examples**: Concrete code snippets showing what to reject vs accept
5. **Review workflow**: Ordered checklist at the end
6. **Closing philosophy**: Memorable one-liner tying it together

## Shell Reviewer Design Decisions

The 10 review sections were chosen to address the most common categories of shell script defects:

1. **Safety First** (set -euo pipefail, trap, guarded rm) -- the single most impactful improvement for any shell script
2. **Quoting** -- the #1 source of real-world shell bugs; missing quotes cause word splitting and globbing
3. **Error Handling** -- check exit codes, meaningful stderr messages, die() helper pattern
4. **Portability** -- shebang/syntax mismatch detection, GNU vs BSD differences, POSIX awareness
5. **Injection Risks** -- eval prohibition, mktemp for temp files, safe command construction via arrays
6. **Naming** -- UPPER_SNAKE for exports, lower_snake for locals, meaningful function names
7. **Functions** -- local variables, return vs exit, avoid unnecessary subshells
8. **Performance** -- builtins over external commands, useless cat, subshells in loops, IFS-based parsing
9. **Logging & Output** -- stderr vs stdout discipline, consistent prefixes, --verbose support
10. **Core Philosophy** -- 100-line rule, ShellCheck mandatory, idempotency, comments explain WHY

## Key Differences from Python/TypeScript Reviewers

- **Safety-first ordering**: Shell's biggest risks are runtime failures (unset vars, failed commands), so safety and quoting come before style concerns
- **Portability section**: Unique to shell -- no equivalent concern in Python/TS
- **Injection risks**: Elevated to its own section because shell's eval/expansion model makes it uniquely vulnerable
- **Performance framing**: Focused on fork/subshell overhead rather than algorithmic complexity
- **100-line philosophy**: Shell scripts should stay small; complex logic belongs in a real language
- **ShellCheck as mandatory**: The equivalent of a linter rule -- non-negotiable baseline

## Target File

`/root/projects/Clavain/agents/review/kieran-shell-reviewer.md`

This sits alongside 12 existing agent files in that directory, including the Python and TypeScript Kieran variants that were copied from the compound-engineering plugin.
