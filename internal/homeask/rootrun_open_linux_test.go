package homeask

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRootRunOpenUsesNonblockingDescriptor(t *testing.T) {
	p := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(p, []byte("true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := openRootRunScript(p)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	raw, err := file.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Control(func(fd uintptr) {
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
		if errno != 0 || flags&syscall.O_NONBLOCK == 0 {
			t.Errorf("script descriptor is blocking: flags=%x errno=%v", flags, errno)
		}
	}); err != nil {
		t.Fatal(err)
	}
}
