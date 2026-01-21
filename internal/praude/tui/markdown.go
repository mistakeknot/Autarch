package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

type MarkdownCache struct {
	items map[string]string
}

func NewMarkdownCache() *MarkdownCache {
	return &MarkdownCache{items: make(map[string]string)}
}

func (c *MarkdownCache) key(id, hash string) string {
	return id + ":" + hash
}

func (c *MarkdownCache) Get(id, hash string) (string, bool) {
	if c == nil {
		return "", false
	}
	val, ok := c.items[c.key(id, hash)]
	return val, ok
}

func (c *MarkdownCache) Set(id, hash, value string) {
	if c == nil {
		return
	}
	c.items[c.key(id, hash)] = value
}

func renderMarkdown(content string, width int) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content
	}
	out, err := renderer.Render(content)
	if err != nil {
		return content
	}
	return out
}

func detailMarkdown(spec specs.Spec) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(spec.ID)
	if strings.TrimSpace(spec.Title) != "" {
		b.WriteString(" ")
		b.WriteString(spec.Title)
	}
	b.WriteString("\n\n## Summary\n")
	if strings.TrimSpace(spec.Summary) == "" {
		b.WriteString("_Missing summary._\n")
	} else {
		b.WriteString(spec.Summary)
		b.WriteString("\n")
	}
	b.WriteString("\n## Completeness\n")
	b.WriteString("- ")
	b.WriteString(formatCompleteness(spec))
	b.WriteString("\n")
	b.WriteString("\n## CUJ\n- ")
	b.WriteString(formatCUJDetail(spec))
	b.WriteString("\n")
	b.WriteString("\n## Research\n- ")
	b.WriteString(formatResearchDetail(spec))
	b.WriteString("\n")
	if len(spec.Metadata.ValidationWarnings) > 0 {
		b.WriteString("\n## Validation Warnings\n")
		for _, warning := range spec.Metadata.ValidationWarnings {
			b.WriteString("- ")
			b.WriteString(warning)
			b.WriteString("\n")
		}
	}
	return b.String()
}
