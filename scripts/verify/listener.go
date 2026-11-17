//go:build linux

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Fake-mode services must use this launcher. Owned-real services instead use the
// existing harness's namespace-local allocation and socket ownership proof.
func startFakeListener(host string) (net.Listener, error) {
	if host != "127.0.0.1" && host != "::1" {
		return nil, fmt.Errorf("fake listener requires numeric loopback")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return nil, err
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr.Port == 0 || !addr.IP.IsLoopback() {
		listener.Close()
		return nil, fmt.Errorf("invalid bound address")
	}
	if err = proveFakeListener(host, addr.Port, os.Getpid()); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func proveFakeListener(host string, port, pid int) error {
	if host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("wildcard listener refused")
	}
	want := "0100007F"
	table := "tcp"
	if host == "::1" {
		want = "00000000000000000000000001000000"
		table = "tcp6"
	}
	content, err := os.ReadFile("/proc/net/" + table)
	if err != nil {
		return err
	}
	inodes := map[string]bool{}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || fields[3] != "0A" {
			continue
		}
		parts := strings.Split(fields[1], ":")
		if len(parts) != 2 {
			continue
		}
		bound, _ := strconv.ParseInt(parts[1], 16, 32)
		if int(bound) == port {
			if parts[0] != want {
				return fmt.Errorf("bound address is not the requested loopback")
			}
			inodes[fields[9]] = true
		}
	}
	if len(inodes) != 1 {
		return fmt.Errorf("expected one owned listen socket")
	}
	fds, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		return err
	}
	for _, fd := range fds {
		link, _ := os.Readlink(filepath.Join(fmt.Sprintf("/proc/%d/fd", pid), fd.Name()))
		for inode := range inodes {
			if link == "socket:["+inode+"]" {
				return nil
			}
		}
	}
	return fmt.Errorf("listener not held by requested PID")
}
