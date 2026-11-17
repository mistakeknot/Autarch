package callerlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQueueOverflowNeverWaits(t *testing.T) {
	// No worker: model a disk operation that never completes.
	l := &Log{queue: make(chan item, 1)}
	l.Record(map[string]any{}, "")
	done := make(chan struct{})
	go func() { l.Record(map[string]any{}, ""); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request waited for writer")
	}
	if l.Dropped() != 1 {
		t.Fatal(l.Dropped())
	}
}
func TestWriteFailureCounted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	os.WriteFile(path, nil, 0600)
	l := New(path)
	l.Record(map[string]any{}, "")
	l.Close()
	if l.Dropped() != 1 {
		t.Fatal(l.Dropped())
	}
}
func TestPeerProcFixtures(t *testing.T) {
	for _, addr := range []string{"0100007F:ABCD", "00000000000000000000000001000000:ABCD"} {
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "net"), 0700)
		os.MkdirAll(filepath.Join(root, "123/fd"), 0700)
		table := "tcp"
		remote := "127.0.0.1:43981"
		if len(addr) > 20 {
			table = "tcp6"
			remote = "[::1]:43981"
		}
		os.WriteFile(filepath.Join(root, "net", table), []byte(" 0: "+addr+" 00000000:0000 01 0 0 0 1000 0 5678\n"), 0600)
		os.Symlink("socket:[5678]", filepath.Join(root, "123/fd/4"))
		os.WriteFile(filepath.Join(root, "123/comm"), []byte("probe\n"), 0600)
		os.Symlink("/fixture/probe", filepath.Join(root, "123/exe"))
		got := resolvePeerAt(root, remote)
		peer, ok := got.(map[string]any)
		if !ok || peer["pid"] != 123 || peer["comm"] != "probe" || peer["exe"] != "/fixture/probe" {
			t.Fatalf("%v", got)
		}
		if got := resolvePeerAt(root, "127.0.0.1:1"); got != "unknown" {
			t.Fatal(got)
		}
	}
}
