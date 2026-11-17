package callerlog

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRejectUnsafeLogTargets(t *testing.T) {
	for _, mode := range []os.FileMode{0750, 0707} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatal(err)
			}
			l := New(dir)
			l.Record(map[string]any{}, "")
			l.Close()
			if l.Dropped() != 1 {
				t.Fatal("unsafe directory accepted")
			}
			if _, err := os.Stat(filepath.Join(dir, "caller-log.jsonl")); !os.IsNotExist(err) {
				t.Fatal("created log in unsafe directory")
			}
		})
	}
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
func TestDefaultNeverWritesInTests(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	l := Default()
	l.Record(map[string]any{}, "")
	l.Close()
	if _, err := os.Stat(filepath.Join(home, ".autarch", "caller-log.jsonl")); !os.IsNotExist(err) {
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
	got := resolvePeer(peer.RemoteAddr().String())
	p, ok := got.(map[string]any)
	if !ok || p["pid"] != os.Getpid() {
		t.Fatalf("peer = %v", got)
	}
}
