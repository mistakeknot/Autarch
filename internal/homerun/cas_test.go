//go:build unix

package homerun

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
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

// Separate instances model independent clients sharing a store. A mutex on one
// CAS object would not protect either an existing-entry Put or a concurrent Read.
func TestCASConcurrentInstances(t *testing.T) {
	store := newCAS(t, os.Getuid())
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := &CAS{Root: store.Root, OwnerUID: store.OwnerUID, MaxBytes: store.MaxBytes}
			data := []byte(fmt.Sprintf("echo concurrent %d\n", i%4))
			sha := sum(data)
			for j := 0; j < 20; j++ {
				if got, err := c.Put(data, sha); err != nil || got != sha {
					t.Errorf("Put: digest %q, error %v", got, err)
					return
				}
				if got, err := c.Read(sha); err != nil || !bytes.Equal(got, data) {
					t.Errorf("Read: content %q, error %v", got, err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	ents, err := os.ReadDir(store.Root)
	if err != nil || len(ents) != 4 {
		t.Errorf("CAS root holds %d entries, want 4: %v", len(ents), err)
	}
}

// A second open file description models another process holding the directory lock.
func TestCASLockDeadline(t *testing.T) {
	for _, op := range []string{"Put", "Read", "Verify"} {
		t.Run(op, func(t *testing.T) {
			c := newCAS(t, os.Getuid())
			data := []byte("lock deadline")
			sha, err := c.Put(data, "")
			if err != nil {
				t.Fatal(err)
			}
			c.lockWait = 20 * time.Millisecond
			f, err := os.Open(c.Root)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			fd := int(f.Fd())
			if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
				t.Fatal(err)
			}
			// Release even on the old blocking implementation, so red never hangs the suite.
			released := make(chan struct{})
			release := time.AfterFunc(250*time.Millisecond, func() { syscall.Flock(fd, syscall.LOCK_UN); close(released) })
			defer func() {
				if !release.Stop() {
					<-released
				}
			}()
			start := time.Now()
			switch op {
			case "Put":
				_, err = c.Put(data, "")
			case "Read":
				_, err = c.Read(sha)
			case "Verify":
				err = c.Verify(sha)
			}
			if !errors.Is(err, ErrCAS) || !strings.Contains(fmt.Sprint(err), "lock deadline exceeded") || !strings.Contains(fmt.Sprint(err), c.Root) {
				t.Fatalf("want named lock deadline error, got %v (after %v)", err, time.Since(start))
			}
			if elapsed := time.Since(start); elapsed >= 250*time.Millisecond {
				t.Fatalf("lock waited %v", elapsed)
			}
			if entries, err := os.ReadDir(c.Root); err != nil || len(entries) != 1 {
				t.Fatalf("lock failure left temporary files: %v, %v", entries, err)
			}
		})
	}
}

func TestCASReleasedLock(t *testing.T) {
	for _, op := range []string{"Put", "Read"} {
		t.Run(op, func(t *testing.T) {
			c := newCAS(t, os.Getuid())
			data := []byte("released lock")
			sha, err := c.Put(data, "")
			if err != nil {
				t.Fatal(err)
			}
			c.lockWait = time.Second
			f, err := os.Open(c.Root)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			fd := int(f.Fd())
			if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			time.AfterFunc(20*time.Millisecond, func() { syscall.Flock(fd, syscall.LOCK_UN); close(done) })
			if op == "Put" {
				_, err = c.Put(data, "")
			} else {
				_, err = c.Read(sha)
			}
			<-done
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCASStaleTemp(t *testing.T) {
	for _, kind := range []string{"stale", "foreign-name", "malformed-temp", "leading-nondot", "hex-15", "hex-17", "hex-uppercase", "pid-letters", "trailing-newline", "outside", "three-links", "corrupt", "mode"} {
		t.Run(kind, func(t *testing.T) {
			c := newCAS(t, os.Getuid())
			data := []byte("stale publication")
			sha, err := c.Put(data, "")
			if err != nil {
				t.Fatal(err)
			}
			p, _ := c.Path(sha)
			leftover := filepath.Join(c.Root, ".tmp-123-0123456789abcdef")
			switch kind {
			case "leading-nondot":
				leftover = filepath.Join(c.Root, "xtmp-123-0123456789abcdef")
			case "hex-15":
				leftover = filepath.Join(c.Root, ".tmp-123-0123456789abcde")
			case "hex-17":
				leftover = filepath.Join(c.Root, ".tmp-123-0123456789abcdef0")
			case "hex-uppercase":
				leftover = filepath.Join(c.Root, ".tmp-123-0123456789abcdeF")
			case "pid-letters":
				leftover = filepath.Join(c.Root, ".tmp-12a-0123456789abcdef")
			case "trailing-newline":
				leftover = filepath.Join(c.Root, ".tmp-123-0123456789abcdef\n")
			}
			if kind == "malformed-temp" {
				leftover = filepath.Join(c.Root, ".tmp-crashed")
			}
			if kind == "foreign-name" {
				leftover = filepath.Join(c.Root, "foreign")
			}
			if kind == "outside" {
				leftover = filepath.Join(t.TempDir(), ".tmp-outside")
			}
			if err := os.Link(p, leftover); err != nil {
				t.Fatal(err)
			}
			if kind == "three-links" {
				if err := os.Link(p, filepath.Join(c.Root, ".tmp-another")); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "corrupt" || kind == "mode" {
				if err := os.Chmod(p, 0o644); err != nil {
					t.Fatal(err)
				}
				if kind == "corrupt" {
					if err := os.WriteFile(p, []byte("bad"), 0o644); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(p, 0o444); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := c.Read(sha); !errors.Is(err, ErrCAS) {
				t.Fatalf("Read accepted leftover: %v", err)
			}
			if err := c.Verify(sha); !errors.Is(err, ErrCAS) {
				t.Fatalf("Verify accepted leftover: %v", err)
			}
			if _, err := os.Lstat(leftover); err != nil {
				t.Fatalf("reader repaired leftover: %v", err)
			}
			_, err = c.Put(data, sha)
			if kind == "stale" {
				if err != nil {
					t.Fatalf("Put did not repair stale temp: %v", err)
				}
				if _, err := os.Lstat(leftover); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("temp remains: %v", err)
				}
				if err := c.Verify(sha); err != nil {
					t.Fatal(err)
				}
			} else {
				if !errors.Is(err, ErrCAS) {
					t.Errorf("Put accepted %s: %v", kind, err)
				}
				if kind == "foreign-name" && !strings.Contains(fmt.Sprint(err), leftover) {
					t.Errorf("error does not name leftover: %v", err)
				}
				if _, err := os.Lstat(leftover); err != nil {
					t.Fatalf("Put removed foreign/unsafe leftover: %v", err)
				}
			}
		})
	}
}

func TestCASRepairOwnerMismatch(t *testing.T) {
	c, sha, leftover := staleCASFixture(t)
	dir, err := c.lock(syscall.LOCK_EX)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	err = c.repairAndVerifyWith(sha, dir, func() int { return os.Geteuid() + 1 }, nil, nil)
	if !errors.Is(err, ErrCAS) || !strings.Contains(fmt.Sprint(err), leftover) {
		t.Fatalf("repair accepted effective-owner mismatch: %v", err)
	}
	if _, err := os.Lstat(leftover); err != nil {
		t.Fatalf("owner-mismatched alias was removed: %v", err)
	}
}

func TestCASRepairReverifies(t *testing.T) {
	c, sha, leftover := staleCASFixture(t)
	dir, err := c.lock(syscall.LOCK_EX)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	called := false
	err = c.repairAndVerifyWith(sha, dir, os.Geteuid, nil, func() {
		called = true
		if _, err := os.Lstat(leftover); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("hook ran before unlink: %v", err)
		}
		p, _ := c.Path(sha)
		if err := os.Chmod(p, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("corrupted after unlink"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, 0o444); err != nil {
			t.Fatal(err)
		}
	})
	if !called {
		t.Fatal("after-unlink hook was not reached")
	}
	if !errors.Is(err, ErrCAS) || !strings.Contains(fmt.Sprint(err), "hash") {
		t.Fatalf("repair did not re-verify corrupted content: %v", err)
	}
}

func TestCASLinkCountErrors(t *testing.T) {
	c, sha, _ := staleCASFixture(t)
	p, _ := c.Path(sha)
	if err := os.Link(p, filepath.Join(c.Root, "third-link")); err != nil {
		t.Fatal(err)
	}
	dir, err := c.lock(syscall.LOCK_EX)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	for _, tc := range []struct {
		name  string
		links []uint64
		want  string
	}{
		{"read", []uint64{1}, "must have exactly 1 link;"},
		{"repair", []uint64{1, 2}, "must have 1 or 2 links;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := c.readWithLinks(sha, tc.links...)
			if !errors.Is(err, ErrCAS) || !strings.Contains(fmt.Sprint(err), tc.want) {
				t.Fatalf("link-count error: %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCASRepairUsesValidatedStat(t *testing.T) {
	c, sha, originalAlias := staleCASFixture(t)
	dir, err := c.lock(syscall.LOCK_EX)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	p, _ := c.Path(sha)
	replacementAlias := filepath.Join(c.Root, ".tmp-456-fedcba9876543210")
	called := false
	err = c.repairAndVerifyWith(sha, dir, os.Geteuid, func() {
		called = true
		// Allocate the replacement while the original still exists, so its inode
		// cannot be reused. It has the same digest, mode and two-link topology.
		if err := os.WriteFile(replacementAlias, []byte("repair guards"), 0o444); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(replacementAlias, p); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(originalAlias); err != nil {
			t.Fatal(err)
		}
	}, nil)
	if !called {
		t.Fatal("before-scan hook was not reached")
	}
	want := fmt.Sprintf("entry %s changed or has a leftover hard link outside the store", p)
	if !errors.Is(err, ErrCAS) || !strings.Contains(fmt.Sprint(err), want) {
		t.Errorf("repair refusal: %v, want ErrCAS with %q", err, want)
	}
	for _, path := range []string{p, replacementAlias} {
		fi, err := os.Lstat(path)
		if err != nil {
			t.Errorf("repair removed replacement file: %v", err)
			continue
		}
		if fi.Sys().(*syscall.Stat_t).Nlink != 2 {
			t.Errorf("repair changed replacement link count: %s", path)
		}
		b, err := os.ReadFile(path)
		if err != nil || sum(b) != sha {
			t.Errorf("repair changed replacement content: %s: %v", path, err)
		}
	}
}

func staleCASFixture(t *testing.T) (*CAS, string, string) {
	t.Helper()
	c := newCAS(t, os.Getuid())
	sha, err := c.Put([]byte("repair guards"), "")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Path(sha)
	leftover := filepath.Join(c.Root, ".tmp-123-0123456789abcdef")
	if err := os.Link(p, leftover); err != nil {
		t.Fatal(err)
	}
	return c, sha, leftover
}

func TestCASReadWithLinksStat(t *testing.T) {
	c, sha, _ := staleCASFixture(t)
	dir, err := c.lock(syscall.LOCK_EX)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	p, _ := c.Path(sha)
	before, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	_, validated, err := c.readWithLinks(sha, 1, 2)
	if err != nil || validated == nil {
		t.Fatalf("validated stat: %v, %v", validated, err)
	}
	want := before.Sys().(*syscall.Stat_t)
	if validated.Dev != want.Dev || validated.Ino != want.Ino || validated.Nlink != 2 {
		t.Fatalf("stat does not identify the validated entry: %+v", validated)
	}
	// A later path replacement cannot turn the returned snapshot into metadata
	// for another inode, even if that replacement has the same digest and mode.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("repair guards"), 0o444); err != nil {
		t.Fatal(err)
	}
	replacement, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	next := replacement.Sys().(*syscall.Stat_t)
	if validated.Dev == next.Dev && validated.Ino == next.Ino {
		t.Fatal("stat followed the replacement path")
	}
	if _, st, err := c.readWithLinks(sha, 2); !errors.Is(err, ErrCAS) || st != nil {
		t.Fatalf("failed validation returned usable stat: %+v, %v", st, err)
	}
}
