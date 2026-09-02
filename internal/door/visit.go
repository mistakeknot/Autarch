package door

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The last-visit stamp: the one client-local fact the door keeps. It is what
// makes "since the last visit" a window rather than a guess -- Stellaris's
// Empire Timeline is the lineage, the bridge between parking a session and
// loading it. It is preference-class local state (same class as pins), not
// world state: the estate's truth lives in the gardens and is re-read every
// open; this file only remembers when mk was last here.

// visitFile is the stamp's name under ~/.autarch.
const visitFile = "last-visit"

// firstVisitWindow is how far back the first-ever briefing looks. With no
// stamp there is no window, and "everything, ever" is not a briefing.
const firstVisitWindow = 24 * time.Hour

// DefaultVisitPath is ~/.autarch/last-visit.
func DefaultVisitPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".autarch", visitFile)
}

// LoadLastVisit reads the stamp. An absent file is ok=false with no error --
// the first visit. A file that exists but does not parse is an error, so a
// corrupted stamp is reported rather than silently treated as a first visit.
func LoadLastVisit(path string) (time.Time, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("last-visit stamp %s: %w", path, err)
	}
	return t, true, nil
}

// SaveVisit writes the stamp. Called on quit: a completed walk closes the
// window. If the write fails the previous stamp stands and the next window
// is wider, never narrower -- the failure mode shows more, not less.
func SaveVisit(path string, t time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(t.UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Window resolves the briefing window. An explicit override (--since) wins
// and never touches the stamp; otherwise the stamp; otherwise the first-visit
// default. source names which, for the header. A stamp that exists but could
// not be read falls back to the first-visit window *and* returns the error,
// because a silent fallback would look exactly like a real first visit. A bad
// override returns a zero time with the error, which callers treat as fatal.
func Window(override, visitPath string, now time.Time) (since time.Time, source string, err error) {
	if override != "" {
		t, perr := parseWindow(override, now)
		if perr != nil {
			return time.Time{}, "", perr
		}
		return t, "--since " + override, nil
	}
	t, ok, lerr := LoadLastVisit(visitPath)
	switch {
	case lerr != nil:
		return now.Add(-firstVisitWindow), "first visit", lerr
	case ok:
		return t, "last visit", nil
	default:
		return now.Add(-firstVisitWindow), "first visit", nil
	}
}

// parseWindow accepts a duration (Go syntax, plus a d suffix for days) or an
// RFC3339 instant.
func parseWindow(s string, now time.Time) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if strings.HasSuffix(s, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil && n > 0 {
			return now.Add(-time.Duration(n) * 24 * time.Hour), nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return time.Time{}, fmt.Errorf("--since %q: want a duration like 36h or 3d, or an RFC3339 time", s)
	}
	return now.Add(-d), nil
}
