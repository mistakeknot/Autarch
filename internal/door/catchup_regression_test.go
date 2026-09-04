package door

import (
	"errors"
	"strings"
	"testing"
)

func TestReviewQuestionsToThreadsNavigation(t *testing.T) {
	for _, detail := range []bool{false, true} {
		m := catchupFixture()
		m, _ = press(m, "a")
		if detail {
			m, _ = press(m, "enter")
		}
		m, _ = press(m, "t")
		if m.screen != screenThreads {
			t.Errorf("t advertised but inert; detail=%v screen=%v", detail, m.screen)
		}
	}
}
func TestReviewUnreadGitDoesNotClaimNoCommits(t *testing.T) {
	m := catchupFixture()
	m.threads.Threads[0].Conversation.Report = "Checked keyboard navigation."
	m.finishThreads()
	m.movements["/estate/reader"] = Movement{Root: "/estate/reader", Name: "reader", Err: errors.New("git unavailable")}
	s := strings.Join(m.catchupLines(25), "\n")
	if strings.Contains(s, "no recent commit found") {
		t.Fatalf("unread git shown as no commit: %s", s)
	}
}
func TestReviewOldAgentReportNotPromotedByNewUserRequest(t *testing.T) {
	p := conversationFile(t, `{"type":"assistant","timestamp":"2026-09-01T10:00:00Z","message":{"content":"Old agent report from before this visit window."}}`, `{"type":"user","timestamp":"2026-09-04T10:00:00Z","message":{"content":"Please check the current state."}}`)
	c, err := ReadConversation(p, RuntimeClaude)
	if err != nil {
		t.Fatal(err)
	}
	m := catchupFixture()
	m.threads.Threads[0].Conversation = c
	m.finishThreads()
	s := strings.Join(m.catchupLines(25), "\n")
	if strings.Contains(s, "claude reported: Old agent report") {
		t.Fatalf("old report presented in recent changes: %s", s)
	}
}
