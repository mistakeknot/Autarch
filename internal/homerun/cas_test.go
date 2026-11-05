//go:build unix

package homerun

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// T2.1 (CAS parts) — what runs is exactly what was signed.

func newCAS(t *testing.T, owner int) *CAS {
	t.Helper()
	parent := t.TempDir()
	// the sandbox umask can leave temp directories group-writable, which the store rightly refuses
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "cas")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	os.Chmod(root, 0o755)
	return &CAS{Root: root, OwnerUID: owner, MaxBytes: 1024}
}

func TestCASPutVerifyRead(t *testing.T) {
	for _, owner := range []int{-1, os.Getuid()} {
		c := newCAS(t, owner)
		data := []byte("#!/bin/bash\necho hi\n")
		want := sum(data)
		got, err := c.Put(data, want)
		if err != nil || got != want {
			t.Fatalf("Put: %q %v", got, err)
		}
		p, _ := c.Path(want)
		fi, _ := os.Lstat(p)
		if fi.Mode().Perm() != 0o444 {
			t.Errorf("entry mode %v", fi.Mode())
		}
		b, err := c.Read(want)
		if err != nil || !bytes.Equal(b, data) {
			t.Fatalf("Read: %v", err)
		}
		// idempotent, and no temp files left behind
		if _, err := c.Put(data, ""); err != nil {
			t.Fatal(err)
		}
		ents, _ := os.ReadDir(c.Root)
		if len(ents) != 1 {
			t.Errorf("CAS root holds %d entries", len(ents))
		}
	}
}

func TestCASPutRefusals(t *testing.T) {
	c := newCAS(t, -1)
	data := []byte("echo hi\n")
	if _, err := c.Put(data, sha1Hex); !errors.Is(err, ErrCAS) {
		t.Errorf("wrong digest accepted: %v", err)
	}
	if _, err := c.Put(bytes.Repeat([]byte("x"), 1025), ""); !errors.Is(err, ErrCAS) {
		t.Errorf("content over the cap accepted: %v", err)
	}
	for _, bad := range []string{"", "../etc/passwd", strings.Repeat("A", 64), sha1Hex[:63], sha1Hex + "0"} {
		if _, err := c.Path(bad); err == nil {
			t.Errorf("Path accepted %q", bad)
		}
	}
	if ents, _ := os.ReadDir(c.Root); len(ents) != 0 {
		t.Errorf("a refused Put left %d files", len(ents))
	}
}

func TestCASTamperDetection(t *testing.T) {
	data := []byte("echo hi\n")
	sha := sum(data)
	setup := func(t *testing.T) (*CAS, string) {
		c := newCAS(t, os.Getuid())
		if _, err := c.Put(data, sha); err != nil {
			t.Fatal(err)
		}
		p, _ := c.Path(sha)
		return c, p
	}
	t.Run("mode changed", func(t *testing.T) {
		c, p := setup(t)
		os.Chmod(p, 0o644)
		if err := c.Verify(sha); !errors.Is(err, ErrCAS) {
			t.Fatal(err)
		}
	})
	t.Run("content rewritten", func(t *testing.T) {
		c, p := setup(t)
		os.Chmod(p, 0o644)
		os.WriteFile(p, []byte("rm -rf /\n"), 0o644)
		os.Chmod(p, 0o444)
		if err := c.Verify(sha); !errors.Is(err, ErrCAS) {
			t.Fatal(err)
		}
	})
	t.Run("hard link", func(t *testing.T) {
		c, p := setup(t)
		if err := os.Link(p, filepath.Join(t.TempDir(), "other")); err != nil {
			t.Skip(err)
		}
		if err := c.Verify(sha); !errors.Is(err, ErrCAS) {
			t.Fatal(err)
		}
	})
	t.Run("symlink entry", func(t *testing.T) {
		c, p := setup(t)
		real := filepath.Join(t.TempDir(), "real")
		os.WriteFile(real, data, 0o444)
		os.Remove(p)
		os.Symlink(real, p)
		if err := c.Verify(sha); !errors.Is(err, ErrCAS) {
			t.Fatal(err)
		}
		if _, err := c.Put(data, sha); !errors.Is(err, ErrCAS) {
			t.Fatalf("Put trusted a symlink entry: %v", err)
		}
	})
	t.Run("corrupt existing entry is not overwritten", func(t *testing.T) {
		c, p := setup(t)
		os.Chmod(p, 0o644)
		os.WriteFile(p, []byte("x"), 0o644)
		os.Chmod(p, 0o444)
		if _, err := c.Put(data, sha); !errors.Is(err, ErrCAS) {
			t.Fatalf("Put accepted a corrupt entry: %v", err)
		}
		if b, _ := os.ReadFile(p); string(b) != "x" {
			t.Error("Put overwrote a corrupt entry instead of refusing")
		}
	})
	t.Run("directory group-writable", func(t *testing.T) {
		c, _ := setup(t)
		os.Chmod(c.Root, 0o775)
		if err := c.Verify(sha); !errors.Is(err, ErrCAS) {
			t.Fatal(err)
		}
		if _, err := c.Put([]byte("another"), ""); !errors.Is(err, ErrCAS) {
			t.Fatalf("Put into an unsafe directory: %v", err)
		}
	})
	t.Run("root is a symlink", func(t *testing.T) {
		c, _ := setup(t)
		link := filepath.Join(t.TempDir(), "link")
		os.Symlink(c.Root, link)
		c2 := &CAS{Root: link, OwnerUID: os.Getuid(), MaxBytes: 1024}
		if err := c2.Verify(sha); !errors.Is(err, ErrCAS) {
			t.Fatal(err)
		}
	})
	t.Run("relative root", func(t *testing.T) {
		c := &CAS{Root: "cas", OwnerUID: -1}
		if _, err := c.Put(data, sha); !errors.Is(err, ErrCAS) {
			t.Fatal(err)
		}
	})
	t.Run("wrong owner", func(t *testing.T) {
		c, _ := setup(t)
		c.OwnerUID = os.Getuid() + 1
		if err := c.Verify(sha); !errors.Is(err, ErrCAS) {
			t.Fatal(err)
		}
	})
}

func TestSnapshotSourceThenSourceChanges(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "run.sh")
	data := []byte("#!/bin/bash\necho signed\n")
	os.WriteFile(src, data, 0o755)
	sha := sum(data)

	b, err := SnapshotSource(src, sha, 1024)
	if err != nil {
		t.Fatal(err)
	}
	c := newCAS(t, os.Getuid())
	if _, err := c.Put(b, sha); err != nil {
		t.Fatal(err)
	}
	// after the snapshot: edit, chmod, rename the source and its parent; the stored bytes stay put
	os.WriteFile(src, []byte("echo evil\n"), 0o755)
	os.Chmod(src, 0o777)
	os.Rename(d, d+"-moved")
	got, err := c.Read(sha)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("stored bytes changed with the source: %v", err)
	}

	// a source swapped between signing and snapshot is refused and nothing is stored
	d2 := t.TempDir()
	src2 := filepath.Join(d2, "run.sh")
	os.WriteFile(src2, []byte("echo swapped\n"), 0o755)
	if _, err := SnapshotSource(src2, sha, 1024); !errors.Is(err, ErrCAS) {
		t.Fatalf("swapped source accepted: %v", err)
	}
	// a symlink source, a directory, a relative path and an oversized file
	link := filepath.Join(d2, "link.sh")
	os.Symlink(src2, link)
	if _, err := SnapshotSource(link, "", 1024); !errors.Is(err, ErrCAS) {
		t.Errorf("symlink source accepted: %v", err)
	}
	if _, err := SnapshotSource(d2, "", 1024); !errors.Is(err, ErrCAS) {
		t.Errorf("directory source accepted: %v", err)
	}
	if _, err := SnapshotSource("run.sh", "", 1024); !errors.Is(err, ErrCAS) {
		t.Errorf("relative source accepted: %v", err)
	}
	big := filepath.Join(d2, "big.sh")
	os.WriteFile(big, bytes.Repeat([]byte("x"), 2000), 0o755)
	if _, err := SnapshotSource(big, "", 1024); !errors.Is(err, ErrCAS) {
		t.Errorf("oversized source accepted: %v", err)
	}
	// a dependency swapped after signing is refused the same way
	dep := filepath.Join(d2, "dep.sh")
	os.WriteFile(dep, []byte("dep v1\n"), 0o644)
	depSHA := sum([]byte("dep v1\n"))
	os.WriteFile(dep, []byte("dep v2\n"), 0o644)
	if _, err := SnapshotSource(dep, depSHA, 1024); !errors.Is(err, ErrCAS) {
		t.Errorf("swapped dependency accepted: %v", err)
	}
}

func TestCASConcurrentPut(t *testing.T) {
	c := newCAS(t, os.Getuid())
	data := []byte("echo concurrent\n")
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Put(data, sum(data))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if ents, _ := os.ReadDir(c.Root); len(ents) != 1 {
		t.Errorf("CAS root holds %d entries", len(ents))
	}
}
