//go:build unix

package homerun

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// ErrCAS is wrapped by every content-addressed-store failure.
var ErrCAS = errors.New("content store")

var casTempName = regexp.MustCompile(`^\.tmp-[0-9]+-[0-9a-f]{16}$`)

func casErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrCAS, fmt.Sprintf(format, a...))
}

// CAS is a directory of immutable files named by the sha256 of their content. Entries are mode 0444. The
// store is written by the verifier and read by the launcher; a launched script runs from this copy, never
// from the card's checkout.
type CAS struct {
	Root     string
	OwnerUID int           // required owner of the root and the entries; -1 skips the owner checks (tests)
	MaxBytes int64         // per-entry cap
	lockWait time.Duration // zero uses the production lock deadline
}

// Path returns the entry path for a strict 64-hex digest.
func (c *CAS) Path(sha string) (string, error) {
	if !reHex64.MatchString(sha) {
		return "", casErr("not a sha256 digest")
	}
	return filepath.Join(c.Root, sha), nil
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func (c *CAS) checkDir() error {
	if !filepath.IsAbs(c.Root) || filepath.Clean(c.Root) != c.Root {
		return casErr("root must be a clean absolute path")
	}
	// Every path component must be a real directory, not a symlink, and not writable by group/other.
	cur := string(filepath.Separator)
	parts := splitPath(c.Root)
	for i, p := range append([]string{""}, parts...) {
		if i > 0 {
			cur = filepath.Join(cur, p)
		}
		fi, err := os.Lstat(cur)
		if err != nil {
			return casErr("%v", err)
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
			return casErr("%s is not a plain directory", cur)
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return casErr("no ownership information")
		}
		if cur == c.Root {
			if fi.Mode().Perm()&0o022 != 0 {
				return casErr("store directory is group- or world-writable")
			}
			if c.OwnerUID >= 0 && int(st.Uid) != c.OwnerUID {
				return casErr("store directory has the wrong owner")
			}
		} else if c.OwnerUID >= 0 {
			// ancestors must be owned by the store owner or root, and not writable by others
			if (int(st.Uid) != c.OwnerUID && st.Uid != 0) || (fi.Mode().Perm()&0o022 != 0 && fi.Mode()&os.ModeSticky == 0) {
				return casErr("ancestor %s is not safe", cur)
			}
		}
	}
	return nil
}

func splitPath(p string) []string {
	var out []string
	for _, s := range strings.Split(p, string(filepath.Separator)) {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (c *CAS) max() int64 {
	if c.MaxBytes > 0 {
		return c.MaxBytes
	}
	return 256 << 10
}

// lock coordinates separate CAS instances and processes through the directory inode.
// Readers must not observe the two-link interval between publication and temp cleanup.
func (c *CAS) lock(how int) (*os.File, error) {
	if err := c.checkDir(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(c.Root, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, casErr("%v", err)
	}
	// Thirty seconds is deliberately far above normal holds: one directory fsync
	// plus a re-read (default cap 256 KiB). Repair adds a directory scan and
	// a second full read; publication and repair each sync the directory once.
	// It still bounds a stalled or hostile lock holder.
	wait := c.lockWait
	if wait <= 0 {
		wait = 30 * time.Second
	}
	deadline := time.Now().Add(wait)
	pause := time.Millisecond
	for {
		err := syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN && err != syscall.EINTR {
			f.Close()
			return nil, casErr("lock store %s: %v", c.Root, err)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			f.Close()
			return nil, casErr("lock deadline exceeded for store %s", c.Root)
		}
		time.Sleep(min(pause, remaining))
		pause = min(2*pause, 50*time.Millisecond)
	}
}

// Put stores data and returns its digest. When wantSHA is non-empty the data must hash to it. An existing
// entry is verified, never overwritten. The stored file is re-read and re-hashed before Put returns.
func (c *CAS) Put(data []byte, wantSHA string) (string, error) {
	if int64(len(data)) > c.max() {
		return "", casErr("content over the %d-byte cap", c.max())
	}
	got := sum(data)
	if wantSHA != "" && wantSHA != got {
		return "", casErr("content does not match the expected digest")
	}
	if err := c.checkDir(); err != nil {
		return "", err
	}
	dst, _ := c.Path(got)
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", casErr("%v", err)
	}
	tmp := filepath.Join(c.Root, fmt.Sprintf(".tmp-%d-%s", os.Getpid(), hex.EncodeToString(rnd[:])))
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return "", casErr("%v", err)
	}
	cleanup := func() { os.Remove(tmp) }
	if _, err := f.Write(data); err != nil {
		f.Close()
		cleanup()
		return "", casErr("%v", err)
	}
	if err := f.Chmod(0o444); err != nil {
		f.Close()
		cleanup()
		return "", casErr("%v", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		cleanup()
		return "", casErr("%v", err)
	}
	f.Close()
	defer cleanup()
	// Prepare the unique temp before locking. The exclusive lock covers existing
	// entry repair/verification, publication, unlink, directory sync and re-read.
	lock, err := c.lock(syscall.LOCK_EX)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if _, err := os.Lstat(dst); err == nil {
		if err := c.repairAndVerify(got, lock); err != nil {
			return "", err
		}
		return got, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", casErr("%v", err)
	}
	// link fails if dst appeared meanwhile, so a concurrent writer of other content cannot be overwritten.
	if err := os.Link(tmp, dst); err != nil && !errors.Is(err, os.ErrExist) {
		cleanup()
		return "", casErr("%v", err)
	}
	if err := os.Remove(tmp); err != nil {
		return "", casErr("remove publication temp %s: %v", tmp, err)
	}
	if err := lock.Sync(); err != nil {
		return "", casErr("sync store: %v", err)
	}
	if _, err := c.read(got); err != nil {
		return "", err
	}
	return got, nil
}

// repairAndVerify requires the exclusive directory lock. Only a validated entry
// with exactly two links and an owned, same-directory .tmp-* alias is repairable.
func (c *CAS) repairAndVerify(sha string, dir *os.File) error {
	return c.repairAndVerifyWith(sha, dir, os.Geteuid, nil)
}

// repairAndVerifyWith provides private test injection without configurable hooks
// on CAS. Put always uses the real effective UID and no after-unlink callback.
func (c *CAS) repairAndVerifyWith(sha string, dir *os.File, effectiveUID func() int, afterUnlink func()) error {
	p, _ := c.Path(sha)
	// Match aliases only against metadata from the same fd whose bytes, mode,
	// ownership and link count were validated. One link needs no repair.
	_, st, err := c.readWithLinks(sha, 1, 2)
	if err != nil {
		return err
	}
	if st.Nlink == 1 {
		return nil
	}
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return casErr("find leftover for %s: %v", p, err)
	}
	for _, entry := range entries {
		if entry.Name() == sha {
			continue
		}
		leftover := filepath.Join(c.Root, entry.Name())
		name := entry.Name()
		var alias unix.Stat_t
		if err := unix.Fstatat(int(dir.Fd()), name, &alias, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return casErr("check leftover %s: %v", leftover, err)
		}
		if uint64(alias.Dev) != uint64(st.Dev) || uint64(alias.Ino) != uint64(st.Ino) {
			continue
		}
		if !casTempName.MatchString(name) || int(alias.Uid) != effectiveUID() || alias.Mode&unix.S_IFMT != unix.S_IFREG || alias.Nlink != 2 {
			return casErr("entry %s has unsafe leftover %s", p, leftover)
		}
		// Resolve the alias check and unlink against the locked directory inode.
		// An owner bypassing the flock can still change it between these calls;
		// the full re-read below refuses any resulting entry-integrity failure.
		if err := unix.Unlinkat(int(dir.Fd()), name, 0); err != nil {
			return casErr("remove leftover %s: %v", leftover, err)
		}
		if afterUnlink != nil {
			afterUnlink()
		}
		if err := dir.Sync(); err != nil {
			return casErr("sync after removing leftover %s: %v", leftover, err)
		}
		_, err = c.read(sha)
		return err
	}
	return casErr("entry %s has an unidentified leftover hard link outside the store", p)
}

// Verify checks an entry is exactly what its name says: a regular file reached without symlinks, mode 0444,
// one link, owned as required, within the size cap, and hashing to its name.
func (c *CAS) Verify(sha string) error {
	_, err := c.Read(sha)
	return err
}

// Read verifies and returns an entry's bytes.
func (c *CAS) Read(sha string) ([]byte, error) {
	lock, err := c.lock(syscall.LOCK_SH)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	return c.read(sha)
}

// read requires the caller to hold the store lock (shared or exclusive).
func (c *CAS) read(sha string) ([]byte, error) {
	b, _, err := c.readWithLinks(sha, 1)
	return b, err
}

// readWithLinks permits two links only for validation before exclusive-lock repair.
func (c *CAS) readWithLinks(sha string, links ...uint64) ([]byte, *syscall.Stat_t, error) {
	p, err := c.Path(sha)
	if err != nil {
		return nil, nil, err
	}
	if err := c.checkDir(); err != nil {
		return nil, nil, err
	}
	lfi, err := os.Lstat(p)
	if err != nil {
		return nil, nil, casErr("%v", err)
	}
	if !lfi.Mode().IsRegular() {
		return nil, nil, casErr("entry is not a regular file")
	}
	f, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, casErr("%v", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, nil, casErr("%v", err)
	}
	if !os.SameFile(lfi, fi) {
		return nil, nil, casErr("entry changed while being opened")
	}
	if fi.Mode().Perm() != 0o444 || fi.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, nil, casErr("entry mode is %v, want 0444", fi.Mode())
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || !slices.Contains(links, uint64(st.Nlink)) {
		return nil, nil, casErr("entry %s must have one of %v link(s); leftover hard link refused", p, links)
	}
	if c.OwnerUID >= 0 && int(st.Uid) != c.OwnerUID {
		return nil, nil, casErr("entry has the wrong owner")
	}
	if fi.Size() > c.max() {
		return nil, nil, casErr("entry over the size cap")
	}
	b, err := io.ReadAll(io.LimitReader(f, c.max()+1))
	if err != nil {
		return nil, nil, casErr("%v", err)
	}
	if int64(len(b)) > c.max() || sum(b) != sha {
		return nil, nil, casErr("entry content does not hash to its name")
	}
	return b, st, nil
}

// SnapshotSource reads a source file once, refusing symlinks and non-regular files, and returns its bytes.
// When wantSHA is non-empty the bytes must hash to it: a source swapped after signing is refused and
// nothing is stored.
func SnapshotSource(path, wantSHA string, max int64) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, casErr("source path must be absolute")
	}
	lfi, err := os.Lstat(path)
	if err != nil {
		return nil, casErr("%v", err)
	}
	if !lfi.Mode().IsRegular() {
		return nil, casErr("source is not a regular file (symlinks are refused)")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, casErr("%v", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, casErr("%v", err)
	}
	if !fi.Mode().IsRegular() || !os.SameFile(lfi, fi) {
		return nil, casErr("source changed while being opened")
	}
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, casErr("%v", err)
	}
	if int64(len(b)) > max {
		return nil, casErr("source over the %d-byte cap", max)
	}
	if wantSHA != "" && sum(b) != wantSHA {
		return nil, casErr("source content differs from the signed digest")
	}
	return b, nil
}
