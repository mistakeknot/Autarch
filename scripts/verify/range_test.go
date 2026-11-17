//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
