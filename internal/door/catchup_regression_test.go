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

func TestCatchupQuestionsDistinguishPendingFailedAndEmptyReads(t *testing.T) {
	for _, phase := range []string{"starting", "reading", "failed", "empty"} {
		t.Run(phase, func(t *testing.T) {
			m := catchupFixture()
			m.threads.Threads = nil
			m.sessionsLoaded, m.threadsLoaded = true, true
			switch phase {
			case "starting":
				m.sessionsLoaded = false
			case "reading":
				m.threadsLoaded = false
			case "failed":
				m.threads.Err = errors.New("Permission denied")
			}
			summary := m.questionSummary()
			list := strings.Join(m.questionsLines(25), "\n")
			if phase == "empty" {
				if !strings.Contains(summary, "0 on screen") || !strings.Contains(list, "No question") {
					t.Fatal(summary, list)
				}
				return
			}
			if strings.Contains(summary, "0 on screen") || strings.Contains(list, "No question") {
				t.Fatalf("%s became an empty answer: %s\n%s", phase, summary, list)
			}
			m.projects, m.movements = nil, nil
			if strings.Contains(strings.Join(m.catchupLines(25), "\n"), "No recent commits or conversation activity") {
				t.Fatal("unknown conversations became no recent activity")
			}
			want := "Reading conversations"
			if phase == "failed" {
				want = "Permission denied"
			}
			if !strings.Contains(summary, want) || !strings.Contains(list, want) {
				t.Fatal(summary, list)
			}
		})
	}
}
