package door

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func catchupFixture() Model {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	m := NewModel([]Project{{Root: "/estate/reader", Name: "reader"}}, Ranking{}, "", nil, "", errors.New("no checker"))
	m = m.WithBriefing(BriefingOptions{Since: now.Add(-24 * time.Hour), Now: func() time.Time { return now }}).WithThreads(ThreadsOptions{})
	m.width, m.height = 80, 26
	m.movements["/estate/reader"] = Movement{Root: "/estate/reader", Name: "reader", Dirty: 12}
	m.moveRemaining = 0
	m.threads.Threads = []Thread{{Session: "iterm[reader - abc", Seat: Seat{Topic: "reader", ResumeID: "abc"}, Runtime: RuntimeClaude, Path: "/estate/reader", Conversation: Conversation{Provider: RuntimeClaude, Source: "/transcripts/abc.jsonl", Question: "Should the reader open with a list or a preview?", Context: "Keyboard navigation works.\n\n" + strings.Repeat("Supporting evidence with wide text 漢字. ", 80), Request: "Ship the new reader", QuestionAt: now, Updated: now}}}
	m.finishThreads()
	return m
}

func press(m Model, key string) (Model, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "enter":
		msg.Type = tea.KeyEnter
	case "esc":
		msg.Type = tea.KeyEsc
	}
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func TestCatchupDoesNotDateDirtyFiles(t *testing.T) {
	m := catchupFixture()
	m.threads.ByRoot = nil
	if len(m.catchupMovements()) != 0 {
		t.Fatal("undated dirty files became recent movement")
	}
	if !strings.Contains(m.View(), "12 uncommitted files (not dated)") {
		t.Fatal(m.View())
	}
}

func TestCatchupQuestionEvidenceBeforeHandoff(t *testing.T) {
	m := catchupFixture()
	m, _ = press(m, "a")
	if !strings.Contains(m.View(), "current wait unverified") {
		t.Fatal(m.View())
	}
	m, cmd := press(m, "enter")
	if cmd != nil || m.screen != screenQuestion {
		t.Fatal("opening evidence executed a handoff")
	}
	view := m.View()
	for _, want := range []string{"Should the reader", "Keyboard navigation works"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q: %s", want, view)
		}
	}
	m, _ = press(m, "G")
	if !strings.Contains(m.View(), "/transcripts/abc.jsonl") {
		t.Fatal("source not reachable by scrolling: " + m.View())
	}
	_, cmd = press(m, "enter")
	if cmd == nil {
		t.Fatal("explicit handoff has no command")
	}
	m, _ = press(m, "esc")
	if m.screen != screenQuestions {
		t.Fatal("escape quit instead of returning")
	}
}

func TestCatchupSavedQuestionsAreNotLiveWaits(t *testing.T) {
	m := catchupFixture()
	m.threads.Threads[0].Runtime = RuntimeCodex
	if !strings.Contains(questionState(m.threads.Threads[0]), "different process") {
		t.Fatal("provider mismatch became a live wait")
	}
	if !strings.Contains(m.questionSummary(), "0 on screen · 0 other open · 1 saved") {
		t.Fatal(m.questionSummary())
	}
	m.threads.Threads[0].Conversation.Reply = "Please use the question tool"
	if len(m.questions()) != 0 {
		t.Fatal("ambiguous later reply was presented as no reply")
	}
	if !strings.Contains(questionState(m.threads.Threads[0]), "resolution unverified") {
		t.Fatal("reply falsely confirmed resolution")
	}
}

func TestCatchupRefreshWithoutChecker(t *testing.T) {
	m := catchupFixture()
	updated, cmd := m.reread()
	if cmd == nil || updated.(Model).sessionsLoaded {
		t.Fatal("failed checker prevented conversation refresh")
	}
}

func TestCatchupNarrowViewportAndSelection(t *testing.T) {
	m := catchupFixture()
	m.width, m.height = 40, 16
	m, _ = press(m, "a")
	m, _ = press(m, "enter")
	for _, view := range []string{m.View(), func() string { m, _ = press(m, "G"); return m.View() }()} {
		if len(strings.Split(view, "\n")) > m.height {
			t.Fatal("view exceeds terminal height")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > m.width {
				t.Fatalf("line too wide: %q", line)
			}
		}
	}
	m.screen = screenQuestions
	m.questionSel = m.threads.Threads[0].Session
	m.threads.Threads = append([]Thread{{Session: "other", Conversation: Conversation{Question: "New question", QuestionAt: m.now().Add(time.Hour)}}}, m.threads.Threads...)
	if m.questions()[questionIndex(m.questions(), m.questionSel)].Session != m.questionSel {
		t.Fatal("asynchronous reorder changed selection")
	}
}

func TestQuestionOnScreenRequiresActualQuestion(t *testing.T) {
	q := "Should the reader open with a list or a preview?"
	if QuestionOnScreen(q, "> Press enter to continue") {
		t.Fatal("generic prompt became question evidence")
	}
	if !QuestionOnScreen(q, "Should the reader open with a list\nor a preview?\n1. List") {
		t.Fatal("wrapped question not recognized")
	}
}
