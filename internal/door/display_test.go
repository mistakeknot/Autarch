package door

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestDisplayDensitySwitchesAndSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "display.yaml")
	m := catchupFixture().WithDisplay(path)
	if m.density != DensityCozy {
		t.Fatal("first visit should be cozy")
	}
	m, _ = press(m, "d")
	if m.density != DensityCompact || !strings.Contains(m.View(), "Compact") {
		t.Fatal(m.View())
	}
	if got := catchupFixture().WithDisplay(path); got.density != DensityCompact {
		t.Fatal("choice lost on reopen")
	}
	if err := os.WriteFile(path, []byte("density: compact\nfuture_preference: keep\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, _ = press(m, "d")
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "future_preference: keep") {
		t.Fatal("unrelated preference overwritten")
	}
}

func TestDisplayRangeCanBeChosenWithoutArguments(t *testing.T) {
	m := catchupFixture()
	opening := m.window
	m, _ = press(m, "w")
	if m.menu != "range" || !strings.Contains(m.View(), "Last 24 hours") {
		t.Fatal(m.View())
	}
	m, _ = press(m, "j")
	m, cmd := press(m, "enter")
	if cmd == nil || !m.window.Equal(m.now().Add(-24*time.Hour)) || m.menu != "" {
		t.Fatal("range not applied")
	}
	m.moveRemaining = 0
	m.movements = map[string]Movement{m.projects[0].Root: {Root: m.projects[0].Root}}
	m, _ = press(m, "w")
	m, _ = press(m, "home")
	m, _ = press(m, "enter")
	if !m.window.Equal(opening) {
		t.Fatal("opening window lost")
	}
}

func TestDisplayRangeQueuesWhileReading(t *testing.T) {
	m := catchupFixture()
	m.moveRemaining = 1
	original := m.window
	m, _ = press(m, "w")
	m, _ = press(m, "j")
	m, cmd := press(m, "enter")
	if cmd != nil || !m.window.Equal(original) || m.pendingRange == nil {
		t.Fatal("overlapping read started")
	}
	x, cmd := m.Update(movementMsg{m: Movement{Root: m.projects[0].Root, Since: original}})
	m = x.(Model)
	if cmd == nil || m.pendingRange != nil || !m.window.Equal(m.now().Add(-24*time.Hour)) {
		t.Fatal("queued range not applied")
	}
}

func TestDisplayModesFitAndRetainEvidence(t *testing.T) {
	for _, size := range [][2]int{{120, 38}, {80, 26}, {40, 16}} {
		for _, density := range []Density{DensityCozy, DensityCompact} {
			m := catchupFixture()
			m.width, m.height, m.density = size[0], size[1], density
			for _, key := range []string{"", "a", "enter", "esc", "w"} {
				if key != "" {
					m, _ = press(m, key)
				}
				lines := strings.Split(m.View(), "\n")
				if len(lines) > m.height {
					t.Fatalf("height overflow: %d", len(lines))
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > m.width {
						t.Fatalf("width overflow: %q", line)
					}
				}
			}
		}
	}
}

func TestDisplayNarrowRangeKeepsSelectedOptionVisible(t *testing.T) {
	m := catchupFixture()
	m.width, m.height = 40, 16
	m, _ = press(m, "w")
	m, _ = press(m, "G")
	if !strings.Contains(m.View(), "› ● Last 30 days") {
		t.Fatal(m.View())
	}
	x, cmd := m.Update(tea.MouseMsg{X: 10, Y: 14, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if cmd != nil || x.(Model).menu != "range" {
		t.Fatal("footer click applied an offscreen choice")
	}
}

func TestDisplayLateStartMetadataCannotReopenCompletedRead(t *testing.T) {
	m := catchupFixture()
	x, _ := m.chooseRange(1)
	m = x.(Model)
	x, _ = m.Update(movementMsg{m: Movement{Root: m.projects[0].Root, Since: m.window}, generation: m.moveGeneration})
	m = x.(Model)
	x, _ = m.Update(movementsStartedMsg{count: 1, generation: m.moveGeneration})
	m = x.(Model)
	if m.moveRemaining != 0 {
		t.Fatal("completed read became pending again")
	}
	x, cmd := m.chooseRange(2)
	m = x.(Model)
	if cmd == nil || m.pendingRange != nil {
		t.Fatal("next range stuck behind a completed read")
	}
	x, _ = m.Update(movementsStartedMsg{count: 1, generation: m.moveGeneration - 1, sessionsErr: errors.New("old error")})
	if x.(Model).sessionsErr != nil {
		t.Fatal("old metadata overwrote the new read")
	}
}

func TestDisplayActiveThreadsTabPreservesBack(t *testing.T) {
	m := catchupFixture()
	m, _ = press(m, "4")
	m, _ = press(m, "4")
	m, _ = press(m, "t")
	if m.screen != screenBriefing {
		t.Fatal("active tab broke return navigation")
	}
}
