package tui

// ChatSettings controls chat panel behavior.
type ChatSettings struct {
	AutoScroll           bool
	ShowHistoryOnNewChat bool
	GroupMessages        bool
}

// DefaultChatSettings returns the default chat settings.
func DefaultChatSettings() ChatSettings {
	return ChatSettings{
		AutoScroll:           true,
		ShowHistoryOnNewChat: true,
		GroupMessages:        true,
	}
}
