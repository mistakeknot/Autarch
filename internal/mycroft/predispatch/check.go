// Package predispatch runs an optional coordinator-controlled assignment check.
package predispatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// Run executes configured argv directly, inheriting cwd and setting assignment context.
// Empty commands succeed. Errors are bounded to 240 runes; timeout defaults to 30s.
func Run(parent context.Context, check mycroft.PreDispatchCheck, beadID, agent string) error {
	if len(check.Command) == 0 {
		return nil
	}
	timeout := time.Duration(check.Timeout)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	// Inherit the caller's working directory.
	cmd := exec.CommandContext(ctx, check.Command[0], check.Command[1:]...)
	cmd.Env = append(os.Environ(), "MYCROFT_BEAD_ID="+beadID, "MYCROFT_AGENT="+agent)
	err := cmd.Run()
	timedOut := ctx.Err() != nil
	cancel()
	if err != nil {
		name := filepath.Base(check.Command[0])
		if r := []rune(name); len(r) > 60 {
			name = string(r[:57]) + "..."
		}
		detail := err.Error()
		var pathErr *os.PathError
		var execErr *exec.Error
		if errors.As(err, &pathErr) {
			detail = "start error: " + pathErr.Err.Error()
		}
		if errors.As(err, &execErr) {
			detail = "start error: " + execErr.Err.Error()
		}
		reason := fmt.Sprintf("pre_dispatch_check %s: %s", name, detail)
		if timedOut {
			reason = fmt.Sprintf("pre_dispatch_check %s: timeout after %s", name, timeout)
		}
		if r := []rune(reason); len(r) > 240 {
			reason = string(r[:237]) + "..."
		}
		return fmt.Errorf("%s", reason)
	}
	return nil
}
