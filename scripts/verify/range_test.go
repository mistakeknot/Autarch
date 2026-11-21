//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestChangedUsesRecordedMergeBase(t *testing.T) {
	root := testRepository(t)
	run := func(args ...string) string {
		t.Helper()
		out, err := git(root, args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	write := func(name, content string) {
		t.Helper()
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	const own = "internal/homeask/filer.go"
	const unrelated = "internal/homefooter/footer.go"
	const verifier = "scripts/verify/main.go"
	for _, name := range []string{own, unrelated, verifier} {
		write(name, "original\n")
	}
	run("add", ".")
	run("commit", "-qm", "branch point")
	fork := run("rev-parse", "HEAD")
	write(own, "head edit\n")
	run("commit", "-qam", "head change")
	head := run("rev-parse", "HEAD")
	run("checkout", "--detach", fork)
	write(unrelated, "base edit\n")
	write(verifier, "base verifier edit\n")
	run("commit", "-qam", "base changes")
	movedBase := run("rev-parse", "HEAD")
	rows, err := loadMap("map.tsv")
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{movedBase, fork} {
		t.Run(map[bool]string{true: "advanced-base", false: "base-is-merge-base"}[base == movedBase], func(t *testing.T) {
			report := Report{Base: base, Head: head, MergeBase: run("merge-base", base, head)}
			names, err := changed(root, report)
			if err != nil {
				t.Fatal(err)
			}
			paths, flows := selectFlows(names, rows, nil)
			values, _ := coverageVerdicts(paths)
			t.Logf("paths=%v journeys=%v coverage=%v", names, flows, values)
			if !reflect.DeepEqual(names, []string{own}) {
				t.Errorf("selected paths %v; want only %s", names, own)
			}
			_, wantFlows := selectFlows([]string{own}, rows, nil)
			if !reflect.DeepEqual(flows, wantFlows) {
				t.Errorf("journeys %v; want %v", flows, wantFlows)
			}
			for _, value := range values {
				if value == "VERIFIER-CHANGED" {
					t.Error("base-only verifier edit credited to head")
				}
			}
			if verdict(values) == "PASS" {
				t.Error("partial coverage must never PASS")
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var recorded Report
			if err := json.Unmarshal(data, &recorded); err != nil {
				t.Fatal(err)
			}
			if recorded.Base != base || recorded.MergeBase != fork {
				t.Fatal("lost range provenance", recorded)
			}
		})
	}
}

// Exercise execute through a clean, VCS-stamped driver built in an owned repo.
// No selected journey is implemented, so this never launches Home or bb.
func TestExecuteMergeBaseRanges(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("execute requires a non-root user")
	}
	root := testRepository(t)
	run := func(args ...string) string {
		t.Helper()
		out, err := git(root, args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	write := func(name string, data []byte) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"main.go", "core.go", "listener.go", "map.tsv"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		write("scripts/verify/"+name, data)
	}
	write("scripts/verify/fixture_lock.go", []byte(fmt.Sprintf("package main\nfunc init() { checkoutWalkStop = %q; gitScratchRoot = %q; repoLockName = func() string { return %q } }\n", checkoutWalkStop, gitScratchRoot, filepath.Join(ownedTestDir(t), "verify.lock"))))
	write("go.mod", []byte("module verifyfixture\n\ngo 1.25.0\n"))
	run("add", ".")
	run("commit", "-qm", "verifier fixture")
	fork := run("rev-parse", "HEAD")
	write("docs/guide.md", []byte("head documentation\n"))
	run("add", ".")
	run("commit", "-qm", "head documentation")
	head := run("rev-parse", "HEAD")
	run("checkout", "--detach", fork)
	write("internal/homefooter/footer.go", []byte("base-only change\n"))
	run("add", ".")
	run("commit", "-qm", "base change")
	base := run("rev-parse", "HEAD")
	run("checkout", "--orphan", "unrelated")
	run("commit", "-qm", "unrelated root")
	unrelated := run("rev-parse", "HEAD")
	run("checkout", "--detach", base)
	bin := filepath.Join(ownedTestDir(t), "verify")
	build := exec.Command("go", "build", "-race", "-buildvcs=true", "-o", bin, "./scripts/verify")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GOMAXPROCS=2", "GOFLAGS=-p=2")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build driver: %v\n%s", err, out)
	}
	for _, tc := range []struct{ name, base, head, mergeBase string }{
		{"unrelated-histories", unrelated, head, ""},
		{"head-ancestor-of-base", base, fork, fork},
		{"merge-base-before-changed", base, head, fork},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := ownedTestDir(t)
			outDir := filepath.Join(state, "out")
			cmd := exec.Command(bin, "--base", tc.base, "--head", tc.head, "--verifier-commit", base, "--out", outDir)
			cmd.Dir = root
			cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + state, "XDG_STATE_HOME=" + state}
			output, runErr := cmd.CombinedOutput()
			data, err := os.ReadFile(filepath.Join(outDir, "report.json"))
			if err != nil {
				t.Fatalf("report: %v; run: %v\n%s", err, runErr, output)
			}
			var report Report
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			if tc.mergeBase == "" {
				if len(report.Journeys) != 0 {
					t.Fatalf("unrelated histories ran journeys: %+v", report.Journeys)
				}
				entries, err := os.ReadDir(outDir)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if entry.Name() != "report.json" && entry.Name() != "report.md" {
						t.Errorf("unrelated histories created journey output: %s", entry.Name())
					}
				}
				if runErr == nil || report.Verdict != "INCOMPLETE" {
					t.Fatalf("unrelated histories did not fail closed: %v %+v", runErr, report)
				}
				if !strings.Contains(string(output), "no merge-base between base and head:") {
					t.Fatalf("missing range context: %s", output)
				}
				return
			}
			if runErr != nil {
				t.Fatalf("execute: %v\n%s", runErr, output)
			}
			if report.MergeBase != tc.mergeBase {
				t.Fatalf("merge base = %q, want %q", report.MergeBase, tc.mergeBase)
			}
			if report.Verdict != "NO-FLOW" {
				t.Fatalf("verdict = %s, want NO-FLOW: %+v", report.Verdict, report)
			}
			var paths []string
			for _, p := range report.Paths {
				paths = append(paths, p.Path)
			}
			var want []string
			if tc.head == head {
				want = []string{"docs/guide.md"}
			}
			if !reflect.DeepEqual(paths, want) {
				t.Fatalf("changed paths = %v, want %v", paths, want)
			}
		})
	}
}
