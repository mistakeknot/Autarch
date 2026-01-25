package tui

// StatusIndicator returns a styled status indicator string with consistent symbols.
// Status symbols:
//   ● running/working - actively executing
//   ○ waiting/todo/pending/assigned - ready but not started
//   ◐ in_progress - work in progress
//   ◌ idle/draft - inactive
//   ✓ done/completed - successfully finished
//   ✗ error/blocked/failed - problem state
func StatusIndicator(status string) string {
	switch status {
	// Active/running states
	case "running", "working":
		return StatusRunning.Render("● RUNNING")

	// Waiting states
	case "waiting":
		return StatusWaiting.Render("○ WAITING")
	case "paused":
		return StatusWaiting.Render("○ PAUSED")

	// Idle/inactive states
	case "idle":
		return StatusIdle.Render("◌ IDLE")
	case "draft":
		return StatusIdle.Render("◌ DRAFT")

	// Error/blocked states
	case "error":
		return StatusError.Render("✗ ERROR")
	case "blocked":
		return StatusError.Render("✗ BLOCKED")
	case "failed":
		return StatusError.Render("✗ FAILED")
	case "stopped":
		return StatusError.Render("✗ STOPPED")

	// Done/completed states
	case "done", "completed":
		return StatusRunning.Render("✓ DONE")
	case "closed":
		return StatusIdle.Render("✓ CLOSED")

	// In progress states
	case "in_progress", "in-progress":
		return StatusWaiting.Render("◐ IN PROGRESS")
	case "active":
		return StatusWaiting.Render("◐ ACTIVE")

	// Todo/pending states
	case "todo", "pending":
		return StatusIdle.Render("○ TODO")
	case "assigned":
		return StatusWaiting.Render("○ ASSIGNED")
	case "open":
		return StatusIdle.Render("○ OPEN")

	// Review states
	case "review":
		return StatusWaiting.Render("◐ REVIEW")

	default:
		return StatusIdle.Render("? UNKNOWN")
	}
}

// StatusSymbol returns just the symbol for a status (no text)
func StatusSymbol(status string) string {
	switch status {
	case "running", "working":
		return StatusRunning.Render("●")
	case "waiting", "paused", "assigned":
		return StatusWaiting.Render("○")
	case "idle", "draft":
		return StatusIdle.Render("◌")
	case "error", "blocked", "failed", "stopped":
		return StatusError.Render("✗")
	case "done", "completed", "closed":
		return StatusRunning.Render("✓")
	case "in_progress", "in-progress", "active", "review":
		return StatusWaiting.Render("◐")
	case "todo", "pending", "open":
		return StatusIdle.Render("○")
	default:
		return StatusIdle.Render("?")
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
