package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPrepareAskRefusesTheWrongProjectBeforeTheCheck(t *testing.T) {
	f := bindingFiler(confirmed)
	checked := false
	f.Check = func(string, string) error { checked = true; return nil }
	_, _, err := f.PrepareAsk(context.Background(), "SHWK", "shadow-workipedia", "/srv/x")
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), `expected project "shadow-work"`) || checked {
		t.Fatalf("err=%v checked=%v", err, checked)
	}
}

func TestPrepareAskChecksTheRootAgainstTheResolvedName(t *testing.T) {
	f := bindingFiler(confirmed)
	var gp, gr string
	f.Check = func(p, r string) error { gp, gr = p, r; return nil }
	got, _, err := f.PrepareAsk(context.Background(), "SHWK", "SHWK", "/srv/x")
	if err != nil || got != "shadow-work" || gp != "shadow-work" || gr != "/srv/x" {
		t.Fatalf("got=%q err=%v checked=(%q,%q)", got, err, gp, gr)
	}
}

func TestPrepareAskReturnsTheCheckError(t *testing.T) {
	f := bindingFiler(confirmed)
	want := errors.New("root not listed")
	f.Check = func(string, string) error { return want }
	if _, _, err := f.PrepareAsk(context.Background(), "SHWK", "shadow-work", "/srv/x"); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}

func TestPrepareAskFailsOpenWhenHomeCannotAnswer(t *testing.T) {
	f := bindingFiler("")
	got, _, err := f.PrepareAsk(context.Background(), "SHWK", "anything", "/srv/x")
	if err != nil || got != "anything" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestDefaultProjectPrefersTheBindingThenTheFolderName(t *testing.T) {
	ctx := context.Background()
	bound := bindingFiler(confirmed)
	bound.TasksProject = "SHWK"
	if got := DefaultProject(ctx, bound, "/w/Autarch-project-visit"); got != "shadow-work" {
		t.Fatalf("bound: %q", got)
	}
	unbound := bindingFiler("")
	unbound.TasksProject = "SHWK"
	if got := DefaultProject(ctx, unbound, "/w/Autarch-project-visit"); got != "Autarch-project-visit" {
		t.Fatalf("unbound: %q", got)
	}
	if got := DefaultProject(ctx, nil, "/w/Folder"); got != "Folder" {
		t.Fatalf("nil filer: %q", got)
	}
	none := bindingFiler(confirmed)
	if got := DefaultProject(ctx, none, "/w/Folder"); got != "Folder" {
		t.Fatalf("no board configured: %q", got)
	}
	one := bindingFiler(confirmed)
	one.TasksProjects = map[string]string{"shadow-work": "SHWK"}
	if got := DefaultProject(ctx, one, "/w/Folder"); got != "shadow-work" {
		t.Fatalf("sole TasksProjects entry: %q", got)
	}
}

func TestFilePrepareRefusesBeforeCreatingAnything(t *testing.T) {
	f := bindingFiler(confirmed)
	f.TasksProject = "SHWK"
	f.LockDir = t.TempDir()
	ask, err := Parse([]byte(`{"v":1,"kind":"decide","asker":"thread","thread":"t","subject":"s","question":"q?","project":"shadow-workipedia","project_root":"/srv/x","options":[{"id":"a","label":"A","kind":"ruling-only"},{"id":"b","label":"B","kind":"ruling-only"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.File(context.Background(), ask)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), `expected project "shadow-work"`) {
		t.Fatalf("err=%v", err)
	}
}

func TestPrepareAskKeepsTheWarningWhenTheCheckFails(t *testing.T) {
	f := bindingFiler(`{"home_project":"shadow-work","state":"unconfirmed"}`)
	f.Check = func(string, string) error { return errors.New("root not listed") }
	_, warn, err := f.PrepareAsk(context.Background(), "SHWK", "shadow-work", "/srv/x")
	if err == nil || !strings.Contains(warn, "no confirmed Home project binding") {
		t.Fatalf("warn=%q err=%v", warn, err)
	}
}
