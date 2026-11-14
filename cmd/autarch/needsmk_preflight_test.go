package main

import (
	"bytes"
	"github.com/spf13/cobra"
	"strings"
	"testing"
)

func TestNeedsMkCheckDoesNotSubmit(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	s := &stubBB{}
	out, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Choose", "--ask-file", writeAsk(t), "--check")
	if err != nil || !strings.Contains(out, `"checked":true`) || len(writeCalls(s)) != 0 {
		t.Fatalf("out=%s err=%v writes=%v", out, err, writeCalls(s))
	}
}

func TestCLIReplayWithMissingTemporaryMedia(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	s := &stubBB{}
	args := []string{"file", "--project", "P1", "--title", "Choose", "--ask-file", writeAsk(t)}
	if _, err := runNeedsMk(t, s, args...); err != nil {
		t.Fatal(err)
	}
	args = append(args, "--ui", "--card-type", "play", "--attach", t.TempDir()+"/gone.png")
	out, err := runNeedsMk(t, s, args...)
	if err != nil || !strings.Contains(out, `"replay":true`) {
		t.Fatal(out, err)
	}
	if s.creates != 1 {
		t.Fatal("replay created a new card")
	}
}

func TestNeedsMkAggregateOwnershipAndFormat(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	// Deliberately bypass the fixture helper that supplies an owner.
	root := &cobra.Command{Use: "autarch"}
	root.AddCommand(needsMkCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"needs-mk", "file", "--project", "P1", "--title", "Choose", "--card-type", "design", "--ask-file", writeAskBody(t, `{"project":"sample","project_root":"/sample","question":"Which?","options":[{"id":"A","label":"A","kind":"ruling-only"},{"id":"b","label":"Wait","kind":"ruling-only"}]}`), "--check"})
	// runNeedsMk installs the stubs; --check completes before any mutation.
	s := &stubBB{}
	_, _ = runNeedsMk(t, s, "file", "--project", "P1", "--title", "Choose", "--ask-file", writeAsk(t), "--check")
	err := root.Execute()
	for _, want := range []string{"lowercase", "three-line", "owner-thread"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %s: %v", want, err)
		}
	}
	if s.creates != 0 {
		t.Fatal("submitted invalid card")
	}
}
