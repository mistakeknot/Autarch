//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var productionCheckoutWalkStop = checkoutWalkStop
var productionGitScratchRoot = gitScratchRoot

func TestProductionScratchDefaults(t *testing.T) {
	if productionCheckoutWalkStop != "" || productionGitScratchRoot != "/tmp" {
		t.Fatalf("production defaults: checkoutWalkStop=%q gitScratchRoot=%q", productionCheckoutWalkStop, productionGitScratchRoot)
	}
}

// Only the test binary sets the boundary; every fixture below TMPDIR remains
// subject to checkout and symlink checks. No tests mutate it after init.
func init() {
	checkoutWalkStop = filepath.Clean(os.TempDir())
	// A symlinked TMPDIR remains unsupported: bwrap cannot bind onto a symlink.
	// Use a real directory for TMPDIR when running these tests.
	gitScratchRoot = checkoutWalkStop
}

func TestTempAncestorIsolation(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	names := []string{"TestExecuteMergeBaseRanges", "TestNamespaceSetupFailureReportsIncomplete", "TestMissingFakeNeverFallsBack", "TestOfflineCacheMissIsIncomplete", "TestMachineDirectoryComponents"}
	cmd := exec.Command(os.Args[0], "-test.run=^("+strings.Join(names, "|")+")$", "-test.v")
	cmd.Env = append(os.Environ(), "TMPDIR="+parent)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tests below checkout ancestor: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "--- SKIP") {
		t.Fatalf("ancestor-isolation child skipped coverage:\n%s", out)
	}
	for _, name := range names {
		if !strings.Contains(string(out), "--- PASS: "+name+" (") {
			t.Fatalf("ancestor-isolation child did not pass %s:\n%s", name, out)
		}
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
