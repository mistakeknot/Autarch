package homeask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func prMove() map[string]any {
	return map[string]any{"schema": "home-move/v1", "kind": "pr", "pr": map[string]any{"url": "https://github.com/o/r/pull/12"}}
}

func scriptMove() map[string]any {
	sha := strings.Repeat("a", 64)
	return map[string]any{"schema": "home-move/v1", "kind": "script", "script": map[string]any{"path": "/srv/x/run.sh", "sha256": sha, "args": []any{"--check"}, "recover": map[string]any{"path": "/srv/x/undo.sh", "sha256": sha}}}
}

func TestFileCardWithMoveLabelsAndWritesTheBlock(t *testing.T) {
	for name, mv := range map[string]map[string]any{
		"pr":      prMove(),
		"script":  scriptMove(),
		"read":    {"schema": "home-move/v1", "kind": "read", "read": map[string]any{"url": "https://github.com/o/r/pull/3"}},
		"context": {"schema": "home-move/v1", "kind": "context", "context": map[string]any{"need": "the staging token's owner"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			req := testReq()
			req.Move = mv
			res, err := f.filer().FileCard(context.Background(), req)
			if err != nil || res.ID != "T1" {
				t.Fatalf("res=%+v err=%v", res, err)
			}
			if !f.labels[MoveLabel] || !f.labels[NeedsMkLabel] {
				t.Fatalf("labels %v", f.labels)
			}
			tk := f.tasks[0]
			if strings.Join(tk.Labels, ",") != NeedsMkLabel+","+MoveLabel {
				t.Fatalf("task labels %v", tk.Labels)
			}
			c, err := ParseCard(tk.Desc)
			if err != nil || c.Move == nil || c.Move.Kind != name {
				t.Fatalf("card does not read back with its move: %v %+v\n%s", err, c.Move, tk.Desc)
			}
		})
	}
}

func TestFileCardWithoutMoveStaysUnchanged(t *testing.T) {
	f := newFake(t)
	if _, err := f.filer().FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	if f.labels[MoveLabel] || strings.Contains(f.tasks[0].Desc, "home-move") || len(f.tasks[0].Labels) != 1 {
		t.Fatalf("a card with no move got move state: %v %v", f.labels, f.tasks[0].Labels)
	}
	a, _ := RequestIdentity(testReq())
	r := testReq()
	r.Move = prMove()
	b, _ := RequestIdentity(r)
	if a == b {
		t.Fatal("the move is not part of the request identity")
	}
}

func TestFileCardRefusesAMalformedMoveBeforeCreatingAnything(t *testing.T) {
	bad := map[string]map[string]any{
		"unknown field":   {"schema": "home-move/v1", "kind": "pr", "pr": map[string]any{"url": "https://github.com/o/r/pull/1"}, "run": "rm -rf /"},
		"pr off github":   {"schema": "home-move/v1", "kind": "pr", "pr": map[string]any{"url": "https://evil.example/o/r/pull/1"}},
		"relative script": {"schema": "home-move/v1", "kind": "script", "script": map[string]any{"path": "run.sh", "sha256": strings.Repeat("a", 64)}},
		"dotdot script":   {"schema": "home-move/v1", "kind": "script", "script": map[string]any{"path": "/a/../b", "sha256": strings.Repeat("a", 64)}},
		"short sha":       {"schema": "home-move/v1", "kind": "script", "script": map[string]any{"path": "/a/b", "sha256": "abc"}},
		"shell arg":       {"schema": "home-move/v1", "kind": "script", "script": map[string]any{"path": "/a/b", "sha256": strings.Repeat("a", 64), "args": []any{"; rm -rf /"}}},
		"read other host": {"schema": "home-move/v1", "kind": "read", "read": map[string]any{"url": "https://example.com/x"}},
		"empty need":      {"schema": "home-move/v1", "kind": "context", "context": map[string]any{"need": "  "}},
		"wrong schema":    {"schema": "home-move/v2", "kind": "pr", "pr": map[string]any{"url": "https://github.com/o/r/pull/1"}},
		"recover args":    {"schema": "home-move/v1", "kind": "script", "script": map[string]any{"path": "/a/b", "sha256": strings.Repeat("a", 64), "recover": map[string]any{"path": "/a/c", "sha256": strings.Repeat("a", 64), "args": []any{"x"}}}},
	}
	for name, mv := range bad {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			req := testReq()
			req.Move = mv
			_, err := f.filer().FileCard(context.Background(), req)
			wantErrIs(t, err, ErrInvalid)
			if f.cards() != 0 || f.count("tasks", "create") != 0 {
				t.Fatal("something was created for a refused move")
			}
		})
	}
}

func TestParseCardRefusesTwoMovesAndABadMove(t *testing.T) {
	f := newFake(t)
	req := testReq()
	req.Move = prMove()
	if _, err := f.filer().FileCard(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	d := f.tasks[0].Desc
	if _, err := ParseCard(d + "\n```home-move\n{}\n```\n"); err == nil || !strings.Contains(err.Error(), "more than one home-move block") {
		t.Fatalf("two blocks: %v", err)
	}
	if _, err := ParseCard(strings.Replace(d, `"kind":"pr"`, `"kind":"exec"`, 1)); err == nil || !strings.Contains(err.Error(), "kind must be") {
		t.Fatalf("bad kind: %v", err)
	}
	if _, err := ParseCard(strings.Replace(d, "```home-move\n", "```home-move\n", 1)[:len(d)-4]); err == nil || !strings.Contains(err.Error(), "unterminated home-move block") {
		t.Fatalf("unterminated: %v", err)
	}
}

func TestAdoptMoveAttachesToAnExistingCard(t *testing.T) {
	f := newFake(t)
	if _, err := f.filer().FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	before, _ := ParseCard(f.tasks[0].Desc)
	res, err := f.filer().AdoptMove(context.Background(), "T1", "thr_a", prMove())
	if err != nil || res.Replay || res.ID != "T1" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	after, err := ParseCard(f.tasks[0].Desc)
	if err != nil || after.Move == nil || after.Request != before.Request || after.Question != before.Question {
		t.Fatalf("after adopt: %v %+v", err, after)
	}
	if strings.Join(f.tasks[0].Labels, ",") != NeedsMkLabel+","+MoveLabel {
		t.Fatalf("labels %v", f.tasks[0].Labels)
	}
	// Again: a replay that changes nothing.
	n := f.count("tasks", "update")
	res, err = f.filer().AdoptMove(context.Background(), "T1", "thr_a", prMove())
	if err != nil || !res.Replay || f.count("tasks", "update") != n {
		t.Fatalf("replay res=%+v err=%v updates=%d", res, err, f.count("tasks", "update"))
	}
	// A different move on the same card is refused.
	_, err = f.filer().AdoptMove(context.Background(), "T1", "thr_a", scriptMove())
	wantErrIs(t, err, ErrInvalid)
}

func TestAdoptMoveRefusals(t *testing.T) {
	f := newFake(t)
	if _, err := f.filer().FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := f.filer().AdoptMove(ctx, "T1", "thr_other", prMove()); err == nil || !strings.Contains(err.Error(), "not this thread") {
		t.Fatalf("other thread: %v", err)
	}
	if _, err := f.filer().AdoptMove(ctx, "T1", "", prMove()); err == nil {
		t.Fatal("no thread accepted")
	}
	bad := prMove()
	bad["extra"] = true
	_, err := f.filer().AdoptMove(ctx, "T1", "thr_a", bad)
	wantErrIs(t, err, ErrInvalid)
	f.tasks = append(f.tasks, &fakeTask{ID: "PLAIN", Project: "P1", Desc: "just prose, no Request line", Comments: []Comment{{ID: "c1", Kind: "agent", ThreadID: "thr_a", CreatedAt: "2026-10-01T00:00:00"}}})
	_, err = f.filer().AdoptMove(ctx, "PLAIN", "thr_a", prMove())
	wantErrIs(t, err, ErrInvalid)
	if f.count("tasks", "update") != 0 {
		t.Fatal("a refused adoption changed a card")
	}
	_, err = f.filer().AdoptMove(ctx, "NOPE", "thr_a", prMove())
	if err == nil {
		t.Fatal("missing card accepted")
	}
}

// The home-move vectors are shared with the plugin's parseMove (__tests__/move-filing.test.ts reads the same file).
func TestParseMoveSharedVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(filepath.FromSlash(cardFixtureDir), "home-move.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Env   map[string]string `json:"env"`
		Cases []struct {
			Name  string  `json:"name"`
			Body  string  `json:"body"`
			Error *string `json:"error"`
			Kind  *string `json:"kind"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Cases) == 0 {
		t.Fatalf("vectors: %v", err)
	}
	for k, v := range doc.Env {
		t.Setenv(k, v)
	}
	for _, c := range doc.Cases {
		t.Run(c.Name, func(t *testing.T) {
			mv, err := ParseMove(c.Body)
			if c.Error != nil {
				if err == nil || err.Error() != *c.Error {
					t.Fatalf("err = %v, want %q", err, *c.Error)
				}
				return
			}
			if err != nil || mv.Kind != *c.Kind {
				t.Fatalf("mv=%+v err=%v", mv, err)
			}
		})
	}
}

// The extra read host comes from configuration only; without it just github.com is allowed.
func TestParseMoveReadHostIsConfigured(t *testing.T) {
	body := `{"schema":"home-move/v1","kind":"read","read":{"url":"https://docs.example.test/plan"}}`
	t.Setenv(EnvMoveReadHosts, "")
	if _, err := ParseMove(body); err == nil || err.Error() != "read url must be https on an allowed host" {
		t.Fatalf("unconfigured host accepted: %v", err)
	}
	t.Setenv(EnvMoveReadHosts, " x.example.test , DOCS.example.test")
	if _, err := ParseMove(body); err != nil {
		t.Fatalf("configured host refused: %v", err)
	}
	t.Setenv(EnvMoveReadHosts, "*.example.test,docs.example.test/path")
	if _, err := ParseMove(body); err == nil {
		t.Fatal("an invalid entry allowed a host")
	}
}
