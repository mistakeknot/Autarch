package callerlog

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// resolvePeer runs only on the single audit worker, with bounded entries and time.
// A departed connection, inaccessible procfs or exhausted budget is unknown.
func resolvePeer(remote string) any {
	if runtime.GOOS != "linux" {
		return "unknown"
	}
	return resolvePeerAt("/proc", remote)
}

func resolvePeerAt(root, remote string) any {
	host, port, err := net.SplitHostPort(remote)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return "unknown"
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return "unknown"
	}
	deadline := time.Now().Add(20 * time.Millisecond)
	inode := ""
	for _, table := range []string{filepath.Join(root, "net/tcp"), filepath.Join(root, "net/tcp6")} {
		f, err := os.Open(table)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for n := 0; n < 4096 && time.Now().Before(deadline) && sc.Scan(); n++ {
			fields := strings.Fields(sc.Text())
			if len(fields) < 10 {
				continue
			}
			addr := strings.Split(fields[1], ":")
			if len(addr) == 2 && strings.EqualFold(addr[1], fmt.Sprintf("%04X", p)) && procIP(addr[0]).Equal(net.ParseIP(host)) && fields[9] != "0" {
				inode = fields[9]
				break
			}
		}
		f.Close()
		if inode != "" {
			break
		}
	}
	if inode == "" {
		return "unknown"
	}
	proc, err := os.Open(root)
	if err != nil {
		return "unknown"
	}
	defer proc.Close()
	budget := 8192
	for budget > 0 && time.Now().Before(deadline) {
		names, _ := proc.Readdirnames(64)
		if len(names) == 0 {
			break
		}
		for _, name := range names {
			pid, err := strconv.Atoi(name)
			if err != nil {
				continue
			}
			fd, err := os.Open(filepath.Join(root, name, "fd"))
			if err != nil {
				continue
			}
			for budget > 0 && time.Now().Before(deadline) {
				entries, _ := fd.Readdirnames(64)
				if len(entries) == 0 {
					break
				}
				for _, entry := range entries {
					budget--
					target, _ := os.Readlink(filepath.Join(root, name, "fd", entry))
					if target == "socket:["+inode+"]" {
						fd.Close()
						comm, _ := os.ReadFile(filepath.Join(root, name, "comm"))
						exe, _ := os.Readlink(filepath.Join(root, name, "exe"))
						return map[string]any{"pid": pid, "comm": strings.TrimSpace(string(comm)), "exe": exe}
					}
				}
			}
			fd.Close()
		}
	}
	return "unknown"
}

// procfs encodes addresses as native-endian 32-bit words.
func procIP(s string) net.IP {
	b, err := hex.DecodeString(s)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return nil
	}
	for i := 0; i < len(b); i += 4 {
		v := binary.NativeEndian.Uint32(b[i : i+4])
		binary.BigEndian.PutUint32(b[i:i+4], v)
	}
	return net.IP(b)
}
