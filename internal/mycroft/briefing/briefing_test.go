package briefing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

func TestGenerate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "briefings")
	bead := mycroft.BeadView{
		ID:         "Demarch-42",
		Title:      "Fix flaky integration test",
		Type:       "bug",
		Priority:   1,
		Complexity: "simple",
		Labels:     []string{"bug", "complexity/simple"},
	}

	path, err := Generate(dir, "grey-area", bead)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read briefing: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Fix flaky integration test") {
		t.Error("missing title")
	}
	if !strings.Contains(content, "grey-area") {
		t.Error("missing agent name")
	}
	if !strings.Contains(content, "Demarch-42") {
		t.Error("missing bead ID")
	}
	if !strings.Contains(content, "P1") {
		t.Error("missing priority")
	}
}

func TestValidateContextPath(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "src")
	os.MkdirAll(subdir, 0755)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid absolute under root", filepath.Join(root, "src", "main.go"), false},
		{"relative path", "src/main.go", true},
		{"traversal", filepath.Join(root, "..", "escape"), true},
		{"root itself", root, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContextPath(tt.path, root)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContextPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidateContextPathSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// Create a symlink from root/link → outside.
	link := filepath.Join(root, "escape-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlink not supported")
	}

	err := ValidateContextPath(filepath.Join(link, "secret.txt"), root)
	if err == nil {
		t.Error("expected error for symlink escaping project root")
	}
}
