package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// DocSection represents a section of content in the document panel.
type DocSection struct {
	Title   string
	Content string
	Style   lipgloss.Style // Optional custom style for content
}

// DocPanel renders document content (questions, research, tradeoffs) in the left pane.
// This is the left-side panel in the Cursor-style split layout.
type DocPanel struct {
	title    string
	subtitle string
	sections []DocSection
	width    int
	height   int
	scroll   int // Scroll offset
}

// NewDocPanel creates a new document panel.
func NewDocPanel() *DocPanel {
	return &DocPanel{
		sections: []DocSection{},
	}
}

// SetTitle sets the main title of the document.
func (p *DocPanel) SetTitle(title string) {
	p.title = title
}

// SetSubtitle sets the subtitle/description under the title.
func (p *DocPanel) SetSubtitle(subtitle string) {
	p.subtitle = subtitle
}

// SetSections sets all content sections.
func (p *DocPanel) SetSections(sections []DocSection) {
	p.sections = sections
}

// AddSection adds a section to the document.
func (p *DocPanel) AddSection(section DocSection) {
	p.sections = append(p.sections, section)
}

// ClearSections removes all sections.
func (p *DocPanel) ClearSections() {
	p.sections = nil
}

// SetSize sets the dimensions of the document panel.
func (p *DocPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// View renders the document panel.
func (p *DocPanel) View() string {
	if p.height <= 0 || p.width <= 0 {
		return ""
	}

	var lines []string
	contentWidth := p.width - 4 // Account for padding
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Title
	if p.title != "" {
		titleStyle := lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)
		lines = append(lines, titleStyle.Render(p.title))
	}

	// Subtitle
	if p.subtitle != "" {
		subtitleStyle := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)
		wrapped := wrapText(p.subtitle, contentWidth)
		for _, line := range strings.Split(wrapped, "\n") {
			lines = append(lines, subtitleStyle.Render(line))
		}
	}

	// Blank line after header if we have content
	if (p.title != "" || p.subtitle != "") && len(p.sections) > 0 {
		lines = append(lines, "")
	}

	// Sections
	for i, section := range p.sections {
		// Section title
		if section.Title != "" {
			sectionTitleStyle := lipgloss.NewStyle().
				Foreground(ColorSecondary).
				Bold(true)
			lines = append(lines, sectionTitleStyle.Render(section.Title))
		}

		// Section content
		if section.Content != "" {
			contentStyle := section.Style
			if contentStyle.Value() == "" {
				contentStyle = lipgloss.NewStyle().Foreground(ColorFg)
			}

			wrapped := wrapText(section.Content, contentWidth)
			for _, line := range strings.Split(wrapped, "\n") {
				lines = append(lines, contentStyle.Render(line))
			}
		}

		// Blank line between sections (not after last)
		if i < len(p.sections)-1 {
			lines = append(lines, "")
		}
	}

	// Apply scrolling
	if len(lines) > p.height {
		start := p.scroll
		if start < 0 {
			start = 0
		}
		end := start + p.height
		if end > len(lines) {
			end = len(lines)
			start = end - p.height
			if start < 0 {
				start = 0
			}
		}
		lines = lines[start:end]
	}

	// Don't use ensureHeight - SplitLayout.ensureSize handles height normalization
	return strings.Join(lines, "\n")
}

// ScrollUp scrolls the content up.
func (p *DocPanel) ScrollUp() {
	if p.scroll > 0 {
		p.scroll--
	}
}

// ScrollDown scrolls the content down.
func (p *DocPanel) ScrollDown() {
	p.scroll++
}

// ScrollToTop scrolls to the top of the document.
func (p *DocPanel) ScrollToTop() {
	p.scroll = 0
}

// QuestionSection creates a section styled for an interview question.
func QuestionSection(title, prompt string) DocSection {
	return DocSection{
		Title:   title,
		Content: prompt,
		Style: lipgloss.NewStyle().
			Foreground(ColorFg),
	}
}

// ResearchSection creates a section styled for research teasers.
func ResearchSection(content string) DocSection {
	return DocSection{
		Title:   "Research",
		Content: content,
		Style: lipgloss.NewStyle().
			Foreground(ColorSuccess).
			Italic(true),
	}
}

// TradeoffSection creates a section styled for tradeoff suggestions.
func TradeoffSection(content string) DocSection {
	return DocSection{
		Title:   "Suggestions",
		Content: content,
		Style: lipgloss.NewStyle().
			Foreground(ColorWarning),
	}
}

// InfoSection creates a general information section.
func InfoSection(title, content string) DocSection {
	return DocSection{
		Title:   title,
		Content: content,
		Style: lipgloss.NewStyle().
			Foreground(ColorFgDim),
	}
}
