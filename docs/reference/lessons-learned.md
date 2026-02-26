# Lessons Learned — Autarch Development

Extracted from AGENTS.md. Accumulated development lessons across TUI, Bubble Tea, Go patterns, and agent coordination.

## TUI / Bubble Tea

- **Ctrl+number keybindings don't work in Bubble Tea v1**: BT v1 doesn't negotiate the Kitty keyboard protocol, so terminals send bare digits for Ctrl+1-9. Use Alt+number instead.
- **lipgloss `Height()` is a floor, not a ceiling**: If content + padding exceeds `Height(n)`, the block silently expands. Always verify with `strings.Count(rendered, "\n")+1`.
- **Always test lipgloss layout math empirically**: Write a quick `go run` script that counts newlines. Don't trust mental arithmetic with Height/Padding interaction.
- **View height math must match unified_app's content padding**: Views using ShellLayout+SplitLayout MUST use `msg.Height - 4 - 2` (not just `-4`). The `-2` accounts for `contentStyle.Padding(1,3)` in unified_app.go.
- Always check for keybinding conflicts before proposing new shortcuts — grep for the key combo first
- Always check slash command alias collisions against `GlobalCommands()` in `pkg/tui/command_picker.go`
- Estimate refactors by counting entangled state (struct fields, message types, handlers), not just "files to change"

## Go Patterns

- **`exec.CommandContext` kills process on context deadline**: Never pass a timeout context to `exec.CommandContext` for long-running servers — use `exec.Command` instead and manage lifecycle explicitly. The timeout context is fine for health-check polls.
- **`pipefail` + `grep -q`** = SIGPIPE trap. `grep -q` exits early → upstream `tail` gets SIGPIPE. Fix: use `grep >/dev/null 2>&1`
- **JSONL transcript lines are 10-100KB each** — use byte-based `tail -c` not line-based `tail -n`

## Agent Coordination

- **Cross-agent convergence = high confidence**: When N/3 independent flux-drive agents flag the same finding, confidence scales with N. Single-agent findings should be labeled as such.
- **Stale background agents from previous sessions still complete**: TaskOutput shows "running" for agents from ended sessions, but they continue. Don't re-launch duplicates.
- **Subagent output paths are relative to CWD, not input file**: Cross-project reviews write to the wrong project. Derive OUTPUT_DIR from PROJECT_ROOT (nearest .git ancestor).
- **Background agents overwrite same-named files silently**: Name output files with plan identifier or timestamp.

## Codex Agents

- **Codex agents commit+push despite "Do NOT commit" in prompt**: With `sandbox_mode=danger-full-access` and `approval_policy=never`, Codex ignores negative constraints about git ops. Always `git status` after dispatch.
- **Codex agents make unrelated cosmetic changes**: Always `git diff --stat` before committing, revert unrelated files.
- **Scope Codex test commands to avoid hangs**: Always use `-run TestPattern` or `-short`. Codex agents stuck polling for hung tests eat the full timeout.
- **Arbiter phase tests are all integration tests**: Every test in `orchestrator_phase_test.go` calls `Advance()` → `GeneratePhase()` → `runClaude()`. Only confidence tests are true unit tests.

## Testing

- **Pre-existing test failures**: `docs/solutions` build failure (type assertion) and `TestCommandErrorWrapping` in coldwine CLI — not related to TUI changes
