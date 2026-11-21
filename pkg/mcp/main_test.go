package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/pkg/callerlog"
)

// Isolate all existing tests, including subprocesses, from the user's state.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "autarch-test-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Unsetenv("BB_THREAD_ID")
	code := m.Run()
	for _, path := range []string{filepath.Join(home, ".autarch", "caller-log.jsonl"), filepath.Join(home, ".autarch", "audit", "caller-log.jsonl")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "tests wrote the default HOME caller log")
			code = 1
		}
	}

	os.RemoveAll(home)
	os.Exit(code)
}

// newTestServer explicitly disables audit storage; injected audit tests own their logs.
func newTestServer(project string) *Server {
	return NewServer(project).WithCallerLog(callerlog.New(""))
}
