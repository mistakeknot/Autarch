package review

import (
	"encoding/json"
	"testing"
)

func TestCaptureCommandFreezesItsOwnContext(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	var request Request
	data, _ := json.Marshal(map[string]any{"version": Version, "id": "origin", "method": "capture.command", "text": "open", "project": root, "context": map[string]any{"project": root, "item": "source-item", "terminal": map[string]any{"pid": 123, "pane": "%7", "process_ids": []int{123, 42}}}})
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	if r := s.Apply(request); r.Error != "" {
		t.Fatal(r.Error)
	}
	s.Apply(Request{Version: Version, ID: NewID(), Method: "context", Project: root, Context: &UIContext{Project: root, Item: "other-tui"}})
	encoded, _ := json.Marshal(s.Snapshot().Commands[0])
	var command map[string]any
	json.Unmarshal(encoded, &command)
	context, ok := command["context"].(map[string]any)
	if !ok || context["item"] != "source-item" {
		t.Fatalf("invocation context lost: %s", encoded)
	}
	terminal, ok := context["terminal"].(map[string]any)
	if !ok || terminal["pane"] != "%7" {
		t.Fatalf("pane context lost: %s", encoded)
	}
}
