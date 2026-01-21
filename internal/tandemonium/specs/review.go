package specs

import (
	"os"
	"path/filepath"

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
	return writeFileAtomic(path, out)
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
	return writeFileAtomic(path, out)
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
	return writeFileAtomic(path, out)
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
	return writeFileAtomic(path, out)
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	dirHandle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer dirHandle.Close()
	return dirHandle.Sync()
}
