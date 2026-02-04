package project

import (
	"os"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// EnsureInitialized creates .gurgeh and a template spec on first run.
// If the data directory already exists, it leaves the workspace untouched.
func EnsureInitialized(root string) error {
	if _, err := os.Stat(RootDir(root)); err == nil {
		return nil
	}
	if err := Init(root); err != nil {
		return err
	}
	_, err := specs.CreateTemplate(SpecsDir(root), time.Now())
	return err
}
