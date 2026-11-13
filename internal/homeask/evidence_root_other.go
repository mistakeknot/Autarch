//go:build !unix

package homeask

import (
	"fmt"
	"os"
)

func prepareEvidenceRoot(root string) error {
	if err := os.MkdirAll(root, 0700); err != nil {
		return fmt.Errorf("cannot create evidence store")
	}
	if st, err := os.Lstat(root); err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid evidence store")
	}
	return os.Chmod(root, 0700)
}
