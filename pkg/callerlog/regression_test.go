package callerlog

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRejectUnsafeLogTargets(t *testing.T) {

	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "target")
		os.WriteFile(target, []byte("unchanged"), 0644)
		if err := os.Symlink(target, filepath.Join(dir, "caller-log.jsonl")); err != nil {
			t.Fatal(err)
		}
		l := New(dir)
		l.Record(map[string]any{}, "")
		l.Close()
		data, _ := os.ReadFile(target)
		fi, _ := os.Stat(target)
		if string(data) != "unchanged" || fi.Mode().Perm() != 0644 || l.Dropped() != 1 {
			t.Fatal("followed symlink")
		}
	})
}
func TestDefaultUsesCachedDropOnlyLogger(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Inject the process default for this isolated HOME, just as consumers inject
	// a drop-only logger. The production default is covered by the child probe.
	dir := filepath.Join(home, ".autarch", "audit")
	defaults.Store(dir, New(""))
	t.Cleanup(func() { defaults.Delete(dir) })
	l := Default()
	l.Record(map[string]any{}, "")
	l.Close()
	if _, err := os.Stat(filepath.Join(home, ".autarch", "audit", "caller-log.jsonl")); !os.IsNotExist(err) {
		t.Fatal("Default wrote a test record under HOME")
	}
}
func TestMissingHomeDisablesDefault(t *testing.T) {
	t.Setenv("HOME", "")
	l := Default()
	if l.Dir() != "" {
		t.Fatalf("missing home selected directory %q", l.Dir())
	}
}
func TestPeerRealLoopback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("procfs requires Linux")
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	peer, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	// Resolution is deliberately best effort within a 20 ms budget. Keep the
	// socket alive for up to 15 seconds so host contention can subside without
	// changing the production per-attempt budget.
	var got any
	deadline := time.Now().Add(15 * time.Second)
	for {
		got = resolvePeer(peer.RemoteAddr().String())
		if got != "unknown" || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	p, ok := got.(map[string]any)
	if !ok || p["pid"] != os.Getpid() {
		if table, err := os.ReadFile("/proc/net/tcp"); err != nil {
			t.Logf("read /proc/net/tcp: %v", err)
		} else {
			lines := bytes.Count(table, []byte{'\n'})
			if len(table) > 0 && table[len(table)-1] != '\n' {
				lines++
			}
			t.Logf("/proc/net/tcp line count: %d (resolver limit: %d)", lines, peerTableLineLimit)
		}
		t.Fatalf("peer = %v", got)
	}
}
