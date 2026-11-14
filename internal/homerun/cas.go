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
	"strings"
	"syscall"
)

// ErrCAS is wrapped by every content-addressed-store failure.
var ErrCAS = errors.New("content store")

func casErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrCAS, fmt.Sprintf(format, a...))
}

// CAS is a directory of immutable files named by the sha256 of their content. Entries are mode 0444. The
// store is written by the verifier and read by the launcher; a launched script runs from this copy, never
// from the card's checkout.
type CAS struct {
	Root     string
	OwnerUID int   // required owner of the root and the entries; -1 skips the owner checks (tests)
	MaxBytes int64 // per-entry cap
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
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return nil, casErr("lock store: %v", err)
	}
	return f, nil
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
	lock, err := c.lock(syscall.LOCK_EX)
	if err != nil {
		return "", err
	}
	defer lock.Close() // closing releases the flock, including on errors
	dst, _ := c.Path(got)
	if _, err := os.Lstat(dst); err == nil {
		if _, err := c.read(got); err != nil {
			return "", err
		}
		return got, nil
	}
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
	// link fails if dst appeared meanwhile, so a concurrent writer of other content cannot be overwritten.
	if err := os.Link(tmp, dst); err != nil && !errors.Is(err, os.ErrExist) {
		cleanup()
		return "", casErr("%v", err)
	}
	cleanup()
	if d, err := os.Open(c.Root); err == nil {
		d.Sync()
		d.Close()
	}
	if _, err := c.read(got); err != nil {
		return "", err
	}
	return got, nil
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
	p, err := c.Path(sha)
	if err != nil {
		return nil, err
	}
	if err := c.checkDir(); err != nil {
		return nil, err
	}
	lfi, err := os.Lstat(p)
	if err != nil {
		return nil, casErr("%v", err)
	}
	if !lfi.Mode().IsRegular() {
		return nil, casErr("entry is not a regular file")
	}
	f, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, casErr("%v", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, casErr("%v", err)
	}
	if !os.SameFile(lfi, fi) {
		return nil, casErr("entry changed while being opened")
	}
	if fi.Mode().Perm() != 0o444 || fi.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, casErr("entry mode is %v, want 0444", fi.Mode())
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink != 1 {
		return nil, casErr("entry must have exactly one link")
	}
	if c.OwnerUID >= 0 && int(st.Uid) != c.OwnerUID {
		return nil, casErr("entry has the wrong owner")
	}
	if fi.Size() > c.max() {
		return nil, casErr("entry over the size cap")
	}
	b, err := io.ReadAll(io.LimitReader(f, c.max()+1))
	if err != nil {
		return nil, casErr("%v", err)
	}
	if int64(len(b)) > c.max() || sum(b) != sha {
		return nil, casErr("entry content does not hash to its name")
	}
	return b, nil
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
