package serve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mistakeknot/autarch/internal/homeask"
)

// probeTimeout bounds the serve probe so a dead serve costs seconds, not a minute.
const probeTimeout = 3 * time.Second

// URLFromEnv is where `autarch serve` listens (AUTARCH_SERVE_URL, default the serve address).
func URLFromEnv() string {
	if u := os.Getenv("AUTARCH_SERVE_URL"); u != "" {
		return u
	}
	return "http://" + DefaultAddr
}

// TokenPathFromEnv is serve's bearer token (AUTARCH_SERVE_TOKEN_FILE, default ~/.autarch/serve.token).
func TokenPathFromEnv() string {
	if p := os.Getenv("AUTARCH_SERVE_TOKEN_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".autarch", "serve.token")
}

// CheckFiling is the homeask.ProjectCheck every filing path installs: serve must be up and the ask's
// root must be the root it resolves for the ask's project, or Home would show the card display-only.
// It runs before anything is created.
func CheckFiling(project, root string) error {
	ps, err := FetchProjects(URLFromEnv(), TokenPathFromEnv(), probeTimeout)
	if err != nil {
		hint := ""
		if errors.Is(err, ErrNotRunning) {
			hint = "; start `autarch serve` (see AGENTS.md)"
		}
		return fmt.Errorf("%w: %v%s: nothing was filed", homeask.ErrHomeDown, err, hint)
	}
	if err := CheckAsk(ps, project, root); err != nil {
		return fmt.Errorf("%w: %v", homeask.ErrInvalid, err)
	}
	return nil
}
