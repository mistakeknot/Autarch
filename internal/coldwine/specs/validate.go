package specs

import (
	"fmt"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
)

func Validate(raw []byte) error {
	var doc struct {
		ID     string `yaml:"id"`
		Title  string `yaml:"title"`
		Status string `yaml:"status"`
	}
	if err := yamlsafe.Decode(raw, &doc); err != nil {
		return err
	}
	if doc.ID == "" || doc.Title == "" || doc.Status == "" {
		return fmt.Errorf("missing required fields")
	}
	return nil
}
