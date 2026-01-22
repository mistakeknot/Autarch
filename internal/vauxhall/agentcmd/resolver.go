package agentcmd

import (
	"strings"

	pconfig "github.com/mistakeknot/vauxpraudemonium/internal/praude/config"
	vconfig "github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/config"
)

// Resolver finds agent commands based on config with sensible fallbacks.
type Resolver struct {
	cfg *vconfig.Config
}

func NewResolver(cfg *vconfig.Config) *Resolver {
	return &Resolver{cfg: cfg}
}

// Resolve returns the command and args for a given agent type and project path.
func (r *Resolver) Resolve(agentType, projectPath string) (string, []string) {
	key := strings.ToLower(agentType)
	if r.cfg != nil && r.cfg.Agents != nil {
		if cmd, ok := r.cfg.Agents[key]; ok && cmd.Command != "" {
			return cmd.Command, cmd.Args
		}
	}

	if projectPath != "" {
		if pcfg, err := pconfig.LoadFromRoot(projectPath); err == nil {
			if ap, ok := pcfg.Agents[key]; ok && ap.Command != "" {
				return ap.Command, ap.Args
			}
		}
	}

	if key == "codex" {
		return "codex", nil
	}
	return "claude", nil
}
