#!/bin/bash
#
# codex-notify.sh - Codex CLI notify handler for Autarch/Bigend
#
# Called by Codex CLI's `notify` config when agent-turn-complete fires.
# The event type is passed as the first argument.
#
# Usage in ~/.codex/config.toml:
#   notify = "~/.autarch/hooks/codex-notify.sh"
#
# Codex currently only supports these events:
#   - agent-turn-complete: Agent finished a turn, waiting for input
#
# See: https://github.com/openai/codex/issues/2109 for feature request
# to add more events like PreToolUse, PermissionRequest, etc.

set -euo pipefail

EVENT="${1:-agent-turn-complete}"
EMIT_SCRIPT="${HOME}/.autarch/hooks/emit-state.sh"

case "$EVENT" in
  agent-turn-complete)
    # Agent finished processing, now waiting for user input
    "$EMIT_SCRIPT" waiting codex
    ;;
  approval-requested)
    # Codex needs permission (if this event becomes available)
    "$EMIT_SCRIPT" blocked codex
    ;;
  error)
    # Error occurred (if this event becomes available)
    "$EMIT_SCRIPT" error codex
    ;;
  *)
    # Unknown event - log it but don't fail
    echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) | unknown_event | codex | $EVENT" >> "${HOME}/.autarch/agent-states/events.log"
    ;;
esac

exit 0
