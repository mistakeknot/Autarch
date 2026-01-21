package specs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type hashPayload struct {
	StoryText            string                     `json:"story_text"`
	Summary              string                     `json:"summary"`
	Requirements         []string                   `json:"requirements"`
	Acceptance           []AcceptanceCriterion      `json:"acceptance"`
	FilesToModify        []FileChange               `json:"files_to_modify"`
	CriticalUserJourneys []CriticalUserJourney      `json:"critical_user_journeys"`
	MarketResearch       []MarketResearchItem       `json:"market_research"`
	CompetitiveLandscape []CompetitiveLandscapeItem `json:"competitive_landscape"`
}

func StoryHash(text string) string {
	return hashBytes([]byte(text))
}

func SpecHash(spec Spec) string {
	payload := hashPayload{
		StoryText:            spec.UserStory.Text,
		Summary:              spec.Summary,
		Requirements:         spec.Requirements,
		Acceptance:           spec.Acceptance,
		FilesToModify:        spec.FilesToModify,
		CriticalUserJourneys: spec.CriticalUserJourneys,
		MarketResearch:       spec.MarketResearch,
		CompetitiveLandscape: spec.CompetitiveLandscape,
	}
	data, _ := json.Marshal(payload)
	return hashBytes(data)
}

func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
