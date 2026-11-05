//go:build unix && homerunlive

package review

// Live tests for LoadKey against a real tree with real ownership. They must run as root on a throwaway
// Linux machine (a VM) that has an account named runner. Keys are generated here, in the test, and
// exist only on that machine. Observations go to $HR_OUT/obs.txt as "OBS <test> <key> = <value>".

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"
)

var obsMu sync.Mutex

func obs(t *testing.T, key string, val any) {
	t.Helper()
	line := fmt.Sprintf("OBS %s %s = %v", t.Name(), key, val)
	t.Log(line)
	if out := os.Getenv("HR_OUT"); out != "" {
		obsMu.Lock()
		defer obsMu.Unlock()
		if f, err := os.OpenFile(filepath.Join(out, "obs.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintln(f, line)
			f.Close()
		}
	}
}

type keyTree struct {
	t    *testing.T
	base string
	pub  ed25519.PublicKey
	seed string
}

func runnerUID(t *testing.T) int {
	t.Helper()
	u, err := user.Lookup("runner")
	if err != nil {
		t.Skip("no runner account")
	}
	id, _ := strconv.Atoi(u.Uid)
	return id
}

// newKeyTree makes a root-owned tree under /srv/rk and returns it. Base is 0755 root.
func newKeyTree(t *testing.T) *keyTree {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	if err := os.MkdirAll("/srv/rk", 0o755); err != nil {
		t.Fatal(err)
	}
	base, err := os.MkdirTemp("/srv/rk", "t")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	must(t, os.Chmod(base, 0o755))
	seed := make([]byte, ed25519.SeedSize)
	rand.Read(seed)
	return &keyTree{t: t, base: base, seed: hex.EncodeToString(seed),
		pub: ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// key writes the key file at rel (creating root 0755 parents) with the mode and owner and returns its path.
func (k *keyTree) key(rel string, mode os.FileMode, uid int) string {
	k.t.Helper()
	p := filepath.Join(k.base, rel)
	must(k.t, os.MkdirAll(filepath.Dir(p), 0o755))
	must(k.t, os.WriteFile(p, []byte(k.seed+"\n"), 0o600))
	must(k.t, os.Chmod(p, mode))
	must(k.t, os.Chown(p, uid, 0))
	return p
}

func (k *keyTree) dir(rel string, mode os.FileMode, uid int) string {
	k.t.Helper()
	p := filepath.Join(k.base, rel)
	must(k.t, os.MkdirAll(p, 0o755))
	must(k.t, os.Chmod(p, mode))
	must(k.t, os.Chown(p, uid, 0))
	return p
}

func (k *keyTree) load(p string) error {
	s, err := LoadKey(p, KeyOptions{SelfUID: 0})
	if err == nil && !bytes.Equal(s.PublicKey(), k.pub) {
		k.t.Fatal("loaded a different key than the one written")
	}
	return err
}

func (k *keyTree) refuse(name, p string) {
	k.t.Helper()
	err := k.load(p)
	obs(k.t, name, fmt.Sprintf("refused=%v err=%v", err != nil, err))
	if err == nil {
		k.t.Fatalf("%s: accepted", name)
	}
}

func TestLiveKeyAccepted(t *testing.T) {
	k := newKeyTree(t)
	p := k.key("ok/key", 0o600, 0)
	err := k.load(p)
	obs(t, "root-0600-root-dirs", fmt.Sprintf("err=%v", err))
	if err != nil {
		t.Fatal(err)
	}
	// 0400 is also fine.
	must(t, os.Chmod(p, 0o400))
	if err := k.load(p); err != nil {
		t.Fatal(err)
	}
}

func TestLiveKeyWrongOwner(t *testing.T) {
	k := newKeyTree(t)
	k.refuse("file-owned-by-runner", k.key("a/key", 0o600, runnerUID(t)))
}

func TestLiveKeyOwnedBySelfUID(t *testing.T) {
	k := newKeyTree(t)
	uid := runnerUID(t)
	p := k.key("s/key", 0o600, uid)
	// The service uid may own the key, but a parent directory it owns must also be allowed: it is, as
	// SelfUID is trusted; only root-owned parents are used here so the chain passes.
	if _, err := LoadKey(p, KeyOptions{SelfUID: uint32(uid)}); err != nil {
		obs(t, "owned-by-self-uid", fmt.Sprintf("err=%v", err))
		t.Fatal(err)
	}
}

func TestLiveKeyModes(t *testing.T) {
	for _, m := range []os.FileMode{0o640, 0o604, 0o660, 0o606, 0o644, 0o666, 0o610, 0o601} {
		k := newKeyTree(t)
		k.refuse(fmt.Sprintf("mode-%04o", m), k.key("m/key", m, 0))
	}
}

func TestLiveKeyAncestorOwnedByRunner(t *testing.T) {
	k := newKeyTree(t)
	k.dir("o", 0o755, runnerUID(t))
	k.refuse("ancestor-owned-by-runner", k.key("o/key", 0o600, 0))
}

func TestLiveKeyWorldAndGroupWritableAncestor(t *testing.T) {
	for _, m := range []os.FileMode{0o777, 0o757, 0o775, 0o770 | 0o020, 0o722} {
		k := newKeyTree(t)
		k.dir("w", m, 0)
		k.refuse(fmt.Sprintf("ancestor-mode-%04o", m), k.key("w/key", 0o600, 0))
	}
}

func TestLiveKeyStickyRule(t *testing.T) {
	// Root-owned sticky world-writable ancestor (like /tmp): allowed by the rule.
	k := newKeyTree(t)
	k.dir("st", 0o777|os.ModeSticky, 0)
	p := k.key("st/key", 0o600, 0)
	err := k.load(p)
	obs(t, "root-sticky-1777-ancestor", fmt.Sprintf("err=%v", err))
	if err != nil {
		t.Fatal(err)
	}
	// Sticky but owned by a non-root, non-self user: refused.
	k2 := newKeyTree(t)
	k2.dir("st", 0o777|os.ModeSticky, runnerUID(t))
	k2.refuse("runner-owned-sticky-ancestor", k2.key("st/key", 0o600, 0))
	// Sticky bit with a merely group-writable root dir is still refused when not sticky.
	k3 := newKeyTree(t)
	k3.dir("g", 0o775, 0)
	k3.refuse("group-writable-nonsticky", k3.key("g/key", 0o600, 0))
}

func TestLiveKeySymlinkFinal(t *testing.T) {
	k := newKeyTree(t)
	real := k.key("real/key", 0o600, 0)
	link := filepath.Join(k.base, "link")
	must(t, os.Symlink(real, link))
	k.refuse("final-component-symlink", link)
}

func TestLiveKeySymlinkedAncestor(t *testing.T) {
	k := newKeyTree(t)
	k.key("real/sub/key", 0o600, 0)
	must(t, os.Symlink(filepath.Join(k.base, "real"), filepath.Join(k.base, "alias")))
	k.refuse("symlinked-ancestor", filepath.Join(k.base, "alias", "sub", "key"))
}

// A symlink swapped in for an ancestor while LoadKey runs must never yield the other tree's key.
func TestLiveKeyAncestorSwapRace(t *testing.T) {
	k := newKeyTree(t)
	good := k.key("d/key", 0o600, 0) // loads under d
	other := newKeyTree(t)
	// A decoy tree whose key has a different seed and is world-readable (would be refused if reached).
	decoy := other.key("x/key", 0o666, 0)
	_ = decoy
	swap := filepath.Join(k.base, "d")
	hold := filepath.Join(k.base, "d.held")
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if os.Rename(swap, hold) == nil {
				os.Symlink(filepath.Join(other.base, "x"), swap)
				os.Remove(swap)
				os.Rename(hold, swap)
			}
		}
	}()
	ok, refused := 0, 0
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		s, err := LoadKey(good, KeyOptions{SelfUID: 0})
		if err != nil {
			refused++
			continue
		}
		if !bytes.Equal(s.PublicKey(), k.pub) {
			close(stop)
			wg.Wait()
			t.Fatal("swap race produced the decoy key")
		}
		ok++
	}
	close(stop)
	wg.Wait()
	obs(t, "swap-race", fmt.Sprintf("loads_ok=%d refused=%d decoy_never_loaded=true", ok, refused))
	if ok == 0 && refused == 0 {
		t.Fatal("no iterations ran")
	}
}

func TestLiveKeyFIFONoHang(t *testing.T) {
	k := newKeyTree(t)
	p := filepath.Join(k.base, "fifo", "key")
	must(t, os.MkdirAll(filepath.Dir(p), 0o755))
	must(t, syscall.Mkfifo(p, 0o600))
	done := make(chan error, 1)
	go func() { done <- k.load(p) }()
	select {
	case err := <-done:
		obs(t, "fifo-at-key-path", fmt.Sprintf("refused=%v err=%v", err != nil, err))
		if err == nil {
			t.Fatal("accepted a FIFO")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("LoadKey hung on a FIFO")
	}
	// Also with a writer-less FIFO that is a directory component's sibling: a FIFO as an ancestor.
	fa := filepath.Join(k.base, "fifoanc")
	must(t, syscall.Mkfifo(fa, 0o600))
	go func() { done <- k.load(filepath.Join(fa, "key")) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("accepted a FIFO ancestor")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("LoadKey hung on a FIFO ancestor")
	}
}

func TestLiveKeyNotHexAndDevice(t *testing.T) {
	k := newKeyTree(t)
	p := k.key("bad/key", 0o600, 0)
	must(t, os.WriteFile(p, []byte("zz\n"), 0o600))
	k.refuse("not-hex", p)
	k.refuse("char-device", "/dev/null")
}
