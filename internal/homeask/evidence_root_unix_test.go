//go:build unix

package homeask

import (
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
