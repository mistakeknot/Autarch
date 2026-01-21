package tui

// StatusIndicator returns a styled status indicator string
func StatusIndicator(status string) string {
	switch status {
	case "running":
		return StatusRunning.Render("● RUNNING")
	case "waiting":
		return StatusWaiting.Render("○ WAITING")
	case "idle":
		return StatusIdle.Render("◌ IDLE")
	case "error":
		return StatusError.Render("✗ ERROR")
	case "done", "completed":
		return StatusRunning.Render("✓ DONE")
	case "in_progress", "in-progress":
		return StatusWaiting.Render("◐ IN PROGRESS")
	case "todo", "pending":
		return StatusIdle.Render("○ TODO")
	default:
		return StatusIdle.Render("? UNKNOWN")
	}
}

// AgentBadge returns a styled badge for an agent type
func AgentBadge(agentType string) string {
	switch agentType {
	case "claude", "claude-code":
		return BadgeClaudeStyle.Render("Claude")
	case "codex", "codex-cli":
		return BadgeCodexStyle.Render("Codex")
	case "aider":
		return BadgeAiderStyle.Render("Aider")
	case "cursor":
		return BadgeCursorStyle.Render("Cursor")
	default:
		return BadgeStyle.Render(agentType)
	}
}

// PriorityBadge returns a styled badge for task priority
func PriorityBadge(priority int) string {
	switch priority {
	case 0:
		return StatusError.Render("P0")
	case 1:
		return StatusWaiting.Render("P1")
	case 2:
		return LabelStyle.Render("P2")
	default:
		return LabelStyle.Render("P3+")
	}
}
