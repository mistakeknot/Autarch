package agent

import "testing"

func TestParseOpenQuestionsResponse(t *testing.T) {
	content := `{"resolved":[{"question":"Q1?","answer":"A1"}],"remaining":["Q2?"]}`
	res, err := parseOpenQuestionsResponse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Resolved) != 1 || res.Resolved[0].Question != "Q1?" {
		t.Fatalf("unexpected resolved: %#v", res.Resolved)
	}
	if len(res.Remaining) != 1 || res.Remaining[0] != "Q2?" {
		t.Fatalf("unexpected remaining: %#v", res.Remaining)
	}
}
