package specs

import (
	"os"

	fileutil "github.com/mistakeknot/vauxpraudemonium/internal/file"
	"gopkg.in/yaml.v3"
)

func UpdateUserStory(path, text string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	doc := map[string]interface{}{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	userStory := map[string]interface{}{
		"text": text,
		"hash": StoryHash(text),
	}
	doc["user_story"] = userStory
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, out, 0o644)
}

func AppendReviewFeedback(path, text string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	doc := map[string]interface{}{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	var list []interface{}
	if existing, ok := doc["review_feedback"]; ok {
		if asList, ok := existing.([]interface{}); ok {
			list = asList
		}
	}
	list = append(list, text)
	doc["review_feedback"] = list
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, out, 0o644)
}

func AppendMVPExplanation(path, text string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	doc := map[string]interface{}{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	var list []interface{}
	if existing, ok := doc["mvp_explanation"]; ok {
		if asList, ok := existing.([]interface{}); ok {
			list = asList
		}
	}
	list = append(list, text)
	doc["mvp_explanation"] = list
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, out, 0o644)
}

func AcknowledgeMVPOverride(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	doc := map[string]interface{}{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	doc["mvp_override"] = "acknowledged"
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, out, 0o644)
}
