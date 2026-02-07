package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestInsertAt(t *testing.T) {
	ansiBase := "\x1b[31mHello\x1b[0m \x1b[32mWorld\x1b[0m"
	ansiRed := "\x1b[31mRed\x1b[0m"

	tests := []struct {
		name        string
		base        string
		col         int
		overlay     string
		want        string
		wantVisible string
		wantWidth   int
		mustContain []string
		mustPrefix  string
	}{
		{
			name:      "plain text overlay in middle",
			base:      "Hello World",
			col:       6,
			overlay:   "Go",
			want:      "Hello Gorld",
			wantWidth: 11,
		},
		{
			name:      "overlay at start",
			base:      "Hello",
			col:       0,
			overlay:   "Hi",
			want:      "Hillo",
			wantWidth: 5,
		},
		{
			name:      "overlay at end",
			base:      "Hello",
			col:       5,
			overlay:   " World",
			want:      "Hello World",
			wantWidth: 11,
		},
		{
			name:      "base shorter than col pads spaces",
			base:      "Hi",
			col:       5,
			overlay:   "X",
			want:      "Hi   X",
			wantWidth: 6,
		},
		{
			name:        "ansi styled base overlay in middle preserves styles",
			base:        ansiBase,
			col:         6,
			overlay:     "Go",
			wantVisible: "Hello Gorld",
			wantWidth:   11,
			mustContain: []string{"\x1b[31mHello\x1b[0m", "\x1b[32mrld\x1b[0m", "Go"},
		},
		{
			name:      "empty overlay no-op",
			base:      "Hello",
			col:       2,
			overlay:   "",
			want:      "Hello",
			wantWidth: 5,
		},
		{
			name:      "empty base",
			base:      "",
			col:       0,
			overlay:   "X",
			want:      "X",
			wantWidth: 1,
		},
		{
			name:      "overlay wider than base",
			base:      "Hi",
			col:       0,
			overlay:   "Hello World",
			want:      "Hello World",
			wantWidth: 11,
		},
		{
			name:      "col equals base width append",
			base:      "Hello",
			col:       5,
			overlay:   "!",
			want:      "Hello!",
			wantWidth: 6,
		},
		{
			name:        "ansi base overlay at col 0 preserves ansi right",
			base:        ansiRed,
			col:         0,
			overlay:     "X",
			wantVisible: "Xed",
			wantWidth:   3,
			mustContain: []string{"\x1b[31med\x1b[0m"},
			mustPrefix:  "X",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := insertAt(tt.base, tt.col, tt.overlay)

			if tt.want != "" && got != tt.want {
				t.Fatalf("insertAt(%q, %d, %q) = %q, want %q", tt.base, tt.col, tt.overlay, got, tt.want)
			}

			if tt.wantVisible != "" {
				gotVisible := ansi.Strip(got)
				if gotVisible != tt.wantVisible {
					t.Fatalf("visible insertAt(%q, %d, %q) = %q, want %q", tt.base, tt.col, tt.overlay, gotVisible, tt.wantVisible)
				}
			}

			if width := ansi.StringWidth(got); width != tt.wantWidth {
				t.Fatalf("ansi.StringWidth(result) = %d, want %d (result=%q)", width, tt.wantWidth, got)
			}

			for _, needle := range tt.mustContain {
				if !strings.Contains(got, needle) {
					t.Fatalf("result %q does not contain %q", got, needle)
				}
			}

			if tt.mustPrefix != "" && !strings.HasPrefix(got, tt.mustPrefix) {
				t.Fatalf("result %q does not have prefix %q", got, tt.mustPrefix)
			}
		})
	}
}
