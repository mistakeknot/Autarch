package specs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/mistakeknot/vauxpraudemonium/internal/file"
)

func TestUpdateUserStory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	if err := os.WriteFile(path, []byte("id: T1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateUserStory(path, "New story"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "user_story") {
		t.Fatal("expected user_story in yaml")
	}
}

func TestAppendReviewFeedback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	if err := os.WriteFile(path, []byte("id: T1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendReviewFeedback(path, "Needs tests"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "review_feedback") {
		t.Fatal("expected review_feedback in yaml")
	}
}

func TestUpdateUserStoryWritesHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	data := []byte(`id: T1
user_story:
  text: As a user, I want X.
  hash: oldhash
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateUserStory(path, "As a user, I want Y."); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, err := LoadDetail(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.UserStoryHash == "oldhash" || updated.UserStoryHash == "" {
		t.Fatalf("expected updated hash")
	}
}

func TestAppendMVPExplanation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	if err := os.WriteFile(path, []byte("id: T1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendMVPExplanation(path, "Approved for launch"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "mvp_explanation") {
		t.Fatal("expected mvp_explanation in yaml")
	}
}

func TestAcknowledgeMVPOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	if err := os.WriteFile(path, []byte("id: T1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AcknowledgeMVPOverride(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "mvp_override") {
		t.Fatal("expected mvp_override in yaml")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")
	data := []byte("id: T1\n")
	if err := fileutil.AtomicWriteFile(path, data, 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected output file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Fatalf("expected temp file removed")
		}
	}
}
