package yamlsafe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeStrictRejectsUnknownField(t *testing.T) {
	var out struct {
		Name string `yaml:"name"`
	}
	err := DecodeStrict([]byte("name: ok\nextra: nope\n"), &out)
	if err == nil {
		t.Fatalf("expected strict decode error for unknown field")
	}
}

func TestUnmarshalFileStrictAcceptsKnownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.yaml")
	if err := os.WriteFile(path, []byte("name: test\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var out struct {
		Name string `yaml:"name"`
	}
	if _, err := UnmarshalFileStrict(path, &out); err != nil {
		t.Fatalf("strict decode failed: %v", err)
	}
	if out.Name != "test" {
		t.Fatalf("expected name=test, got %q", out.Name)
	}
}

func TestReadFileRejectsWritablePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("name: test\n"), 0o666); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := ReadFile(path); err == nil {
		t.Fatalf("expected writable-permission rejection")
	}
}

func TestReadFileRejectsOversized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.yaml")
	big := strings.Repeat("a", int(DefaultMaxBytes)+10)
	if err := os.WriteFile(path, []byte(big), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadFile(path); err == nil {
		t.Fatalf("expected oversize rejection")
	}
}

func TestReadFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yaml")
	link := filepath.Join(dir, "link.yaml")
	if err := os.WriteFile(target, []byte("name: test\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := ReadFile(link); err == nil {
		t.Fatalf("expected symlink rejection")
	}
}
