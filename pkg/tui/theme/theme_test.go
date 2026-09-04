package theme

import (
	"os"
	"testing"
)

// withDetector replaces detectDarkBackground for the duration of a test
// and resets the auto-theme cache.
func withDetector(t *testing.T, fn func() bool) {
	t.Helper()
	orig := detectDarkBackground
	detectDarkBackground = fn
	resetAutoTheme()
	t.Cleanup(func() {
		detectDarkBackground = orig
		resetAutoTheme()
	})
}

func TestCurrentAutoUsesDarkThemeWhenBackgroundIsDark(t *testing.T) {
	t.Setenv("AUTARCH_NO_COLOR", "0")
	t.Setenv("AUTARCH_THEME", "")
	withDetector(t, func() bool { return true })

	theme := Current()
	if theme.Base != TokyoNight.Base {
		t.Errorf("dark background: got Base=%s, want %s", theme.Base, TokyoNight.Base)
	}
}

func TestCurrentAutoUsesLightThemeWhenBackgroundIsLight(t *testing.T) {
	t.Setenv("AUTARCH_NO_COLOR", "0")
	t.Setenv("AUTARCH_THEME", "")
	withDetector(t, func() bool { return false })

	theme := Current()
	if theme.Base != CatppuccinLatte.Base {
		t.Errorf("light background: got Base=%s, want %s", theme.Base, CatppuccinLatte.Base)
	}
}

func TestCurrentRespectsExplicitThemeOverrides(t *testing.T) {
	t.Setenv("AUTARCH_NO_COLOR", "0")
	tests := []struct {
		env  string
		want Theme
	}{
		{"mocha", CatppuccinMocha},
		{"macchiato", CatppuccinMacchiato},
		{"nord", Nord},
		{"latte", CatppuccinLatte},
		{"light", CatppuccinLatte},
		{"tokyo", TokyoNight},
		{"tokyo-night", TokyoNight},
		{"plain", Plain},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv("AUTARCH_THEME", tt.env)
			resetAutoTheme()
			got := Current()
			if got.Base != tt.want.Base {
				t.Errorf("AUTARCH_THEME=%s: got Base=%s, want %s", tt.env, got.Base, tt.want.Base)
			}
		})
	}
}

func TestThemeColors(t *testing.T) {
	// Verify TokyoNight has the expected primary color
	if TokyoNight.Primary != "#7aa2f7" {
		t.Errorf("TokyoNight.Primary = %s, want #7aa2f7", TokyoNight.Primary)
	}
	if TokyoNight.Claude != "#e07353" {
		t.Errorf("TokyoNight.Claude = %s, want #e07353", TokyoNight.Claude)
	}
	if TokyoNight.Codex != "#00D4AA" {
		t.Errorf("TokyoNight.Codex = %s, want #00D4AA", TokyoNight.Codex)
	}
}

func TestNewStyles(t *testing.T) {
	styles := NewStyles(TokyoNight)
	// Verify styles are populated (non-zero)
	rendered := styles.Title.Render("test")
	if rendered == "" {
		t.Error("Title style rendered empty string")
	}
}

func TestDefaultStyles(t *testing.T) {
	t.Setenv("AUTARCH_THEME", "tokyo")
	resetAutoTheme()
	styles := DefaultStyles()
	rendered := styles.Header.Render("test")
	if rendered == "" {
		t.Error("DefaultStyles().Header rendered empty string")
	}
}

func TestGradient(t *testing.T) {
	g := TokyoNight.Gradient(3)
	if len(g) != 3 {
		t.Errorf("Gradient(3) returned %d colors, want 3", len(g))
	}
	if g[0] != TokyoNight.Blue {
		t.Errorf("Gradient[0] = %s, want %s", g[0], TokyoNight.Blue)
	}

	// More than 5 wraps
	g = TokyoNight.Gradient(7)
	if len(g) != 7 {
		t.Errorf("Gradient(7) returned %d colors, want 7", len(g))
	}
}

func TestNoColorEnabled(t *testing.T) {
	// "default" case: NO_COLOR must be truly absent. os.Unsetenv is the
	// only reliable way; t.Setenv("NO_COLOR","") still sets it (presence=true).
	t.Run("default", func(t *testing.T) {
		t.Setenv("AUTARCH_NO_COLOR", "")
		t.Setenv("NO_COLOR", "") // register restoration before removing it
		os.Unsetenv("NO_COLOR")
		if got := NoColorEnabled(); got != false {
			t.Errorf("NoColorEnabled() = %v, want false", got)
		}
	})

	t.Run("NO_COLOR set", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("AUTARCH_NO_COLOR", "")
		if got := NoColorEnabled(); got != true {
			t.Errorf("NoColorEnabled() = %v, want true", got)
		}
	})

	t.Run("NO_COLOR empty string still counts", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("AUTARCH_NO_COLOR", "")
		if got := NoColorEnabled(); got != true {
			t.Errorf("NoColorEnabled() = %v, want true (presence = disabled)", got)
		}
	})

	t.Run("AUTARCH_NO_COLOR=0 overrides NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("AUTARCH_NO_COLOR", "0")
		if got := NoColorEnabled(); got != false {
			t.Errorf("NoColorEnabled() = %v, want false", got)
		}
	})

	t.Run("AUTARCH_NO_COLOR=false overrides NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("AUTARCH_NO_COLOR", "false")
		if got := NoColorEnabled(); got != false {
			t.Errorf("NoColorEnabled() = %v, want false", got)
		}
	})

	t.Run("AUTARCH_NO_COLOR=1", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		os.Unsetenv("NO_COLOR")
		t.Setenv("AUTARCH_NO_COLOR", "1")
		if got := NoColorEnabled(); got != true {
			t.Errorf("NoColorEnabled() = %v, want true", got)
		}
	})
}

func TestCurrentReturnsPlainWhenNoColorEnabled(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("AUTARCH_NO_COLOR", "")
	resetAutoTheme()
	theme := Current()
	if theme != Plain {
		t.Error("expected Plain theme when NO_COLOR is set")
	}
}

func TestPlainThemeHasEmptyColors(t *testing.T) {
	if Plain.Base != "" {
		t.Errorf("Plain.Base = %q, want empty", Plain.Base)
	}
	if Plain.Primary != "" {
		t.Errorf("Plain.Primary = %q, want empty", Plain.Primary)
	}
}

func TestAutoThemeFallsBackToDarkOnPanic(t *testing.T) {
	t.Setenv("AUTARCH_NO_COLOR", "0")
	t.Setenv("AUTARCH_THEME", "")
	withDetector(t, func() bool { panic("simulated panic") })
	theme := Current()
	if theme.Base != TokyoNight.Base {
		t.Errorf("panic fallback: got Base=%s, want %s (TokyoNight)", theme.Base, TokyoNight.Base)
	}
}

func TestDetectDarkBackgroundSkipsOSCOverSSH(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "192.168.1.1 12345 192.168.1.2 22")
	// Should return true (dark) immediately without OSC query
	result := detectDarkBackground()
	if !result {
		t.Error("expected dark=true over SSH")
	}
}

func TestNewStylesPlainTheme(t *testing.T) {
	styles := NewStyles(Plain)
	// Plain theme should have Reverse on ListSelected for accessibility
	rendered := styles.ListSelected.Render("x")
	if rendered == "" {
		t.Error("Plain ListSelected rendered empty")
	}
}

func TestFromNamePlainVariants(t *testing.T) {
	for _, name := range []string{"plain", "none", "no-color", "nocolor"} {
		t.Setenv("AUTARCH_NO_COLOR", "")
		t.Setenv("NO_COLOR", "")
		// Manually unset NO_COLOR by not including it — but t.Setenv sets it.
		// The FromName function checks the name directly for these aliases.
		got := FromName(name)
		if got != Plain {
			t.Errorf("FromName(%q) returned non-Plain theme", name)
		}
	}
}
