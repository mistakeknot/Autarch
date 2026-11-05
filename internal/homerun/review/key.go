package review

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// FileMeta is what the key-file check needs to know about a path.
type FileMeta struct {
	Mode fs.FileMode
	UID  uint32
}

// KeyOptions configures LoadKey. Every field is injectable so tests need no real ownership.
type KeyOptions struct {
	// SelfUID is the uid the service runs as. A key file may be owned by it or by 0.
	SelfUID uint32
	// Lstat reports the metadata of a path without following a final symlink. Nil uses the OS.
	Lstat func(path string) (FileMeta, error)
}

func osLstat(path string) (FileMeta, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return FileMeta{}, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return FileMeta{}, errors.New("no ownership information on this platform")
	}
	return FileMeta{Mode: fi.Mode(), UID: st.Uid}, nil
}

func (o KeyOptions) trusted(uid uint32) bool { return uid == o.SelfUID || uid == 0 }

// checkDirMeta applies the directory rule: a real directory owned by the service uid or root, not
// writable by group or other unless it is sticky and owned by root (as /tmp is).
func (o KeyOptions) checkDirMeta(m FileMeta) error {
	switch {
	case m.Mode&fs.ModeSymlink != 0 || !m.Mode.IsDir():
		return errors.New("path component is not a real directory")
	case !o.trusted(m.UID):
		return errors.New("path component is owned by neither the service user nor root")
	case m.Mode.Perm()&0o022 != 0 && !(m.Mode&fs.ModeSticky != 0 && m.UID == 0):
		return fmt.Errorf("directory mode %04o is writable by group or other", m.Mode.Perm())
	}
	return nil
}

// checkFileMeta applies the key-file rule.
func (o KeyOptions) checkFileMeta(m FileMeta) error {
	switch {
	case m.Mode&fs.ModeSymlink != 0:
		return errors.New("key file is a symbolic link")
	case !m.Mode.IsRegular():
		return errors.New("key file is not a regular file")
	case m.Mode.Perm()&0o077 != 0:
		return fmt.Errorf("key file mode %04o allows group or other access", m.Mode.Perm())
	case !o.trusted(m.UID):
		return errors.New("key file is owned by neither the service user nor root")
	}
	return nil
}

// CheckKeyPath applies the key-file and directory-chain policy to metadata supplied by o.Lstat
// (path-based, so it can race; LoadKey does not use it). It is the policy under test with injected
// ownership: the file, and every ancestor up to /, must pass.
func CheckKeyPath(path string, o KeyOptions) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return errors.New("key path must be absolute and clean")
	}
	ls := o.Lstat
	if ls == nil {
		ls = osLstat
	}
	fm, err := ls(path)
	if err != nil {
		return fmt.Errorf("key file: %w", err)
	}
	if err := o.checkFileMeta(fm); err != nil {
		return err
	}
	for d := filepath.Dir(path); ; d = filepath.Dir(d) {
		dm, err := ls(d)
		if err != nil {
			return fmt.Errorf("key directory chain: %w", err)
		}
		if err := o.checkDirMeta(dm); err != nil {
			return fmt.Errorf("key directory chain: %w", err)
		}
		if d == "/" {
			return nil
		}
	}
}

func fdMeta(f *os.File) (FileMeta, error) {
	fi, err := f.Stat()
	if err != nil {
		return FileMeta{}, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return FileMeta{}, errors.New("no ownership information on this platform")
	}
	return FileMeta{Mode: fi.Mode(), UID: st.Uid}, nil
}

// openChain opens path component by component from "/" with O_NOFOLLOW, checking each directory
// from its open descriptor before descending, and checks the final file from its descriptor. Nothing
// is decided from a path after an open, so swapping any component (a symlink included) cannot make
// a different file pass the check: a symlink component fails to open, and a replaced component
// is the one whose descriptor was checked.
func openChain(path string, o KeyOptions) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return nil, errors.New("key path must be absolute and clean")
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// O_NONBLOCK keeps the open of a FIFO or device from blocking; the final fstat then refuses
	// anything that is not a regular file.
	const base = syscall.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_CLOEXEC | syscall.O_NONBLOCK
	fd, err := syscall.Open("/", base|syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, errors.New("key path cannot be opened")
	}
	cur := os.NewFile(uintptr(fd), "/")
	for i, name := range parts {
		last := i == len(parts)-1
		m, err := fdMeta(cur)
		if err == nil {
			err = o.checkDirMeta(m)
		}
		if err != nil {
			cur.Close()
			return nil, fmt.Errorf("key directory chain: %w", err)
		}
		flags := base
		if !last {
			flags |= syscall.O_DIRECTORY
		}
		nfd, err := syscall.Openat(int(cur.Fd()), name, flags, 0)
		cur.Close()
		if err != nil {
			return nil, errors.New("key path component cannot be opened without following links")
		}
		cur = os.NewFile(uintptr(nfd), name)
	}
	m, err := fdMeta(cur)
	if err == nil {
		err = o.checkFileMeta(m)
	}
	if err != nil {
		cur.Close()
		return nil, err
	}
	return cur, nil
}

// fileSigner holds a private key. It prints as redacted whatever verb formats it.
type fileSigner struct{ key ed25519.PrivateKey }

func (f *fileSigner) Sign(msg []byte) ([]byte, error) { return ed25519.Sign(f.key, msg), nil }
func (f *fileSigner) PublicKey() ed25519.PublicKey    { return f.key.Public().(ed25519.PublicKey) }
func (f *fileSigner) String() string                  { return "review-signer(redacted)" }
func (f *fileSigner) GoString() string                { return f.String() }

// LoadKey opens the key through a trusted directory chain (see openChain), checks owner and mode on
// the open descriptor, then reads a 64-hex-character Ed25519 seed from that same descriptor and
// returns a Signer. o.Lstat is not used. No error contains key bytes.
func LoadKey(path string, o KeyOptions) (Signer, error) {
	f, err := openChain(path, o)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 256))
	if err != nil {
		return nil, errors.New("key file cannot be read")
	}
	txt := strings.TrimSpace(string(raw))
	seed, err := hex.DecodeString(txt)
	if !reHex64.MatchString(txt) {
		err = errors.New("not lower-case hex")
	}
	for i := range raw {
		raw[i] = 0
	}
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("key file is not a 64-character hex Ed25519 seed")
	}
	k := ed25519.NewKeyFromSeed(seed)
	for i := range seed {
		seed[i] = 0
	}
	return &fileSigner{key: k}, nil
}
