//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testRepository(t *testing.T) string {
	t.Helper()
	root := ownedTestDir(t)
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "verify test"}, {"config", "user.email", "verify@example.invalid"}, {"commit", "--allow-empty", "-m", "test fixture"}} {
		if _, err := git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestEmptyAndDocsOnlyNeverPass(t *testing.T) {
	for _, names := range [][]string{nil, {"docs/guide.md", "README.md"}} {
		paths, flows := selectFlows(names, nil, nil)
		values, nothing := coverageVerdicts(paths)
		if !nothing || len(flows) != 0 || verdict(values) != "NO-FLOW" {
			t.Fatal(names, values, flows, nothing)
		}
	}
	if got := classifyHarness(0, []byte(`{"scenario":"pick-click","pass":true}`)); got != "PASS" {
		t.Fatal("a completed assertion must still pass:", got)
	}
}

func TestRejectSameCommitAndUnknownRefs(t *testing.T) {
	root := testRepository(t)
	for _, refs := range [][2]string{{"HEAD", "HEAD"}, {"HEAD", "missing-head"}, {"missing-base", "HEAD"}} {
		if _, _, err := resolveRange(root, refs[0], refs[1]); err == nil {
			t.Fatal("accepted unverifiable range", refs)
		}
	}
}

func TestMirrorIncludesRemoteOnlyAndDetachedHeads(t *testing.T) {
	root := testRepository(t)
	base, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"remote-only", "detached"} {
		t.Run(name, func(t *testing.T) {
			if _, err := git(root, "checkout", "--detach", base); err != nil {
				t.Fatal(err)
			}
			if _, err := git(root, "commit", "--allow-empty", "-m", name); err != nil {
				t.Fatal(err)
			}
			head, err := git(root, "rev-parse", "HEAD")
			if err != nil {
				t.Fatal(err)
			}
			if name == "remote-only" {
				if _, err = git(root, "update-ref", "refs/remotes/origin/probe", head); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = git(root, "checkout", "--detach", base); err != nil {
				t.Fatal(err)
			}
			work := ownedTestDir(t)
			mirror, product := filepath.Join(work, "objects.git"), filepath.Join(work, "product")
			if err = seedMirror(root, mirror, head); err != nil {
				t.Fatal(err)
			}
			if _, err = git(mirror, "worktree", "add", "--detach", product, head); err != nil {
				t.Fatal(err)
			}
			if got, err := git(product, "rev-parse", "HEAD"); err != nil || got != head {
				t.Fatal(got, err)
			}
		})
	}
}

func TestEveryManifestInputMarksVerifierChanged(t *testing.T) {
	root := testRepository(t)
	for _, name := range []string{"scripts/verify/probe.go", ".claude/skills/autarch-verify/SKILL.md", plugin + "/e2e/probe.ts", plugin + "/scripts/probe.mjs", plugin + "/__tests__/fake-bb-server.ts", plugin + "/__tests__/fixtures/nested/probe.ts"} {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	m, _, err := manifest(root)
	if err != nil {
		t.Fatal(err)
	}
	r := Report{Manifest: m}
	if err = productVerifierManifest(root, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Manifest) != 6 {
		t.Fatal(r.Manifest)
	}
	for name := range r.Manifest {
		name = strings.TrimPrefix(name, "head:")
		values, _ := coverageVerdicts([]Mapping{{Path: name, Coverage: "full"}})
		if verdict(values) != "VERIFIER-CHANGED" {
			t.Fatal("hashed input was not marked", name, values)
		}
	}
}

func TestSuiteResultsAreMarkedAsUnverifiedClaims(t *testing.T) {
	dir := t.TempDir()
	if err := writeReport(dir, filepath.Join(dir, "ledger.jsonl"), Report{Head: strings64(), Suites: map[string]string{"test": "PASS"}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "report.json"))
	var report map[string]any
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	if report["suite_results_source"] != "operator-supplied, unverified" {
		t.Fatal("suite provenance missing", report)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "report.md"))
	if !strings.Contains(string(md), "operator-supplied, unverified") {
		t.Fatal(string(md))
	}
}
