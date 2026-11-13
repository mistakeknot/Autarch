//go:build unix

package homeask

import (
	"fmt"
	"os"
	"path/filepath"
)

func prepareEvidenceRoot(root string) error {
	// Mkdir, unlike MkdirAll, tells us whether this call created the root.
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return fmt.Errorf("cannot create evidence store")
	}
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("cannot create evidence store")
	}
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid evidence store")
	}
	// A Unix umask can only remove permissions from the requested 0700.
	// Never change permissions on a directory supplied by the caller.
	if st.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("evidence store %q has mode %04o; use a private directory or fix its mode", root, st.Mode().Perm())
	}
	return nil
}
