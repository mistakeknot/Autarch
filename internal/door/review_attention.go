package door

import (
	tea "github.com/charmbracelet/bubbletea"
	"time"
)

func attentionTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return reviewAttentionTick{} })
}
