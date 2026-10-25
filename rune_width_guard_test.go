package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A []rune conversion in the same file as lipgloss.Width is the pattern that slices ANSI-styled strings by rune
// count where visual columns are meant. Use ansi.Truncate/TruncateLeft from charmbracelet/x/ansi instead.
var (
	runeConversion = regexp.MustCompile(`\[\]rune\(`)
	lipglossWidth  = regexp.MustCompile(`lipgloss\.Width\(`)
)

func runeWidthPattern(src string) bool {
	return runeConversion.MatchString(src) && lipglossWidth.MatchString(src)
}

func TestRuneWidthPatternDetector(t *testing.T) {
	if !runeWidthPattern("r := []rune(s)\nn := lipgloss.Width(s)") {
		t.Fatal("the []rune + lipgloss.Width pattern must be detected")
	}
	if runeWidthPattern("k := tea.KeyMsg{Runes: []rune{','}}\nn := lipgloss.Width(s)") {
		t.Fatal("a []rune literal is not a string conversion")
	}
	if runeWidthPattern("r := []rune(s)") || runeWidthPattern("n := lipgloss.Width(s)") {
		t.Fatal("either half alone is fine")
	}
}

func TestNoRuneSlicingWithLipglossWidth(t *testing.T) {
	var bad []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "vendor", ".claude", ".clavain", ".playwright-mcp":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if runeWidthPattern(string(b)) {
			bad = append(bad, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Fatalf("[]rune conversion alongside lipgloss.Width in %v: slice visual columns with ansi.Truncate/TruncateLeft from charmbracelet/x/ansi", bad)
	}
}
