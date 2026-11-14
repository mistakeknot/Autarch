package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenDBCreatesNestedDataDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing", "nested", "mycroft")
	t.Setenv("MYCROFT_DATA_DIR", dir)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected absent data directory, got %v", err)
	}
	log, closeDB, err := openDB()
	if err != nil {
		t.Fatalf("openDB with missing nested directory: %v", err)
	}
	defer closeDB()
	if log == nil {
		t.Fatal("openDB returned nil approval log")
	}
	if info, err := os.Stat(filepath.Join(dir, dbFile)); err != nil {
		t.Fatalf("database was not created: %v", err)
	} else if !info.Mode().IsRegular() {
		t.Fatal("database is not a regular file")
	}
}

// Run the real entry point in a child process, including main's error printing.
func TestMycroftCLIHelper(t *testing.T) {
	if os.Getenv("MYCROFT_CLI_HELPER") != "1" {
		return
	}
	os.Args = append([]string{"mycroft"}, strings.Split(os.Getenv("MYCROFT_CLI_ARGS"), " ")...)
	main()
	os.Exit(0)
}

func runMycroftCLI(t *testing.T, cwd, data string, args string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMycroftCLIHelper$")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "MYCROFT_CLI_HELPER=1", "MYCROFT_CLI_ARGS="+args, "MYCROFT_DATA_DIR="+data)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestPreDispatchCLIReportsFailureOnceWithoutUsage(t *testing.T) {
	dir := t.TempDir()
	data := t.TempDir()
	script := "#!/bin/sh\nprintf '%s:%s' \"$MYCROFT_BEAD_ID\" \"$MYCROFT_AGENT\" > checked\nexit 7\n"
	if err := os.WriteFile(filepath.Join(dir, "check.sh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	config := "pre_dispatch_check:\n  command: [./check.sh]\n"
	if err := os.WriteFile(filepath.Join(data, "config.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runMycroftCLI(t, dir, data, "pre-dispatch --bead test-bead --agent test-agent")
	if err == nil {
		t.Fatalf("failed check returned success: %s", out)
	}
	checked, readErr := os.ReadFile(filepath.Join(dir, "checked"))
	if readErr != nil || string(checked) != "test-bead:test-agent" {
		t.Fatalf("check did not receive assignment: %q, %v; output: %s", checked, readErr, out)
	}
	if strings.Contains(out, "Usage:") {
		t.Errorf("failed check printed usage: %s", out)
	}
	if strings.Count(out, "exit status 7") != 1 {
		t.Errorf("expected one check error: %s", out)
	}
}

func TestPreDispatchCLIDoesNotCreateDataDirectory(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "override"}[override], func(t *testing.T) {
			dir := t.TempDir()
			data := ""
			wantAbsent := filepath.Join(dir, defaultDataDir)
			if override {
				data = filepath.Join(dir, "custom-data")
				wantAbsent = data
			}
			out, err := runMycroftCLI(t, dir, data, "pre-dispatch")
			if err != nil {
				t.Fatalf("absent config should succeed: %v: %s", err, out)
			}
			if _, err := os.Stat(wantAbsent); !os.IsNotExist(err) {
				t.Fatalf("pre-dispatch created data directory: stat error %v", err)
			}
		})
	}
}
