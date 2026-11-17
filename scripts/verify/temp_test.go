//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Only the test binary sets the boundary; every fixture below TMPDIR remains
// subject to checkout and symlink checks. No tests mutate it after init.
func init() {
	checkoutWalkStop = filepath.Clean(os.TempDir())
	gitScratchRoot = checkoutWalkStop
}

func TestTempAncestorIsolation(t *testing.T) {
	if os.Getenv("VERIFY_TEMP_ANCESTOR_CHILD") == "1" {
		return
	}
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^(TestExecuteMergeBaseRanges|TestNamespaceSetupFailureReportsIncomplete|TestMissingFakeNeverFallsBack|TestOfflineCacheMissIsIncomplete|TestMachineDirectoryComponents)$", "-test.v")
	cmd.Env = append(os.Environ(), "TMPDIR="+parent, "VERIFY_TEMP_ANCESTOR_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tests below checkout ancestor: %v\n%s", err, out)
	}
}

func TestCheckoutAncestorRefused(t *testing.T) {
	root := t.TempDir()
	if _, err := git(root, "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := safePath(filepath.Join(root, "nested", "out"), ""); err == nil || err.Error() != "refusing path within a checkout" {
		t.Fatalf("checkout ancestor: %v", err)
	}
}
