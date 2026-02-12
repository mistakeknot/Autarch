package agenttargets

import "time"

// BackendType identifies how agents are dispatched.
type BackendType string

const (
	// BackendSubscriptionCLI dispatches via CLI process (uses subscription billing).
	BackendSubscriptionCLI BackendType = "subscription-cli"
	// BackendAPI dispatches via direct API calls (uses API billing).
	BackendAPI BackendType = "api"
)

// DispatchConfig controls how an agent dispatch is executed.
type DispatchConfig struct {
	PreferredBackend BackendType   // Default: subscription-cli
	PreferredAgent   string        // "claude", "codex", "" (auto-detect)
	Timeout          time.Duration // Max wall-clock time (0 = no limit)
	Sandbox          string        // Sandbox mode: "workspace-write", "read-only", "danger-full-access"
	Model            string        // Model override (empty = CLI default)
	SessionID        string        // For --resume (Claude) or resume (Codex)
	OutputFile       string        // Where to write final output (Codex -o)
	ExtraArgs        []string      // Additional CLI flags
	WorkDir          string        // Working directory override (empty = caller's cwd)
	Verbose          bool          // Enable verbose output
	Print            bool          // Print-only mode (no interactive)
	Env              map[string]string // Extra environment variables for the agent process
}

// DefaultDispatchConfig returns subscription-first defaults: claude preferred, 30m timeout.
func DefaultDispatchConfig() DispatchConfig {
	return DispatchConfig{
		PreferredBackend: BackendSubscriptionCLI,
		Timeout:          30 * time.Minute,
		Verbose:          true,
		Print:            true,
	}
}
