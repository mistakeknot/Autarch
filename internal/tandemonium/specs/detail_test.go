package specs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDetail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	data := []byte(`id: T1
title: Example
summary: |
  Did the thing.
acceptance_criteria:
  - id: ac-1
    description: First
  - id: ac-2
    description: Second
user_story:
  text: As a user, I want X.
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	detail, err := LoadDetail(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.ID != "T1" || detail.Title != "Example" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if len(detail.AcceptanceCriteria) != 2 {
		t.Fatalf("expected acceptance criteria")
	}
	if detail.UserStory != "As a user, I want X." {
		t.Fatalf("expected user story")
	}
}

func TestLoadDetailParsesStoryHashAndMVP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	data := []byte(`id: T1
title: Example
user_story:
  text: As a user, I want X.
  hash: abcdef12
strategic_context:
  mvp_included: true
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	detail, err := LoadDetail(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.UserStoryHash != "abcdef12" {
		t.Fatalf("expected hash, got %q", detail.UserStoryHash)
	}
	if detail.MVPIncluded == nil || *detail.MVPIncluded != true {
		t.Fatalf("expected MVP included true")
	}
}
