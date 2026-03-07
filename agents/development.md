# Development Setup

## Prerequisites
- Go 1.24+
- tmux (for session management)
- Node.js (for MCP TypeScript components)

## Build & Run

```bash
# Build all
go build ./cmd/...

# Unified TUI (recommended)
./dev autarch tui                    # Full onboarding flow
./dev autarch tui --skip-onboard     # Direct to dashboard
./dev autarch tui --tool=gurgeh      # Jump to specific tab
./dev autarch tui --inline           # Inline mode (preserves scrollback)

# Standalone CLI (no TUI)
./dev gurgeh list                    # List specs
./dev coldwine status                # Task status
./dev pollard scan                   # Run research

# Test
go test ./...
go test ./internal/<pkg> -v  # Specific package
```

**Note:** Standalone TUI modes (`./dev gurgeh`, `./dev coldwine`) are deprecated. Use `autarch tui --tool=X` instead.

## Configuration

**Shared agent targets** (global + per-project overrides):
- Global: `~/.config/autarch/agents.toml`
- Project: `.gurgeh/agents.toml`

```toml
[targets.claude]
command = "claude"
args = []

[targets.codex]
command = "codex"
args = []
```
