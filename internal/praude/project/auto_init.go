package project

import (
	"os"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

// EnsureInitialized creates .praude and a template spec on first run.
// If .praude already exists, it leaves the workspace untouched.
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
