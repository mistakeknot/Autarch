package agent

import (
	"os"
	"path/filepath"

	"github.com/mistakeknot/vauxpraudemonium/pkg/agenttargets"
)

func ResolveTarget(projectRoot, name string) (agenttargets.ResolvedTarget, error) {
	globalPath := ""
	if configDir, err := os.UserConfigDir(); err == nil {
		globalPath = filepath.Join(configDir, "vauxpraudemonium", "agents.toml")
	}
	globalReg, projectReg, err := agenttargets.Load(globalPath, projectRoot)
	if err != nil {
		return agenttargets.ResolvedTarget{}, err
	}
	resolver := agenttargets.NewResolver(globalReg, projectReg)
	return resolver.Resolve(agenttargets.ProjectContext, name)
}
