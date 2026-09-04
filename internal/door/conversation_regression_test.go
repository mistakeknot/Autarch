package door

import (
	"strings"
	"testing"
)

func TestReviewPlainQuestionKeepsCurrentContext(t *testing.T) {
	p := conversationFile(t, userRequest, `{"type":"assistant","timestamp":"2026-09-04T10:03:00Z","message":{"content":"The list scans quickly; the preview shows more detail.\n\nWhich should open first?"}}`)
	c, err := ReadConversation(p, RuntimeClaude)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Context, "list scans quickly") {
		t.Fatalf("supporting paragraph omitted: question=%q context=%q report=%q", c.Question, c.Context, c.Report)
	}
}
func TestReviewProgressDoesNotAnswerPlainQuestion(t *testing.T) {
	p := conversationFile(t, userRequest, `{"type":"assistant","timestamp":"2026-09-04T10:01:00Z","message":{"content":"Should the reader open with a list or a preview?"}}`, `{"type":"assistant","timestamp":"2026-09-04T10:02:00Z","message":{"content":"I am checking keyboard navigation while you decide."}}`)
	c, err := ReadConversation(p, RuntimeClaude)
	if err != nil {
		t.Fatal(err)
	}
	if c.Question == "" {
		t.Fatal("progress report cleared unanswered question")
	}
}
func TestReviewEmptyStructuredAnswersStayPending(t *testing.T) {
	p := conversationFile(t, `{"type":"response_item","timestamp":"2026-09-04T10:01:00Z","payload":{"type":"function_call","name":"request_user_input","call_id":"q1","arguments":"{\"questions\":[{\"id\":\"layout\",\"question\":\"Which layout?\"}]}"}}`, `{"type":"response_item","timestamp":"2026-09-04T10:02:00Z","payload":{"type":"function_call_output","call_id":"q1","output":"{\"answers\":{\"layout\":{\"answers\":[]}}}"}}`)
	c, err := ReadConversation(p, RuntimeCodex)
	if err != nil {
		t.Fatal(err)
	}
	if c.Question == "" {
		t.Fatal("empty answer list cleared question")
	}
}
func TestReviewMetaMessageDoesNotAnswer(t *testing.T) {
	p := conversationFile(t, userRequest, assistantContext, questionTool, `{"type":"user","isMeta":true,"timestamp":"2026-09-04T10:03:00Z","message":{"content":"Automated session notice: continue independent work."}}`)
	c, err := ReadConversation(p, RuntimeClaude)
	if err != nil {
		t.Fatal(err)
	}
	if c.Question == "" {
		t.Fatal("metadata user message cleared question")
	}
}
