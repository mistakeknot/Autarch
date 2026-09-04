package door

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func conversationFile(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "conversation.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

const userRequest = `{"type":"user","timestamp":"2026-09-04T10:00:00Z","message":{"role":"user","content":"Help me ship the new reader."}}`
const assistantContext = `{"type":"assistant","timestamp":"2026-09-04T10:01:00Z","message":{"role":"assistant","content":[{"type":"text","text":"The reader passes the keyboard check. The remaining choice is its starting layout."}]}}`
const questionTool = `{"type":"assistant","timestamp":"2026-09-04T10:02:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"ask1","name":"AskUserQuestion","input":{"questions":[{"question":"Should the reader open with a list or a preview?","options":[{"label":"List","description":"Compact overview"},{"label":"Preview","description":"Read immediately"}]}]}}]}}`

func TestConversationKeepsQuestionAndEvidence(t *testing.T) {
	p := conversationFile(t, userRequest, assistantContext, questionTool, `{"type":"bridge-session","timestamp":"2026-09-04T12:00:00Z"}`)
	c, err := ReadConversation(p, RuntimeClaude)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Question, "list or a preview?") || !strings.Contains(c.Question, "Compact overview") {
		t.Fatalf("lost question/options: %+v", c)
	}
	if c.Context != "The reader passes the keyboard check. The remaining choice is its starting layout." {
		t.Fatalf("lost context: %+v", c)
	}
	if c.Request != "Help me ship the new reader." || c.QuestionLine != 3 || c.Source != p || c.Updated.Hour() != 10 {
		t.Fatalf("wrong evidence provenance: %+v", c)
	}
}

func TestConversationAnsweredQuestionDisappears(t *testing.T) {
	for name, answer := range map[string]string{
		"structured": `{"type":"user","timestamp":"2026-09-04T10:03:00Z","message":{"content":[{"type":"tool_result","tool_use_id":"ask1","content":"User has answered your questions: List"}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, err := ReadConversation(conversationFile(t, userRequest, assistantContext, questionTool, answer), RuntimeClaude)
			if err != nil {
				t.Fatal(err)
			}
			if c.Question != "" {
				t.Fatalf("answered question still pending: %+v", c)
			}
		})
	}
}

func TestConversationToolResultsAndHookFeedbackAreNotHumanAnswers(t *testing.T) {
	p := conversationFile(t, userRequest, assistantContext, questionTool,
		`{"type":"user","timestamp":"2026-09-04T10:03:00Z","message":{"content":[{"type":"tool_result","tool_use_id":"other","content":"ok"}]}}`,
		`{"type":"user","timestamp":"2026-09-04T10:04:00Z","message":{"content":"Stop hook feedback: continue working"}}`)
	c, err := ReadConversation(p, RuntimeClaude)
	if err != nil {
		t.Fatal(err)
	}
	if c.Question == "" || c.Request != "Help me ship the new reader." {
		t.Fatalf("automation cleared or replaced human evidence: %+v", c)
	}
}

func TestConversationDoesNotTreatReportAsQuestion(t *testing.T) {
	p := conversationFile(t, userRequest, `{"type":"assistant","timestamp":"2026-09-04T10:03:00Z","message":{"content":[{"type":"text","text":"Why a list? It matches the existing navigation. Implemented and checked."}]}}`)
	c, err := ReadConversation(p, RuntimeClaude)
	if err != nil {
		t.Fatal(err)
	}
	if c.Question != "" {
		t.Fatalf("rhetorical question became obligation: %+v", c)
	}
}

func TestConversationCodexAsyncQuestionNeedsHumanAnswer(t *testing.T) {
	lines := []string{
		`{"type":"response_item","timestamp":"2026-09-04T10:00:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Finish the reader."}]}}`,
		`{"type":"response_item","timestamp":"2026-09-04T10:01:00Z","payload":{"type":"message","role":"assistant","channel":"commentary","content":[{"type":"output_text","text":"Both layouts work with the keyboard."}]}}`,
		`{"type":"response_item","timestamp":"2026-09-04T10:02:00Z","payload":{"type":"function_call","name":"request_user_input_async","call_id":"ask2","arguments":"{\"questions\":[{\"title\":\"List or preview?\",\"options\":[\"List\",\"Preview\"]}]}"}}`,
		`{"type":"response_item","timestamp":"2026-09-04T10:02:01Z","payload":{"type":"function_call_output","call_id":"ask2","output":"{\"accepted\":true}"}}`,
	}
	c, err := ReadConversation(conversationFile(t, lines...), RuntimeCodex)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Question, "List or preview?") || c.Context != "Both layouts work with the keyboard." {
		t.Fatalf("lost async request: %+v", c)
	}
	lines = append(lines, `{"type":"response_item","timestamp":"2026-09-04T10:03:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Preview."}]}}`)
	c, err = ReadConversation(conversationFile(t, lines...), RuntimeCodex)
	if err != nil || c.Question == "" || c.Reply != "Preview." {
		t.Fatalf("reply uncertainty not preserved: %+v %v", c, err)
	}
}

func TestConversationFollowupDoesNotSilentlyAnswerQuestion(t *testing.T) {
	p := conversationFile(t, userRequest, assistantContext, questionTool, `{"type":"user","timestamp":"2026-09-04T10:03:00Z","message":{"content":"Please ask using the question tool."}}`)
	c, err := ReadConversation(p, RuntimeClaude)
	if err != nil || c.Question == "" || c.Reply == "" || c.Request != "Help me ship the new reader." {
		t.Fatalf("lost question or original request: %+v %v", c, err)
	}
}

func TestStoppedThreadRetainsConversationAndGarden(t *testing.T) {
	root := t.TempDir()
	id := "aaaa1111-bbbb-2222-cccc-333344445555"
	p := filepath.Join(root, "-estate", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	msg, _ := json.Marshal(map[string]any{"type": "assistant", "timestamp": "2026-09-04T10:01:00Z", "message": map[string]any{"content": []map[string]string{{"type": "text", "text": "Reader ready in /estate/reader/docs/why.md."}}}})
	if err := os.WriteFile(p, append(append(msg, '\n'), []byte(questionTool+"\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	var got Thread
	ReadThreads(context.Background(), []TmuxSession{{Name: "iterm[reader - " + id, Path: "/estate", Command: "zsh"}}, []Project{{Name: "reader", Root: "/estate/reader"}}, nil, root, func(th Thread) { got = th })
	if got.Runtime != RuntimeShell || got.LastTurn.IsZero() || len(got.Gardens) != 1 {
		t.Fatalf("stopped conversation lost context: %+v", got)
	}
	if got.Conversation.Question == "" || got.Conversation.Provider != RuntimeClaude {
		t.Fatalf("lost stopped question: %+v", got)
	}
}
