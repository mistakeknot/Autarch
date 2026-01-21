package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

func renderMarkdown(input string, width int) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", nil
	}
	if width <= 0 {
		width = 80
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithStandardStyle("dark"),
	)
	if err != nil {
		return "", err
	}
	return renderer.Render(input)
}
