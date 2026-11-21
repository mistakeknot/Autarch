package callerlog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuditDirectoryRepairsOwnMode(t *testing.T) {
	for _, mode := range []os.FileMode{0755, 0775} {
		t.Run(mode.String(), func(t *testing.T) {
			parent := t.TempDir()
			if err := os.Chmod(parent, 0755); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(parent, "audit")
			if err := os.Mkdir(dir, mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatal(err)
			}
			l := New(dir)
			l.Record(map[string]any{"probe": "private"}, "")
			l.Close()
			if l.Dropped() != 0 {
				t.Fatalf("dropped %d", l.Dropped())
			}
			data, err := os.ReadFile(filepath.Join(dir, "caller-log.jsonl"))
			if err != nil || !bytes.Contains(data, []byte("private")) {
				t.Fatalf("log: %s %v", data, err)
			}
			fi, _ := os.Stat(dir)
			if fi.Mode().Perm() != 0700 {
				t.Fatal(fi.Mode())
			}
			fi, _ = os.Stat(parent)
			if fi.Mode().Perm() != 0755 {
				t.Fatal("changed parent mode")
			}
		})
	}
}
func TestSymlinkAuditDirectoryRefused(t *testing.T) {
	parent := t.TempDir()
	target := t.TempDir()
	if err := os.Chmod(target, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "audit")
	if err := os.Symlink(target, dir); err != nil {
		t.Fatal(err)
	}
	l := New(dir)
	l.Record(map[string]any{}, "")
	l.Close()
	if l.Dropped() != 1 {
		t.Fatal("accepted symlink directory")
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatalf("target changed: %v %v", entries, err)
	}
}
func TestStorageDropWarnsOnce(t *testing.T) {
	for _, kind := range []string{"directory", "file"} {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "audit")
			if kind == "directory" {
				if err := os.WriteFile(dir, nil, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(dir, "caller-log.jsonl")); err != nil {
					t.Fatal(err)
				}
			}
			stderr, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer stderr.Close()
			old := os.Stderr
			os.Stderr = stderr
			defer func() { os.Stderr = old }()
			l := New(dir)
			for i := 0; i < 3; i++ {
				l.Record(map[string]any{"secret": "do-not-echo"}, "")
			}
			l.Close()
			data, err := os.ReadFile(stderr.Name())
			if err != nil {
				t.Fatal(err)
			}
			if l.Dropped() != 3 || strings.Count(string(data), "\n") != 1 || strings.Contains(string(data), "do-not-echo") || strings.Contains(string(data), dir) {
				t.Fatalf("drops=%d warning=%q", l.Dropped(), data)
			}
		})
	}
}
func TestDefaultProductionAuditPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// Resolve caches and Go configuration before isolating the child HOME.
	// Reuse dependencies already needed to build this test, including offline.
	out, err := exec.CommandContext(ctx, "go", "env", "-json", "GOMODCACHE", "GOCACHE", "GOENV").CombinedOutput()
	if err != nil {
		t.Fatalf("go env: %v %s", err, out)
	}
	var caches struct{ GOMODCACHE, GOCACHE, GOENV string }
	if err := json.Unmarshal(out, &caches); err != nil {
		t.Fatal(err)
	}
	if caches.GOMODCACHE == "" || caches.GOCACHE == "" {
		t.Fatal("go env returned an empty cache path")
	}
	home := t.TempDir()
	parent := filepath.Join(home, ".autarch")
	if err := os.Mkdir(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(src, []byte(`package main
import "github.com/mistakeknot/autarch/pkg/callerlog"
func main(){l:=callerlog.Default();l.Record(map[string]any{"probe":"production-default"},"");l.Close()}
`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "go", "run", "-mod=readonly", src)
	cmd.Env = append(os.Environ(), "HOME="+home, "GOMODCACHE="+caches.GOMODCACHE, "GOCACHE="+caches.GOCACHE)
	// Preserve go env -w settings even though HOME now points at the fixture.
	if caches.GOENV != "" && caches.GOENV != "off" {
		cmd.Env = append(cmd.Env, "GOENV="+caches.GOENV)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("probe: %v %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(parent, "audit", "caller-log.jsonl"))
	if err != nil || !bytes.Contains(data, []byte("production-default")) {
		t.Fatalf("default log: %s %v", data, err)
	}
	fi, _ := os.Stat(parent)
	if fi.Mode().Perm() != 0755 {
		t.Fatal("changed parent")
	}
}

func TestDefaultSizeCap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "caller-log.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Chmod(0644); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(8 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	l := New(dir)
	l.Record(map[string]any{}, "")
	l.Close()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 8<<20 || l.Dropped() != 1 || fi.Mode().Perm() != 0600 {
		t.Fatalf("size=%d drops=%d mode=%04o", fi.Size(), l.Dropped(), fi.Mode().Perm())
	}
}

func TestInjectedSizeCap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	old := os.Stderr
	os.Stderr = stderr
	defer func() { os.Stderr = old }()
	l := New(dir, WithMaxBytes(256))
	for i := 0; i < 20; i++ {
		l.Record(map[string]any{"probe": strings.Repeat("x", 40)}, "")
	}
	l.Close()
	path := filepath.Join(dir, "caller-log.jsonl")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Count(string(data), "\n")
	warning, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	if lines == 0 || fi.Size() > 256 || uint64(lines)+l.Dropped() != 20 || strings.Count(string(warning), "\n") != 1 {
		t.Fatalf("size=%d lines=%d drops=%d warning=%q", fi.Size(), lines, l.Dropped(), warning)
	}
	// A fresh logger must also respect existing file contents.
	before := fi.Size()
	again := New(dir, WithMaxBytes(before))
	again.Record(map[string]any{}, "")
	again.Close()
	fi, _ = os.Stat(path)
	if fi.Size() != before || again.Dropped() != 1 {
		t.Fatal("existing cap ignored")
	}
}
func TestAuditDirectoryOwnershipCheck(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	if err := os.Mkdir(dir, 0775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0775); err != nil {
		t.Fatal(err)
	}
	// Supply another expected UID to exercise foreign ownership without privileges.
	f, err := openAuditDir(dir, uint32(os.Geteuid()+1))
	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("accepted directory belonging to another UID")
	}
	fi, _ := os.Stat(dir)
	if fi.Mode().Perm() != 0775 {
		t.Fatal("changed foreign directory")
	}
}
func TestInaccessibleAuditDirectoryRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can open unreadable directories")
	}
	dir := filepath.Join(t.TempDir(), "audit")
	if err := os.Mkdir(dir, 0311); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0311); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)
	l := New(dir)
	l.Record(map[string]any{}, "")
	l.Close()
	if l.Dropped() != 1 {
		t.Fatal("accepted inaccessible directory")
	}
}

func TestSharedFileSizeCap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	a, b := New(dir, WithMaxBytes(512)), New(dir, WithMaxBytes(512))
	for i := 0; i < 100; i++ {
		a.Record(map[string]any{"writer": "a"}, "")
		b.Record(map[string]any{"writer": "b"}, "")
	}
	a.Close()
	b.Close()
	data, err := os.ReadFile(filepath.Join(dir, "caller-log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 512 || uint64(strings.Count(string(data), "\n"))+a.Dropped()+b.Dropped() != 200 {
		t.Fatalf("bytes=%d drops=%d", len(data), a.Dropped()+b.Dropped())
	}
}
