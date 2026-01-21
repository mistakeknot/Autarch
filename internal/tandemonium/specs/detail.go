package specs

import (
	"os"

	"gopkg.in/yaml.v3"
)

type SpecDetail struct {
	ID                 string
	Title              string
	Summary            string
	UserStory          string
	UserStoryHash      string
	MVPIncluded        *bool
	AcceptanceCriteria []string
}

func LoadDetail(path string) (SpecDetail, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SpecDetail{}, err
	}
	doc := map[string]interface{}{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return SpecDetail{}, err
	}
	detail := SpecDetail{}
	if v, ok := doc["id"].(string); ok {
		detail.ID = v
	}
	if v, ok := doc["title"].(string); ok {
		detail.Title = v
	}
	if v, ok := doc["summary"].(string); ok {
		detail.Summary = v
	}
	if userStory, ok := doc["user_story"].(map[string]interface{}); ok {
		if v, ok := userStory["text"].(string); ok {
			detail.UserStory = v
		}
		if v, ok := userStory["hash"].(string); ok {
			detail.UserStoryHash = v
		}
	}
	if strategic, ok := doc["strategic_context"].(map[string]interface{}); ok {
		if v, ok := strategic["mvp_included"].(bool); ok {
			detail.MVPIncluded = &v
		}
	}
	if items, ok := doc["acceptance_criteria"].([]interface{}); ok {
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				if v, ok := m["description"].(string); ok {
					detail.AcceptanceCriteria = append(detail.AcceptanceCriteria, v)
				}
			}
		}
	}
	return detail, nil
}
