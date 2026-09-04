package theme

import "testing"

func TestSemanticPalette(t *testing.T) {
	themes := map[string]Theme{
		"TokyoNight": TokyoNight,
		"Mocha":      CatppuccinMocha,
		"Macchiato":  CatppuccinMacchiato,
		"Nord":       Nord,
	}

	for name, th := range themes {
		t.Run(name, func(t *testing.T) {
			p := th.Semantic()
			// Every dark theme should have a non-empty BgPrimary
			if p.BgPrimary == "" {
				t.Errorf("%s: BgPrimary is empty", name)
			}
			if p.FgPrimary == "" {
				t.Errorf("%s: FgPrimary is empty", name)
			}
			if p.Interactive == "" {
				t.Errorf("%s: Interactive is empty", name)
			}
			if p.StatusSuccess == "" {
				t.Errorf("%s: StatusSuccess is empty", name)
			}
			if p.AgentClaude == "" {
				t.Errorf("%s: AgentClaude is empty", name)
			}
		})
	}
}

func TestAgentColor(t *testing.T) {
	p := TokyoNight.Semantic()

	tests := []struct {
		input string
		want  string
	}{
		{"claude", string(TokyoNight.Claude)},
		{"cc", string(TokyoNight.Claude)},
		{"codex", string(TokyoNight.Codex)},
		{"cod", string(TokyoNight.Codex)},
		{"gemini", string(TokyoNight.Gemini)},
		{"gmi", string(TokyoNight.Gemini)},
		{"aider", string(TokyoNight.Aider)},
		{"cursor", string(TokyoNight.Cursor)},
		{"user", string(TokyoNight.User)},
		{"unknown", string(p.AgentUnknown)},
		{"", string(p.AgentUnknown)},
	}

	for _, tt := range tests {
		got := string(p.AgentColor(tt.input))
		if got != tt.want {
			t.Errorf("AgentColor(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestStatusColor(t *testing.T) {
	p := TokyoNight.Semantic()

	tests := []struct {
		input string
		want  string
	}{
		{"success", string(p.StatusSuccess)},
		{"ok", string(p.StatusSuccess)},
		{"complete", string(p.StatusSuccess)},
		{"completed", string(p.StatusSuccess)},
		{"done", string(p.StatusSuccess)},
		{"warning", string(p.StatusWarning)},
		{"warn", string(p.StatusWarning)},
		{"attention", string(p.StatusWarning)},
		{"error", string(p.StatusError)},
		{"fail", string(p.StatusError)},
		{"failed", string(p.StatusError)},
		{"failure", string(p.StatusError)},
		{"info", string(p.StatusInfo)},
		{"information", string(p.StatusInfo)},
		{"pending", string(p.StatusPending)},
		{"running", string(p.StatusPending)},
		{"in_progress", string(p.StatusPending)},
		{"working", string(p.StatusPending)},
		{"idle", string(p.StatusIdle)},
		{"inactive", string(p.StatusIdle)},
		{"waiting", string(p.StatusIdle)},
		{"disabled", string(p.StatusDisabled)},
		{"unavailable", string(p.StatusDisabled)},
		{"other", string(p.FgSecondary)},
	}

	for _, tt := range tests {
		got := string(p.StatusColor(tt.input))
		if got != tt.want {
			t.Errorf("StatusColor(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestSemantic(t *testing.T) {
	t.Setenv("AUTARCH_NO_COLOR", "0")
	t.Setenv("AUTARCH_THEME", "tokyo")
	resetAutoTheme()

	p := Semantic()
	if p.BgPrimary != TokyoNight.Base {
		t.Errorf("Semantic().BgPrimary = %s, want %s", p.BgPrimary, TokyoNight.Base)
	}
}

func TestNewSemanticStyles(t *testing.T) {
	styles := NewSemanticStyles(TokyoNight)

	t.Run("TextPrimary", func(t *testing.T) {
		if styles.TextPrimary.Render("x") == "" {
			t.Error("TextPrimary rendered empty")
		}
	})

	t.Run("BadgeClaude", func(t *testing.T) {
		if styles.BadgeClaude.Render("C") == "" {
			t.Error("BadgeClaude rendered empty")
		}
	})

	t.Run("Selected", func(t *testing.T) {
		if styles.Selected.Render("x") == "" {
			t.Error("Selected rendered empty")
		}
	})

	t.Run("Card", func(t *testing.T) {
		if styles.Card.Render("x") == "" {
			t.Error("Card rendered empty")
		}
	})

	t.Run("InputError", func(t *testing.T) {
		if styles.InputError.Render("x") == "" {
			t.Error("InputError rendered empty")
		}
	})
}

func TestNewSemanticStylesPlainTheme(t *testing.T) {
	styles := NewSemanticStyles(Plain)
	// Plain theme should use Reverse for Selected (accessibility guard rail)
	rendered := styles.Selected.Render("x")
	if rendered == "" {
		t.Error("Plain Selected rendered empty")
	}
}

func TestDefaultSemanticStyles(t *testing.T) {
	t.Setenv("AUTARCH_THEME", "tokyo")
	resetAutoTheme()
	styles := DefaultSemanticStyles()
	if styles.TextPrimary.Render("test") == "" {
		t.Error("DefaultSemanticStyles returned empty TextPrimary")
	}
}

func TestSemanticPaletteConsistency(t *testing.T) {
	// Verify Tokyo Night semantic mappings match the raw palette
	p := TokyoNight.Semantic()
	if p.Interactive != TokyoNight.Primary {
		t.Errorf("Interactive=%s, want Primary=%s", p.Interactive, TokyoNight.Primary)
	}
	if p.StatusSuccess != TokyoNight.Success {
		t.Errorf("StatusSuccess=%s, want Success=%s", p.StatusSuccess, TokyoNight.Success)
	}
	if p.BorderDefault != TokyoNight.Surface2 {
		t.Errorf("BorderDefault=%s, want Surface2=%s", p.BorderDefault, TokyoNight.Surface2)
	}
	if p.AgentClaude != TokyoNight.Claude {
		t.Errorf("AgentClaude=%s, want Claude=%s", p.AgentClaude, TokyoNight.Claude)
	}
}
