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

// CheckKeyPath refuses a key file, or its parent directory, that anyone but the service's own uid or
// root could read or change. Errors name the path and the problem, never contents.
func CheckKeyPath(path string, o KeyOptions) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("key path must be absolute and clean")
	}
	ls := o.Lstat
	if ls == nil {
		ls = osLstat
	}
	owner := func(m FileMeta) bool { return m.UID == o.SelfUID || m.UID == 0 }
	fm, err := ls(path)
	if err != nil {
		return fmt.Errorf("key file: %w", err)
	}
	switch {
	case fm.Mode&fs.ModeSymlink != 0:
		return errors.New("key file is a symbolic link")
	case !fm.Mode.IsRegular():
		return errors.New("key file is not a regular file")
	case fm.Mode.Perm()&0o077 != 0:
		return fmt.Errorf("key file mode %04o allows group or other access", fm.Mode.Perm())
	case !owner(fm):
		return errors.New("key file is owned by neither the service user nor root")
	}
	dm, err := ls(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("key directory: %w", err)
	}
	switch {
	case dm.Mode&fs.ModeSymlink != 0 || !dm.Mode.IsDir():
		return errors.New("key directory is not a real directory")
	case dm.Mode.Perm()&0o022 != 0:
		return fmt.Errorf("key directory mode %04o is writable by group or other", dm.Mode.Perm())
	case !owner(dm):
		return errors.New("key directory is owned by neither the service user nor root")
	}
	return nil
}

// fileSigner holds a private key. It prints as redacted whatever verb formats it.
type fileSigner struct{ key ed25519.PrivateKey }

func (f *fileSigner) Sign(msg []byte) ([]byte, error) { return ed25519.Sign(f.key, msg), nil }
func (f *fileSigner) PublicKey() ed25519.PublicKey    { return f.key.Public().(ed25519.PublicKey) }
func (f *fileSigner) String() string                  { return "review-signer(redacted)" }
func (f *fileSigner) GoString() string                { return f.String() }

// LoadKey checks the key file with CheckKeyPath, then reads a 64-hex-character Ed25519 seed from it
// and returns a Signer. The file is opened without following symlinks and must be the same file the
// check saw. No error contains key bytes.
func LoadKey(path string, o KeyOptions) (Signer, error) {
	if err := CheckKeyPath(path, o); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errors.New("key file cannot be opened")
	}
	defer f.Close()
	if o.Lstat == nil { // the real-file recheck only means something against the real OS
		fi, err := f.Stat()
		li, lerr := os.Lstat(path)
		if err != nil || lerr != nil || !os.SameFile(fi, li) {
			return nil, errors.New("key file changed while being opened")
		}
	}
	raw, err := io.ReadAll(io.LimitReader(f, 256))
	if err != nil {
		return nil, errors.New("key file cannot be read")
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
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
