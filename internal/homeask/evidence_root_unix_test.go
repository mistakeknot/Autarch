//go:build unix

package homeask

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvidenceRootPermissions(t *testing.T) {
	for _, tc := range []struct {
		name              string
		mode              os.FileMode
		existing, refused bool
	}{
		{"shared", 0755, true, true},
		{"new", 0700, false, false},
		{"private", 0700, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "store")
			if tc.existing {
				if err := os.Mkdir(root, tc.mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(root, tc.mode); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(root, 0700) })
			}
			t.Setenv("AUTARCH_HOME_EVIDENCE", root)
			path := filepath.Join(t.TempDir(), "image.png")
			if err := os.WriteFile(path, imageBytes(32), 0600); err != nil {
				t.Fatal(err)
			}
			ask := map[string]any{"evidence": []any{map[string]any{"caption": "Screenshot", "image_path": path}}}
			err := PrepareEvidence(ask)
			if tc.refused {
				if err == nil {
					t.Error("accepted shared evidence root")
				} else if !strings.Contains(err.Error(), root) || !strings.Contains(err.Error(), "0755") || !strings.Contains(err.Error(), "private directory") {
					t.Errorf("unclear error: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			st, statErr := os.Stat(root)
			if statErr != nil {
				t.Fatal(statErr)
			}
			if st.Mode().Perm() != tc.mode {
				t.Errorf("root mode = %04o, want %04o", st.Mode().Perm(), tc.mode)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if tc.refused {
				want = 0
			}
			if len(entries) != want {
				t.Errorf("store has %d entries, want %d", len(entries), want)
			}
		})
	}
}

func TestEvidenceRootRefusesNonDirectories(t *testing.T) {
	for _, kind := range []string{"symlink", "file", "symlink-trailing-slash"} {
		t.Run(kind, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "store")
			if kind == "file" {
				if err := os.WriteFile(root, nil, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				target := t.TempDir()
				if err := os.Chmod(target, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, root); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "symlink-trailing-slash" {
				root += string(os.PathSeparator)
			}
			t.Setenv("AUTARCH_HOME_EVIDENCE", root)
			if err := prepareEvidenceRoot(EvidenceRoot()); err == nil {
				t.Fatal("accepted non-directory evidence root")
			}
		})
	}
}

func TestPrepareEvidenceUsesNormalizedRoot(t *testing.T) {
	base := t.TempDir()
	other := t.TempDir()
	target := filepath.Join(other, "sub")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "store")
	// Preserve the spelling so the import must normalize its root consistently.
	t.Setenv("AUTARCH_HOME_EVIDENCE", base+"/link/../store")
	data := imageBytes(32)
	image := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(image, data, 0600); err != nil {
		t.Fatal(err)
	}
	ask := map[string]any{"evidence": []any{map[string]any{"caption": "Screenshot", "image_path": image}}}
	if err := PrepareEvidence(ask); err != nil {
		t.Fatalf("import with normalized root: %v", err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	stored, err := os.ReadFile(filepath.Join(root, hash))
	if err != nil || !bytes.Equal(stored, data) {
		t.Fatalf("stored image: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("store entries: %v, %v", entries, err)
	}
	entries, err = os.ReadDir(other)
	if err != nil || len(entries) != 1 || entries[0].Name() != "sub" {
		t.Fatalf("other entries: %v, %v", entries, err)
	}
}
