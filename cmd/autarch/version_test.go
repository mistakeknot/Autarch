package main

import (
	"runtime/debug"
	"testing"
)

func TestVersionRevisionSources(t *testing.T) {
	original := buildRevision
	t.Cleanup(func() { buildRevision = original })
	info := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "fallback"},
		{Key: "vcs.modified", Value: "true"},
	}}
	buildRevision = ""
	v := versionVCS(info)
	if v.Revision != "fallback" || !v.Stamped || !v.Modified {
		t.Fatalf("ordinary build lost its build info: %+v", v)
	}
	buildRevision = "linked"
	for _, bi := range []*debug.BuildInfo{info, nil} {
		v = versionVCS(bi)
		if v.Revision != "linked" || !v.Stamped || v.Modified {
			t.Fatalf("linker stamp not used: %+v", v)
		}
	}
	buildRevision = ""
	if v = versionVCS(nil); v.Stamped || v.Modified || v.Revision != "" {
		t.Fatalf("unstamped build: %+v", v)
	}
}
