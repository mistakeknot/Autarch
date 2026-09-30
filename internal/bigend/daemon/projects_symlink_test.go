package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

// A symlinked child is discovered only when its target stays inside the scan root.
func TestDiscoverSymlinkContainment(t *testing.T) {
	scan, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(scan, "real", ".gurgeh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "secret", ".coldwine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(scan, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(scan, "real"), filepath.Join(scan, "inside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(scan, "missing"), filepath.Join(scan, "dangling")); err != nil {
		t.Fatal(err)
	}

	m := NewProjectManager([]string{scan})
	if _, ok := m.Get(filepath.Join(scan, "escape")); ok {
		t.Fatal("symlink escaping the scan root must not be discovered")
	}
	if _, ok := m.Get(filepath.Join(scan, "dangling")); ok {
		t.Fatal("dangling symlink must not be discovered")
	}
	if _, ok := m.Get(filepath.Join(scan, "inside")); !ok {
		t.Fatal("symlink inside the scan root should be discovered")
	}
	if _, ok := m.Get(filepath.Join(scan, "real")); !ok {
		t.Fatal("real directory should be discovered")
	}
}

// GetTasks re-checks containment: a project directory swapped for a symlink
// out of the scan root after discovery must not be read.
func TestGetTasksRefusesRetargetOutsideScanRoot(t *testing.T) {
	scan, outside := t.TempDir(), t.TempDir()
	proj := filepath.Join(scan, "p")
	if err := os.MkdirAll(filepath.Join(proj, ".coldwine"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewProjectManager([]string{scan})
	if _, ok := m.Get(proj); !ok {
		t.Fatal("project not discovered")
	}
	if err := os.MkdirAll(filepath.Join(outside, ".coldwine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, proj); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetTasks(proj); err == nil {
		t.Fatal("GetTasks must refuse a project that now resolves outside the scan root")
	}
}
