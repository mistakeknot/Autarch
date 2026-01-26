#!/bin/bash
#
# emit-state.sh - Emit agent state events to Autarch/Bigend
#
# Usage: emit-state.sh <state> [agent_type]
#
# States: working, waiting, blocked, executing_tool, error, done
#
# This script is called by Claude Code and Codex CLI hooks to report
# agent state changes. It writes to multiple destinations for redundancy.

set -euo pipefail

STATE="${1:-unknown}"
AGENT_TYPE="${2:-claude}"
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Get session info from environment or stdin
SESSION_ID="${CLAUDE_SESSION_ID:-${CODEX_SESSION_ID:-$(cat /dev/stdin 2>/dev/null | jq -r '.session_id // "unknown"' 2>/dev/null || echo "unknown")}}"
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-${CODEX_PROJECT_DIR:-$(pwd)}}"
PROJECT_NAME=$(basename "$PROJECT_DIR")

# State event file location (Bigend watches this)
STATE_DIR="${HOME}/.autarch/agent-states"
mkdir -p "$STATE_DIR"

# Create state event JSON
EVENT=$(cat <<EOF
{
  "state": "$STATE",
  "agent_type": "$AGENT_TYPE",
  "session_id": "$SESSION_ID",
  "project_dir": "$PROJECT_DIR",
  "project_name": "$PROJECT_NAME",
  "timestamp": "$TIMESTAMP"
}
EOF
)

# Write to per-session state file (Bigend can poll these)
SESSION_FILE="$STATE_DIR/${AGENT_TYPE}-${PROJECT_NAME}.json"
echo "$EVENT" > "$SESSION_FILE"

# Append to event log for history
LOG_FILE="$STATE_DIR/events.log"
echo "$TIMESTAMP | $STATE | $AGENT_TYPE | $PROJECT_NAME | $SESSION_ID" >> "$LOG_FILE"

# Try to notify Bigend daemon via HTTP (non-blocking)
BIGEND_URL="${BIGEND_URL:-http://localhost:8765}"
curl -s -X POST "$BIGEND_URL/api/agent-state" \
  -H "Content-Type: application/json" \
  -d "$EVENT" \
  --connect-timeout 1 \
  --max-time 2 \
  2>/dev/null || true

# Try to write to Unix socket if available (for low-latency updates)
SOCKET_PATH="${HOME}/.autarch/bigend.sock"
if [[ -S "$SOCKET_PATH" ]]; then
  echo "$EVENT" | nc -U "$SOCKET_PATH" 2>/dev/null || true
fi

exit 0
