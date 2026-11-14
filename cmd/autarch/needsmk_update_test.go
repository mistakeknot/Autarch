package main

import (
	"encoding/json"
	"github.com/mistakeknot/autarch/internal/homeask"
	"os"
	"strings"
	"testing"
)

func TestNeedsMkRefilingCreatesReplacement(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	s := &stubBB{}
	p := writeAsk(t)
	raw, _ := os.ReadFile(p)
	var ask map[string]any
	json.Unmarshal(raw, &ask)
	ask["ask_key"] = "same-ask"
	raw, _ = json.Marshal(ask)
	os.WriteFile(p, raw, 0600)
	args := []string{"file", "--project", "P1", "--title", "Ship?", "--ask-file", p}
	first, err := runNeedsMk(t, s, args...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first, `"replay":false`) || s.creates != 1 {
		t.Fatal(first)
	}
	original, _ := homeask.ParseCard(s.desc)
	ask["question"] = "A refreshed question?"
	raw, _ = json.Marshal(ask)
	os.WriteFile(p, raw, 0600)
	out, err := runNeedsMk(t, s, args...)
	if err != nil || strings.Contains(out, `"updated":true`) || !strings.Contains(out, `"replay":false`) || s.creates != 2 || len(s.updated) != 0 {
		t.Fatalf("out=%s err=%v creates=%d updates=%d", out, err, s.creates, len(s.updated))
	}
	back, _ := homeask.ParseCard(s.desc)
	if back.Question != "A refreshed question?" || back.Request == original.Request || s.thread != "thr_x" {
		t.Fatalf("card=%+v", back)
	}
}
