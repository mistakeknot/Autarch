package specs

import "testing"

func TestStoryHashStable(t *testing.T) {
	one := StoryHash("Hello")
	two := StoryHash("Hello")
	if one == "" || two == "" {
		t.Fatalf("expected hash")
	}
	if one != two {
		t.Fatalf("expected stable hash")
	}
}

func TestSpecHashChangesWhenCUJChanges(t *testing.T) {
	base := Spec{
		UserStory:    UserStory{Text: "Story"},
		Summary:      "Summary",
		Requirements: []string{"REQ-001: R"},
		CriticalUserJourneys: []CriticalUserJourney{{
			ID:       "CUJ-001",
			Title:    "Journey",
			Priority: "high",
		}},
	}
	hashA := SpecHash(base)
	base.CriticalUserJourneys[0].Title = "Different"
	hashB := SpecHash(base)
	if hashA == hashB {
		t.Fatalf("expected cuj change to affect hash")
	}
}

func TestSpecHashChangesWhenEvidenceChanges(t *testing.T) {
	base := Spec{
		UserStory:    UserStory{Text: "Story"},
		Summary:      "Summary",
		Requirements: []string{"REQ-001: R"},
		MarketResearch: []MarketResearchItem{{
			ID:    "MR-001",
			Claim: "Market",
			EvidenceRefs: []EvidenceRef{{
				Path:   ".praude/research/PRD-001-20260115-000000.md",
				Anchor: "section",
				Note:   "note",
			}},
		}},
	}
	hashA := SpecHash(base)
	base.MarketResearch[0].EvidenceRefs[0].Path = ".praude/research/PRD-001-20260115-111111.md"
	hashB := SpecHash(base)
	if hashA == hashB {
		t.Fatalf("expected evidence change to affect hash")
	}
}
