package tui

import (
	"log/slog"
	"testing"
	"time"
)

func TestLogPane_NewLogPane(t *testing.T) {
	p := NewLogPane()
	if p == nil {
		t.Fatal("NewLogPane returned nil")
	}
	if len(p.entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(p.entries))
	}
}

func TestLogPane_BufferRotation(t *testing.T) {
	p := NewLogPane()
	p.SetSize(80, 20) // Initialize viewport

	// Add more than maxLogEntries (500) entries
	for i := 0; i < 600; i++ {
		batch := LogBatchMsg{
			Entries: []LogMsg{{
				Level:   slog.LevelInfo,
				Message: "test",
				Time:    time.Now(),
			}},
		}
		p.Update(batch)
	}

	entries := p.Entries()
	if len(entries) != maxLogEntries {
		t.Errorf("expected %d entries after rotation, got %d", maxLogEntries, len(entries))
	}
}

func TestLogPane_Update_LogBatchMsg(t *testing.T) {
	p := NewLogPane()
	p.SetSize(80, 20)

	now := time.Now()
	batch := LogBatchMsg{
		Entries: []LogMsg{
			{Level: slog.LevelInfo, Message: "msg1", Time: now},
			{Level: slog.LevelWarn, Message: "msg2", Time: now},
			{Level: slog.LevelError, Message: "msg3", Time: now},
		},
	}

	p.Update(batch)

	entries := p.Entries()
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Message != "msg1" {
		t.Errorf("expected 'msg1', got '%s'", entries[0].Message)
	}
	if entries[1].Level != slog.LevelWarn {
		t.Errorf("expected Warn level, got %v", entries[1].Level)
	}
}

func TestLogPane_FormatEntry(t *testing.T) {
	p := NewLogPane()
	p.SetSize(80, 20)

	testCases := []struct {
		level    slog.Level
		expected string // level abbreviation
	}{
		{slog.LevelDebug, "DBG"},
		{slog.LevelInfo, "INF"},
		{slog.LevelWarn, "WRN"},
		{slog.LevelError, "ERR"},
	}

	for _, tc := range testCases {
		entry := LogMsg{
			Level:   tc.level,
			Message: "test",
			Time:    time.Date(2026, 1, 15, 14, 30, 45, 0, time.UTC),
		}

		formatted := p.formatEntry(entry)

		// Check that timestamp is present
		if len(formatted) < 8 {
			t.Errorf("formatted entry too short: %s", formatted)
			continue
		}

		// The format is "HH:MM:SS LEVEL MESSAGE"
		// We can check that it contains the expected parts
		if formatted[:8] != "14:30:45" {
			t.Errorf("expected timestamp '14:30:45', got '%s'", formatted[:8])
		}
	}
}

func TestLogPane_View(t *testing.T) {
	p := NewLogPane()
	p.SetSize(80, 20)

	// Add some entries
	batch := LogBatchMsg{
		Entries: []LogMsg{
			{Level: slog.LevelInfo, Message: "hello", Time: time.Now()},
		},
	}
	p.Update(batch)

	view := p.View()
	if len(view) == 0 {
		t.Error("View returned empty string")
	}

	// Check that "Logs" header is present
	if !containsString(view, "Logs") {
		t.Error("View should contain 'Logs' header")
	}
}

func TestLogPane_SetSize(t *testing.T) {
	p := NewLogPane()

	// Initial state - no viewport set
	if p.width != 0 || p.height != 0 {
		t.Error("initial size should be 0")
	}

	p.SetSize(100, 25)

	if p.width != 100 {
		t.Errorf("expected width 100, got %d", p.width)
	}
	if p.height != 25 {
		t.Errorf("expected height 25, got %d", p.height)
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
