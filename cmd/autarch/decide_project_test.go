package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
)

// worktreeRoot is what a bb worktree checkout looks like: the folder name is not the project.
const worktreeRoot = "/home/dev/worktrees/thr_1/Autarch-project-visit"

// withBoundFiler runs decide file against a tasks service whose board P1 is bound to the Home
// project "shadow-work" (bindingStub). The filer files into P1 and makes no live bb call.
func withBoundFiler(t *testing.T, s *stubBB) {
	t.Helper()
	old := newFiler
	t.Cleanup(func() { newFiler = old })
	newFiler = func() *homeask.CardFiler {
		return &homeask.CardFiler{Run: bindingStub(s), LockDir: t.TempDir(), TasksProject: "P1"}
	}
}

func askWithProject(t *testing.T, project, root string) string {
	t.Helper()
	m := map[string]any{
		"v": 1, "kind": "decide", "asker": "thread", "thread": "thr-x", "subject": "autarch/decide: order",
		"question": "Which order?",
		"options": []map[string]any{
			{"id": "a", "label": "A first", "kind": "instruction", "reversible": true, "instruction": "Do A then B and report."},
			{"id": "b", "label": "B first", "kind": "needs-context"},
		},
	}
	if project != "" {
		m["project"] = project
	}
	if root != "" {
		m["project_root"] = root
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// B4: a project that is not the one bound to the card's task board is hidden by Home, so decide
// file must refuse it (exit 2, naming the expected project) and create nothing.
func TestDecideFileRefusesAProjectHomeWouldHide(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "")
	s := &stubBB{}
	withBoundFiler(t, s)
	out, err := runDecide(t, askWithProject(t, "shadow-workipedia", "/srv/shadow"), "file")
	if exitCode(err) != 2 || err == nil || !strings.Contains(err.Error(), `expected project "shadow-work"`) {
		t.Fatalf("out=%q exit=%d err=%v", out, exitCode(err), err)
	}
	if s.creates != 0 || len(writeCalls(s)) != 0 {
		t.Fatalf("created %d cards; write calls %v", s.creates, writeCalls(s))
	}
}

// B5: with no project in the ask and no --project, the project is the Home project the task
// board is bound to, not the checkout's folder name.
func TestDecideFileDefaultsTheProjectToTheBoardBindingInAWorktree(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "")
	s := &stubBB{}
	withBoundFiler(t, s)
	out, err := runDecide(t, askWithProject(t, "", ""), "file", "--project-root", worktreeRoot)
	if err != nil || out != "T1" {
		t.Fatalf("out=%q err=%v calls=%v", out, err, s.calls)
	}
	if !strings.Contains(s.desc, `"project":"shadow-work"`) || strings.Contains(s.desc, "Autarch-project-visit") {
		t.Fatalf("filed with the wrong project: %s", s.desc)
	}
}

// B5: the folder name is only the last fallback, when the board has no confirmed binding.
func TestDecideFileFallsBackToTheFolderNameWithoutABinding(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "")
	s := &stubBB{}
	withCardFiler(t, s)
	out, err := runDecide(t, askWithProject(t, "", ""), "file", "--project-root", "/srv/Folder-Name")
	if err != nil || out != "T1" || !strings.Contains(s.desc, `"project":"Folder-Name"`) {
		t.Fatalf("out=%q err=%v desc=%s", out, err, s.desc)
	}
}
