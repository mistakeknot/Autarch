// Package theme provides a two-tier color system for Autarch TUIs.
// Tier 1: Theme struct with raw palette colors (Catppuccin, Tokyo Night, Nord, Plain).
// Tier 2: SemanticPalette with role-based aliases (see semantic.go).
// Components should reference SemanticPalette, never raw colors.
package theme

import (
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Theme defines a complete color palette for the TUI.
type Theme struct {
	// Base colors
	Base     lipgloss.Color // Background
	Mantle   lipgloss.Color // Slightly lighter bg
	Crust    lipgloss.Color // Darkest bg
	Surface0 lipgloss.Color // Surface
	Surface1 lipgloss.Color // Surface highlight
	Surface2 lipgloss.Color // Surface bright

	// Text colors
	Text    lipgloss.Color // Primary text
	Subtext lipgloss.Color // Secondary text
	Overlay lipgloss.Color // Dimmed text

	// Accent colors (Catppuccin naming)
	Rosewater lipgloss.Color
	Flamingo  lipgloss.Color
	Pink      lipgloss.Color
	Mauve     lipgloss.Color
	Red       lipgloss.Color
	Maroon    lipgloss.Color
	Peach     lipgloss.Color
	Yellow    lipgloss.Color
	Green     lipgloss.Color
	Teal      lipgloss.Color
	Sky       lipgloss.Color
	Sapphire  lipgloss.Color
	Blue      lipgloss.Color
	Lavender  lipgloss.Color

	// Semantic colors
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	Error     lipgloss.Color
	Info      lipgloss.Color

	// Agent-specific colors
	Claude lipgloss.Color
	Codex  lipgloss.Color
	Gemini lipgloss.Color
	Aider  lipgloss.Color
	Cursor lipgloss.Color
	User   lipgloss.Color
}

// TokyoNight is the default Autarch theme, mapped to Catppuccin Mocha structure.
// These are the exact hex values from pkg/tui/colors.go.
var TokyoNight = Theme{
	Base:     lipgloss.Color("#1a1b26"),
	Mantle:   lipgloss.Color("#16161e"),
	Crust:    lipgloss.Color("#13131a"),
	Surface0: lipgloss.Color("#24283b"),
	Surface1: lipgloss.Color("#292e42"),
	Surface2: lipgloss.Color("#3b4261"),

	Text:    lipgloss.Color("#c0caf5"),
	Subtext: lipgloss.Color("#a9b1d6"),
	Overlay: lipgloss.Color("#565f89"),

	Rosewater: lipgloss.Color("#f5e0dc"),
	Flamingo:  lipgloss.Color("#f2cdcd"),
	Pink:      lipgloss.Color("#f5c2e7"),
	Mauve:     lipgloss.Color("#bb9af7"),
	Red:       lipgloss.Color("#f7768e"),
	Maroon:    lipgloss.Color("#ff9e64"),
	Peach:     lipgloss.Color("#ff9e64"),
	Yellow:    lipgloss.Color("#e0af68"),
	Green:     lipgloss.Color("#9ece6a"),
	Teal:      lipgloss.Color("#73daca"),
	Sky:       lipgloss.Color("#7dcfff"),
	Sapphire:  lipgloss.Color("#2ac3de"),
	Blue:      lipgloss.Color("#7aa2f7"),
	Lavender:  lipgloss.Color("#b4befe"),

	Primary:   lipgloss.Color("#7aa2f7"),
	Secondary: lipgloss.Color("#bb9af7"),
	Success:   lipgloss.Color("#9ece6a"),
	Warning:   lipgloss.Color("#e0af68"),
	Error:     lipgloss.Color("#f7768e"),
	Info:      lipgloss.Color("#7dcfff"),

	Claude: lipgloss.Color("#e07353"),
	Codex:  lipgloss.Color("#00D4AA"),
	Gemini: lipgloss.Color("#f9e2af"),
	Aider:  lipgloss.Color("#14B8A6"),
	Cursor: lipgloss.Color("#0066FF"),
	User:   lipgloss.Color("#9ece6a"),
}

// CatppuccinMocha is the flagship Catppuccin dark theme.
var CatppuccinMocha = Theme{
	Base:     lipgloss.Color("#1e1e2e"),
	Mantle:   lipgloss.Color("#181825"),
	Crust:    lipgloss.Color("#11111b"),
	Surface0: lipgloss.Color("#313244"),
	Surface1: lipgloss.Color("#45475a"),
	Surface2: lipgloss.Color("#585b70"),

	Text:    lipgloss.Color("#cdd6f4"),
	Subtext: lipgloss.Color("#a6adc8"),
	Overlay: lipgloss.Color("#6c7086"),

	Rosewater: lipgloss.Color("#f5e0dc"),
	Flamingo:  lipgloss.Color("#f2cdcd"),
	Pink:      lipgloss.Color("#f5c2e7"),
	Mauve:     lipgloss.Color("#cba6f7"),
	Red:       lipgloss.Color("#f38ba8"),
	Maroon:    lipgloss.Color("#eba0ac"),
	Peach:     lipgloss.Color("#fab387"),
	Yellow:    lipgloss.Color("#f9e2af"),
	Green:     lipgloss.Color("#a6e3a1"),
	Teal:      lipgloss.Color("#94e2d5"),
	Sky:       lipgloss.Color("#89dceb"),
	Sapphire:  lipgloss.Color("#74c7ec"),
	Blue:      lipgloss.Color("#89b4fa"),
	Lavender:  lipgloss.Color("#b4befe"),

	Primary:   lipgloss.Color("#89b4fa"),
	Secondary: lipgloss.Color("#cba6f7"),
	Success:   lipgloss.Color("#a6e3a1"),
	Warning:   lipgloss.Color("#f9e2af"),
	Error:     lipgloss.Color("#f38ba8"),
	Info:      lipgloss.Color("#89dceb"),

	Claude: lipgloss.Color("#e07353"),
	Codex:  lipgloss.Color("#00D4AA"),
	Gemini: lipgloss.Color("#f9e2af"),
	Aider:  lipgloss.Color("#14B8A6"),
	Cursor: lipgloss.Color("#0066FF"),
	User:   lipgloss.Color("#a6e3a1"),
}

// CatppuccinMacchiato is a darker Catppuccin variant.
var CatppuccinMacchiato = Theme{
	Base:     lipgloss.Color("#24273a"),
	Mantle:   lipgloss.Color("#1e2030"),
	Crust:    lipgloss.Color("#181926"),
	Surface0: lipgloss.Color("#363a4f"),
	Surface1: lipgloss.Color("#494d64"),
	Surface2: lipgloss.Color("#5b6078"),

	Text:    lipgloss.Color("#cad3f5"),
	Subtext: lipgloss.Color("#a5adcb"),
	Overlay: lipgloss.Color("#6e738d"),

	Rosewater: lipgloss.Color("#f4dbd6"),
	Flamingo:  lipgloss.Color("#f0c6c6"),
	Pink:      lipgloss.Color("#f5bde6"),
	Mauve:     lipgloss.Color("#c6a0f6"),
	Red:       lipgloss.Color("#ed8796"),
	Maroon:    lipgloss.Color("#ee99a0"),
	Peach:     lipgloss.Color("#f5a97f"),
	Yellow:    lipgloss.Color("#eed49f"),
	Green:     lipgloss.Color("#a6da95"),
	Teal:      lipgloss.Color("#8bd5ca"),
	Sky:       lipgloss.Color("#91d7e3"),
	Sapphire:  lipgloss.Color("#7dc4e4"),
	Blue:      lipgloss.Color("#8aadf4"),
	Lavender:  lipgloss.Color("#b7bdf8"),

	Primary:   lipgloss.Color("#8aadf4"),
	Secondary: lipgloss.Color("#c6a0f6"),
	Success:   lipgloss.Color("#a6da95"),
	Warning:   lipgloss.Color("#eed49f"),
	Error:     lipgloss.Color("#ed8796"),
	Info:      lipgloss.Color("#91d7e3"),

	Claude: lipgloss.Color("#e07353"),
	Codex:  lipgloss.Color("#00D4AA"),
	Gemini: lipgloss.Color("#eed49f"),
	Aider:  lipgloss.Color("#14B8A6"),
	Cursor: lipgloss.Color("#0066FF"),
	User:   lipgloss.Color("#a6da95"),
}

// CatppuccinLatte is a light theme for light terminals.
var CatppuccinLatte = Theme{
	Base:     lipgloss.Color("#eff1f5"),
	Mantle:   lipgloss.Color("#e6e9ef"),
	Crust:    lipgloss.Color("#dce0e8"),
	Surface0: lipgloss.Color("#ccd0da"),
	Surface1: lipgloss.Color("#bcc0cc"),
	Surface2: lipgloss.Color("#acb0be"),

	Text:    lipgloss.Color("#4c4f69"),
	Subtext: lipgloss.Color("#6c6f85"),
	Overlay: lipgloss.Color("#7c7f93"),

	Rosewater: lipgloss.Color("#dc8a78"),
	Flamingo:  lipgloss.Color("#dd7878"),
	Pink:      lipgloss.Color("#ea76cb"),
	Mauve:     lipgloss.Color("#8839ef"),
	Red:       lipgloss.Color("#d20f39"),
	Maroon:    lipgloss.Color("#e64553"),
	Peach:     lipgloss.Color("#fe640b"),
	Yellow:    lipgloss.Color("#df8e1d"),
	Green:     lipgloss.Color("#40a02b"),
	Teal:      lipgloss.Color("#179299"),
	Sky:       lipgloss.Color("#04a5e5"),
	Sapphire:  lipgloss.Color("#209fb5"),
	Blue:      lipgloss.Color("#1e66f5"),
	Lavender:  lipgloss.Color("#7287fd"),

	Primary:   lipgloss.Color("#1e66f5"),
	Secondary: lipgloss.Color("#8839ef"),
	Success:   lipgloss.Color("#40a02b"),
	Warning:   lipgloss.Color("#df8e1d"),
	Error:     lipgloss.Color("#d20f39"),
	Info:      lipgloss.Color("#04a5e5"),

	Claude: lipgloss.Color("#e07353"),
	Codex:  lipgloss.Color("#00D4AA"),
	Gemini: lipgloss.Color("#df8e1d"),
	Aider:  lipgloss.Color("#14B8A6"),
	Cursor: lipgloss.Color("#0066FF"),
	User:   lipgloss.Color("#40a02b"),
}

// Plain is a no-color theme that uses empty/default colors.
// Used when NO_COLOR is set or for accessibility needs.
var Plain = Theme{}

// Nord is a popular arctic theme.
var Nord = Theme{
	Base:     lipgloss.Color("#2e3440"),
	Mantle:   lipgloss.Color("#272c36"),
	Crust:    lipgloss.Color("#21262e"),
	Surface0: lipgloss.Color("#3b4252"),
	Surface1: lipgloss.Color("#434c5e"),
	Surface2: lipgloss.Color("#4c566a"),

	Text:    lipgloss.Color("#eceff4"),
	Subtext: lipgloss.Color("#d8dee9"),
	Overlay: lipgloss.Color("#7b88a1"),

	Rosewater: lipgloss.Color("#d8dee9"),
	Flamingo:  lipgloss.Color("#d08770"),
	Pink:      lipgloss.Color("#b48ead"),
	Mauve:     lipgloss.Color("#b48ead"),
	Red:       lipgloss.Color("#bf616a"),
	Maroon:    lipgloss.Color("#d08770"),
	Peach:     lipgloss.Color("#d08770"),
	Yellow:    lipgloss.Color("#ebcb8b"),
	Green:     lipgloss.Color("#a3be8c"),
	Teal:      lipgloss.Color("#8fbcbb"),
	Sky:       lipgloss.Color("#88c0d0"),
	Sapphire:  lipgloss.Color("#81a1c1"),
	Blue:      lipgloss.Color("#5e81ac"),
	Lavender:  lipgloss.Color("#b48ead"),

	Primary:   lipgloss.Color("#88c0d0"),
	Secondary: lipgloss.Color("#b48ead"),
	Success:   lipgloss.Color("#a3be8c"),
	Warning:   lipgloss.Color("#ebcb8b"),
	Error:     lipgloss.Color("#bf616a"),
	Info:      lipgloss.Color("#81a1c1"),

	Claude: lipgloss.Color("#e07353"),
	Codex:  lipgloss.Color("#00D4AA"),
	Gemini: lipgloss.Color("#ebcb8b"),
	Aider:  lipgloss.Color("#14B8A6"),
	Cursor: lipgloss.Color("#0066FF"),
	User:   lipgloss.Color("#a3be8c"),
}

// Default is the currently active theme.
var Default = TokyoNight

// NoColorEnabled returns true if color output should be disabled.
// Respects the NO_COLOR standard (https://no-color.org/).
// AUTARCH_NO_COLOR=0/false forces colors ON (overrides NO_COLOR).
func NoColorEnabled() bool {
	override := strings.TrimSpace(os.Getenv("AUTARCH_NO_COLOR"))
	switch strings.ToLower(override) {
	case "0", "false", "no", "off":
		return false
	case "1", "true", "yes", "on":
		return true
	}
	_, noColorSet := os.LookupEnv("NO_COLOR")
	return noColorSet
}

// FromName returns a theme by name.
func FromName(name string) Theme {
	if NoColorEnabled() {
		return Plain
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "plain", "none", "no-color", "nocolor":
		return Plain
	case "macchiato":
		return CatppuccinMacchiato
	case "nord":
		return Nord
	case "latte", "light":
		return CatppuccinLatte
	case "mocha":
		return CatppuccinMocha
	case "tokyo", "tokyo-night":
		return TokyoNight
	case "auto", "":
		return autoTheme()
	default:
		return autoTheme()
	}
}

// Current returns the theme specified by AUTARCH_THEME, defaulting to auto.
func Current() Theme {
	return FromName(os.Getenv("AUTARCH_THEME"))
}

// detectDarkBackground inspects the terminal to determine if a dark
// background is in use. Defined as a variable for testability.
// Skips OSC queries over SSH to avoid escape sequence races.
var detectDarkBackground = func() bool {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return true
	}
	output := termenv.NewOutput(os.Stdout)
	return output.HasDarkBackground()
}

var (
	cachedAutoTheme Theme
	autoThemeOnce   sync.Once
)

// resetAutoTheme resets the cached auto theme for testing.
var resetAutoTheme = func() {
	autoThemeOnce = sync.Once{}
	cachedAutoTheme = Theme{}
}

func autoTheme() Theme {
	autoThemeOnce.Do(func() {
		cachedAutoTheme = TokyoNight
		defer func() {
			if recover() != nil {
				cachedAutoTheme = TokyoNight
			}
		}()
		if detectDarkBackground() {
			cachedAutoTheme = TokyoNight
		} else {
			cachedAutoTheme = CatppuccinLatte
		}
	})
	return cachedAutoTheme
}

// Styles contains pre-built lipgloss styles for a theme.
type Styles struct {
	App     lipgloss.Style
	Header  lipgloss.Style
	Title   lipgloss.Style
	Divider lipgloss.Style

	Normal    lipgloss.Style
	Bold      lipgloss.Style
	Dim       lipgloss.Style
	Highlight lipgloss.Style

	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Info    lipgloss.Style

	Box          lipgloss.Style
	BoxTitle     lipgloss.Style
	List         lipgloss.Style
	ListItem     lipgloss.Style
	ListSelected lipgloss.Style
	ListCursor   lipgloss.Style

	Claude lipgloss.Style
	Codex  lipgloss.Style
	Gemini lipgloss.Style
	User   lipgloss.Style

	Button       lipgloss.Style
	ButtonActive lipgloss.Style
	Input        lipgloss.Style
	InputFocused lipgloss.Style

	Help      lipgloss.Style
	StatusBar lipgloss.Style
}

// NewStyles creates a Styles instance from a theme.
func NewStyles(t Theme) Styles {
	styles := Styles{
		App:    lipgloss.NewStyle().Background(t.Base),
		Header: lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Padding(0, 1),
		Title:  lipgloss.NewStyle().Bold(true).Foreground(t.Text),
		Divider: lipgloss.NewStyle().Foreground(t.Surface2),

		Normal:    lipgloss.NewStyle().Foreground(t.Text),
		Bold:      lipgloss.NewStyle().Bold(true).Foreground(t.Text),
		Dim:       lipgloss.NewStyle().Foreground(t.Overlay),
		Highlight: lipgloss.NewStyle().Bold(true).Foreground(t.Rosewater),

		Success: lipgloss.NewStyle().Bold(true).Foreground(t.Success),
		Warning: lipgloss.NewStyle().Bold(true).Foreground(t.Warning),
		Error:   lipgloss.NewStyle().Bold(true).Foreground(t.Error),
		Info:    lipgloss.NewStyle().Bold(true).Foreground(t.Info),

		Box:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Surface2).Padding(1, 2),
		BoxTitle: lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Padding(0, 1),
		List:     lipgloss.NewStyle().Padding(0, 1),
		ListItem: lipgloss.NewStyle().Foreground(t.Text).Padding(0, 1),
		ListSelected: lipgloss.NewStyle().Bold(true).Foreground(t.Base).Background(t.Primary).Padding(0, 1),
		ListCursor:   lipgloss.NewStyle().Bold(true).Foreground(t.Primary),

		Claude: lipgloss.NewStyle().Foreground(t.Claude),
		Codex:  lipgloss.NewStyle().Foreground(t.Codex),
		Gemini: lipgloss.NewStyle().Foreground(t.Gemini),
		User:   lipgloss.NewStyle().Foreground(t.User),

		Button:       lipgloss.NewStyle().Foreground(t.Text).Background(t.Surface1).Padding(0, 2),
		ButtonActive: lipgloss.NewStyle().Bold(true).Foreground(t.Base).Background(t.Primary).Padding(0, 2),
		Input:        lipgloss.NewStyle().Foreground(t.Text).Background(t.Surface0).Padding(0, 1),
		InputFocused: lipgloss.NewStyle().Foreground(t.Text).Background(t.Surface1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(t.Primary).Padding(0, 1),

		Help:      lipgloss.NewStyle().Foreground(t.Overlay),
		StatusBar: lipgloss.NewStyle().Foreground(t.Subtext).Background(t.Surface0).Padding(0, 1),
	}

	if t == Plain {
		styles.ListSelected = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
		styles.Warning = styles.Warning.Underline(true)
		styles.Error = styles.Error.Underline(true)
	}

	return styles
}

// DefaultStyles returns styles for the current theme.
func DefaultStyles() Styles {
	return NewStyles(Current())
}

// Gradient returns a slice of colors for gradient effects.
func (t Theme) Gradient(steps int) []lipgloss.Color {
	colors := []lipgloss.Color{t.Blue, t.Sapphire, t.Lavender, t.Mauve, t.Pink}
	if steps <= len(colors) {
		return colors[:steps]
	}
	result := make([]lipgloss.Color, steps)
	for i := range result {
		result[i] = colors[i%len(colors)]
	}
	return result
}
