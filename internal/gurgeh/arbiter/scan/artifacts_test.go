package scan

import "testing"

func TestArtifacts_HasEvidence_Nil(t *testing.T) {
	var a *Artifacts
	if a.HasEvidence() {
		t.Error("nil artifacts should not have evidence")
	}
}

func TestArtifacts_HasEvidence_Empty(t *testing.T) {
	a := &Artifacts{}
	if a.HasEvidence() {
		t.Error("empty artifacts should not have evidence")
	}
}

func TestArtifacts_HasEvidence_WithEvidence(t *testing.T) {
	a := &Artifacts{
		Vision: &PhaseData{
			Evidence: []EvidenceItem{{FilePath: "README.md", Quote: "test"}},
		},
	}
	if !a.HasEvidence() {
		t.Error("expected HasEvidence=true when vision has evidence")
	}
}

func TestArtifacts_PhaseFor(t *testing.T) {
	vision := &PhaseData{Summary: "v"}
	problem := &PhaseData{Summary: "p"}
	users := &PhaseData{Summary: "u"}
	a := &Artifacts{Vision: vision, Problem: problem, Users: users}

	tests := []struct {
		phase string
		want  *PhaseData
	}{
		{"Vision", vision},
		{"Problem", problem},
		{"Users", users},
		{"Features + Goals", nil},
		{"Unknown", nil},
	}
	for _, tt := range tests {
		got := a.PhaseFor(tt.phase)
		if got != tt.want {
			t.Errorf("PhaseFor(%q) = %v, want %v", tt.phase, got, tt.want)
		}
	}
}

func TestArtifacts_PhaseFor_Nil(t *testing.T) {
	var a *Artifacts
	if a.PhaseFor("Vision") != nil {
		t.Error("nil artifacts PhaseFor should return nil")
	}
}
