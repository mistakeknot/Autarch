# Claude Code Ecosystem — Autarch Integration

Extracted from AGENTS.md. Covers hooks, plugins, MCP servers, and agent coordination patterns specific to Autarch development.

## Hook System

- **`updatedInput` is REPLACE not merge** — must pass ALL original tool_input fields
- **Bug #15897**: Multiple PreToolUse hooks cause `updatedInput` to be silently dropped (last hook wins)
- **Conflicting plugins**: hookify + tool-time both have catch-all PreToolUse hooks that nuke `updatedInput`
- **Subagents don't inherit CLAUDE.md** — instructions in parent CLAUDE.md don't reach Task subagents
- **Explore/Plan agents are read-only** — no Write tool; only general-purpose and specialized reviewers can write files
- hookify disabled (`false` in settings.json) — it has no rules and its catch-all PreToolUse nukes `updatedInput`
- **Hooks are cached at session start** — changes to settings.json/hooks.json don't take effect until next session
- **hook.sh IS re-read from disk each invocation** — only the hooks.json registration is cached, so editing hook.sh works mid-session

### `updatedInput` Semantics

- **REPLACE, not merge**: `updatedInput` completely replaces `tool_input`. Must include ALL fields.
- For Task tool: must include `prompt`, `description`, `subagent_type`, and optionally `model`, `max_turns`, `run_in_background`, `resume`
- Best pattern: capture full `ORIGINAL_INPUT=$(echo "$INPUT" | jq '.tool_input')` then `echo "$ORIGINAL_INPUT" | jq --arg prompt "$NEW_PROMPT" '. + {"prompt": $prompt}'`

### Multi-Hook Bug (#15897)

- When multiple PreToolUse hooks match the same tool, `updatedInput` from earlier hooks is overwritten by later hooks
- Even a hook returning `{}` (no modification) will silently nuke a previous hook's `updatedInput`
- **Affected plugins on this server**:
  - `hookify` — catch-all PreToolUse (no matcher), returns `{}` for non-Bash/Edit tools
  - `tool-time` — `matcher: "*"`, logs events, returns nothing to stdout
  - `security-guidance` — `matcher: "Edit|Write|MultiEdit"`, only fires for file tools (not Task)

## Plugin Version Resolution

- **Version selection is unpredictable**: CC may load an older cached version instead of latest or local
- **hook.sh is re-read from disk each invocation** — editing cached hook.sh takes effect mid-session
- **settings.json / hooks.json changes are cached at session start** — require restart to take effect
- **Workaround for cached versions**: sync hook.sh to ALL cached versions when making changes
- **Diagnostic trick**: Add `echo "$0 $CLAUDE_PLUGIN_ROOT" >> /tmp/hook-identity.log` to identify which version is running
- **`localPlugins` is undocumented and unreliable** — the CORRECT way is marketplace install: add to `marketplace.json`, update, install with `name@marketplace` format

## Interclode Plugin (Cross-AI Delegation)

- **Plugin**: `interclode@interagency-marketplace` — dispatch Codex agents from Claude Code
- **Components**: `/interclode` command + `delegate` skill + `dispatch.sh` script
- **Codex CLI**: `codex exec -s workspace-write -C <dir> -o <output> "prompt"` with `run_in_background: true`
- `--inject-docs` auto-prepends CLAUDE.md/AGENTS.md; Codex reads AGENTS.md natively from `-C` dir
- **GOCACHE fix**: Codex agents hit permission errors on `/root/.cache/go-build`; add `GOCACHE=/tmp/go-build-cache` to prompts
- **Always verify independently**: Codex agents can report success while tests actually fail
- **Scope test commands**: Always use `-run TestPattern` or `-short` to avoid hanging integration tests

## MCP Agent Mail

- **Server**: systemd service `mcp-agent-mail.service`, `http://127.0.0.1:8765/mcp/`, SQLite backend
- **MCP configured twice**: global `settings.json` (with auth token) AND Clavain `plugin.json` (no token)
- **Hook gracefully degrades**: 2s timeout on health check, silent exit 0 if unreachable
- **All concurrent sessions must register** — the hook solves the bootstrap problem
- **File reservations are advisory** — they don't block edits, but report conflicts

## Flux Drive Skill

- Invoked as `/clavain:flux-drive` (moved from gurgeh-plugin to Clavain — project-agnostic)
- Accepts file (plan, brainstorm, spec, ADR, README) or bare directory (repo review mode)
- **Always check codebase reality before profiling** — documents diverge from implementation
- **Front-load divergence context in agent prompts** — list actual file paths + line numbers
- **Convergence tracking** — when N/M agents flag the same issue, that's high confidence signal
- **fd-architecture is the highest-value single agent** for "what to adopt" reviews

## Agent Type Reference

| Agent Type | Has Write | Use Case |
|------------|-----------|----------|
| `Task general-purpose` | Yes | Most research agents |
| `Task Explore` | No | Best-practices research |
| `Task [agent-name]` | Yes | Specialized reviewers |
