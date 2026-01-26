#!/bin/bash
#
# install-hooks.sh - Install Autarch agent state hooks for Claude Code and Codex CLI
#
# This script:
# 1. Creates ~/.autarch/hooks/ directory
# 2. Copies hook scripts there
# 3. Merges hook config into Claude Code settings
# 4. Updates Codex config.toml with notify setting
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AUTARCH_HOOKS_DIR="${HOME}/.autarch/hooks"
AUTARCH_STATES_DIR="${HOME}/.autarch/agent-states"

echo "Installing Autarch agent state hooks..."

# Create directories
mkdir -p "$AUTARCH_HOOKS_DIR"
mkdir -p "$AUTARCH_STATES_DIR"

# Copy hook scripts
cp "$SCRIPT_DIR/emit-state.sh" "$AUTARCH_HOOKS_DIR/"
cp "$SCRIPT_DIR/codex-notify.sh" "$AUTARCH_HOOKS_DIR/"
chmod +x "$AUTARCH_HOOKS_DIR"/*.sh

echo "  Copied hook scripts to $AUTARCH_HOOKS_DIR"

# --- Claude Code Setup ---
CLAUDE_SETTINGS="${HOME}/.claude/settings.json"

if [[ -f "$CLAUDE_SETTINGS" ]]; then
  echo "  Found existing Claude Code settings at $CLAUDE_SETTINGS"

  # Check if hooks already configured
  if grep -q "emit-state.sh" "$CLAUDE_SETTINGS" 2>/dev/null; then
    echo "  Claude Code hooks already installed, skipping..."
  else
    echo "  To add Autarch hooks to Claude Code, merge this into $CLAUDE_SETTINGS:"
    echo ""
    echo "  Or add to your project's .claude/settings.json for project-specific hooks."
    echo ""
    cat "$SCRIPT_DIR/claude-hooks.json" | head -30
    echo "  ..."
  fi
else
  echo "  Claude Code settings not found. Creating with Autarch hooks..."
  mkdir -p "${HOME}/.claude"
  cp "$SCRIPT_DIR/claude-hooks.json" "$CLAUDE_SETTINGS"
  echo "  Created $CLAUDE_SETTINGS"
fi

# --- Codex CLI Setup ---
CODEX_CONFIG="${HOME}/.codex/config.toml"

if [[ -f "$CODEX_CONFIG" ]]; then
  echo "  Found existing Codex config at $CODEX_CONFIG"

  if grep -q "notify" "$CODEX_CONFIG" 2>/dev/null; then
    echo "  Codex notify already configured. Current setting:"
    grep "notify" "$CODEX_CONFIG" || true
    echo ""
    echo "  To use Autarch hooks, set: notify = \"$AUTARCH_HOOKS_DIR/codex-notify.sh\""
  else
    echo "  Adding notify setting to Codex config..."
    echo "" >> "$CODEX_CONFIG"
    echo "# Autarch agent state hooks" >> "$CODEX_CONFIG"
    echo "notify = \"$AUTARCH_HOOKS_DIR/codex-notify.sh\"" >> "$CODEX_CONFIG"
    echo "  Added notify setting"
  fi
else
  echo "  Codex config not found. Creating with Autarch notify..."
  mkdir -p "${HOME}/.codex"
  cat > "$CODEX_CONFIG" << EOF
# Codex CLI configuration
# See: https://developers.openai.com/codex/config-advanced/

# Autarch agent state hooks
notify = "$AUTARCH_HOOKS_DIR/codex-notify.sh"
EOF
  echo "  Created $CODEX_CONFIG"
fi

echo ""
echo "Installation complete!"
echo ""
echo "Hook scripts installed to: $AUTARCH_HOOKS_DIR"
echo "State files written to:    $AUTARCH_STATES_DIR"
echo ""
echo "State events will be emitted to:"
echo "  - Per-session JSON files: $AUTARCH_STATES_DIR/{agent}-{project}.json"
echo "  - Event log: $AUTARCH_STATES_DIR/events.log"
echo "  - Bigend HTTP API: \$BIGEND_URL/api/agent-state (if running)"
echo "  - Unix socket: ~/.autarch/bigend.sock (if available)"
echo ""
echo "Note: Codex CLI currently only fires 'agent-turn-complete' events."
echo "See https://github.com/openai/codex/issues/2109 for more event support."
