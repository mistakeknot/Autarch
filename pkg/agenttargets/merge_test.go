package agenttargets

import "testing"

func TestMergeTargetsProjectOverridesGlobal(t *testing.T) {
	global := Registry{
		Targets: map[string]Target{
			"codex":  {Name: "codex", Type: TargetDetected, Command: "codex"},
			"custom": {Name: "custom", Type: TargetCommand, Command: "/bin/custom"},
		},
	}
	project := Registry{
		Targets: map[string]Target{
			"custom": {Name: "custom", Type: TargetCommand, Command: "/bin/project-custom"},
		},
	}
	merged := Merge(global, project)
	if merged.Targets["custom"].Command != "/bin/project-custom" {
		t.Fatalf("expected project override")
	}
}
