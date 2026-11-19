package homeask

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRepeatedOwnerAttentionDoesNotRewriteState(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	f.StateDir = t.TempDir()
	r := testReq()
	r.NonBlocking = true
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := f.recordOwnerAttention(r.Key, "project needs correction"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.StateDir, "filings.json")
	stamp := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.recordOwnerAttention(r.Key, "project needs correction"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("duplicate attention rewrote state")
	}
}
