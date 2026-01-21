package project

import (
	"os"
	"path/filepath"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/config"
)

const PraudeDir = ".praude"

func RootDir(root string) string {
	return filepath.Join(root, PraudeDir)
}

func SpecsDir(root string) string {
	return filepath.Join(RootDir(root), "specs")
}

func ResearchDir(root string) string {
	return filepath.Join(RootDir(root), "research")
}

func SuggestionsDir(root string) string {
	return filepath.Join(RootDir(root), "suggestions")
}

func BriefsDir(root string) string {
	return filepath.Join(RootDir(root), "briefs")
}

func ConfigPath(root string) string {
	return filepath.Join(RootDir(root), "config.toml")
}

func Init(root string) error {
	dirs := []string{
		RootDir(root),
		SpecsDir(root),
		ResearchDir(root),
		SuggestionsDir(root),
		BriefsDir(root),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if _, err := os.Stat(ConfigPath(root)); os.IsNotExist(err) {
		if err := os.WriteFile(ConfigPath(root), []byte(config.DefaultConfigToml), 0o644); err != nil {
			return err
		}
	}
	return nil
}
