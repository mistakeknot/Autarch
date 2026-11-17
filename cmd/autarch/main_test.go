package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
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
	if _, err := os.Stat(filepath.Join(home, ".autarch", "caller-log.jsonl")); !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "tests wrote the default HOME caller log")
		code = 1
	}
	os.RemoveAll(home)
	os.Exit(code)
}
