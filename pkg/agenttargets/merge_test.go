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

func TestMergeDetected_PriorityOrder(t *testing.T) {
	detected := Registry{
		Targets: map[string]Target{
			"claude": {Name: "claude", Type: TargetDetected},
			"codex":  {Name: "codex", Type: TargetDetected},
		},
	}
	global := Registry{
		Targets: map[string]Target{
			"claude": {Name: "claude", Type: TargetCommand, Command: "/usr/local/bin/claude"},
		},
	}
	project := Registry{
		Targets: map[string]Target{
			"claude": {Name: "claude", Type: TargetCommand, Command: "/project/bin/claude"},
		},
	}

	merged := MergeDetected(detected, global, project)

	// Project overrides global overrides detected.
	if merged.Targets["claude"].Command != "/project/bin/claude" {
		t.Errorf("claude command = %q, want /project/bin/claude", merged.Targets["claude"].Command)
	}
	// Detected-only entries survive if not overridden.
	if _, ok := merged.Targets["codex"]; !ok {
		t.Error("codex should survive from detected")
	}
}

func TestDetectAvailableTargets_ReturnsRegistry(t *testing.T) {
	// DetectAvailableTargets wraps DetectAllTools. On this system it should
	// find at least one tool (claude is installed). The lookPath arg is
	// ignored but must be accepted.
	reg := DetectAvailableTargets(nil)
	// Not asserting specific tools — CI may not have them. Just verify the
	// returned registry is valid.
	if reg.Targets == nil {
		t.Fatal("Targets map is nil")
	}
	for name, target := range reg.Targets {
		if target.Type != TargetDetected {
			t.Errorf("target %q type = %q, want %q", name, target.Type, TargetDetected)
		}
	}
}
