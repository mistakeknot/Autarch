package commands

import "testing"

func TestFormatDoctorLines(t *testing.T) {
	lines := formatDoctorLines(doctorSummary{Initialized: false})
	if len(lines) == 0 {
		t.Fatal("expected doctor lines")
	}
}

func TestDoctorIncludesWarnings(t *testing.T) {
	lines := formatDoctorLines(doctorSummary{Initialized: true, SpecWarnings: []string{"missing id"}})
	found := false
	for _, l := range lines {
		if l == "spec warnings: 1" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected spec warnings line")
	}
}

func TestDoctorIncludesDriftLine(t *testing.T) {
	lines := formatDoctorLines(doctorSummary{Initialized: true, DriftFiles: []string{"b.txt"}})
	found := false
	for _, l := range lines {
		if l == "drift warnings: 1" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected drift warnings line")
	}
}
