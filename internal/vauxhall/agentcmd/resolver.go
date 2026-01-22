package agentcmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/pkg/agenttargets"
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
	globalPath := ""
	if configDir, err := os.UserConfigDir(); err == nil {
		globalPath = filepath.Join(configDir, "vauxpraudemonium", "agents.toml")
	}
	globalReg, projectReg, err := agenttargets.Load(globalPath, projectPath)
	if err == nil {
		mergedGlobal := agenttargets.Merge(globalReg, registryFromVauxhall(r.cfg))
		resolver := agenttargets.NewResolver(mergedGlobal, projectReg)
		ctx := agenttargets.GlobalContext
		if projectPath != "" {
			ctx = agenttargets.ProjectContext
		}
		if resolved, err := resolver.Resolve(ctx, key); err == nil && resolved.Command != "" {
			return resolved.Command, resolved.Args
		}
	}
	return r.resolveFallback(key, projectPath)
}

func (r *Resolver) resolveFallback(agentType, projectPath string) (string, []string) {
	if r.cfg != nil && r.cfg.Agents != nil {
		if cmd, ok := r.cfg.Agents[agentType]; ok && cmd.Command != "" {
			return cmd.Command, cmd.Args
		}
	}

	if projectPath != "" {
		if pcfg, err := pconfig.LoadFromRoot(projectPath); err == nil {
			if ap, ok := pcfg.Agents[agentType]; ok && ap.Command != "" {
				return ap.Command, ap.Args
			}
		}
	}

	if agentType == "codex" {
		return "codex", nil
	}
	return "claude", nil
}

func registryFromVauxhall(cfg *vconfig.Config) agenttargets.Registry {
	reg := agenttargets.Registry{Targets: map[string]agenttargets.Target{}}
	if cfg == nil || cfg.Agents == nil {
		return reg
	}
	for name, cmd := range cfg.Agents {
		reg.Targets[strings.ToLower(name)] = agenttargets.Target{
			Name:    name,
			Type:    agenttargets.TargetCommand,
			Command: cmd.Command,
			Args:    cmd.Args,
		}
	}
	return reg
}
