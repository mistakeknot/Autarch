package specs

import (
	fileutil "github.com/mistakeknot/autarch/internal/file"
	"github.com/mistakeknot/autarch/pkg/yamlsafe"
	"gopkg.in/yaml.v3"
)

func UpdateUserStory(path, text string) error {
	doc := map[string]interface{}{}
	if _, err := yamlsafe.UnmarshalFile(path, &doc); err != nil {
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
	doc := map[string]interface{}{}
	if _, err := yamlsafe.UnmarshalFile(path, &doc); err != nil {
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
	doc := map[string]interface{}{}
	if _, err := yamlsafe.UnmarshalFile(path, &doc); err != nil {
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
	doc := map[string]interface{}{}
	if _, err := yamlsafe.UnmarshalFile(path, &doc); err != nil {
		return err
	}
	doc["mvp_override"] = "acknowledged"
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, out, 0o644)
}
