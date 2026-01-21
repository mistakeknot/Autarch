package specs

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func Validate(raw []byte) error {
	var doc struct {
		ID     string `yaml:"id"`
		Title  string `yaml:"title"`
		Status string `yaml:"status"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if doc.ID == "" || doc.Title == "" || doc.Status == "" {
		return fmt.Errorf("missing required fields")
	}
	return nil
}
