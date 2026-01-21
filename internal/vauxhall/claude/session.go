package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

// claudeDirNameRegex matches any character that's not alphanumeric or hyphen
// Claude Code replaces all such characters with hyphens in project directory names
var claudeDirNameRegex = regexp.MustCompile(`[^a-zA-Z0-9-]`)

// ConvertToClaudeDirName converts a filesystem path to Claude's directory naming format.
// Claude Code replaces all non-alphanumeric characters (except hyphens) with hyphens.
// Example: /Users/master/Code cloud/!Project → -Users-master-Code-cloud--Project
func ConvertToClaudeDirName(path string) string {
	return claudeDirNameRegex.ReplaceAllString(path, "-")
}

// ClaudeProject represents a project entry in Claude's config
type ClaudeProject struct {
	LastSessionID string `json:"lastSessionId"`
}

// ClaudeConfig represents the structure of .claude.json
type ClaudeConfig struct {
	Projects map[string]ClaudeProject `json:"projects"`
}

// SessionInfo contains Claude session information for a project
type SessionInfo struct {
	SessionID   string    `json:"session_id"`
	ProjectPath string    `json:"project_path"`
	DetectedAt  time.Time `json:"detected_at"`
}

// Session info cache
var (
	sessionCacheMu    sync.RWMutex
	sessionCache      = make(map[string]*SessionInfo) // projectPath -> SessionInfo
	sessionCacheTime  time.Time
	sessionCacheTTL   = 30 * time.Second
)

// GetClaudeConfigDir returns the Claude configuration directory
func GetClaudeConfigDir() string {
	// Check environment variable first
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}

	// Default to ~/.claude
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// GetSessionID returns the Claude session ID for a project path (cached)
func GetSessionID(projectPath string) (string, error) {
	// Check cache first
	sessionCacheMu.RLock()
	if cached, ok := sessionCache[projectPath]; ok {
		if time.Since(sessionCacheTime) < sessionCacheTTL {
			sessionCacheMu.RUnlock()
			return cached.SessionID, nil
		}
	}
	sessionCacheMu.RUnlock()

	// Read from config file
	configDir := GetClaudeConfigDir()
	if configDir == "" {
		return "", nil
	}

	configPath := filepath.Join(configDir, ".claude.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var config ClaudeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return "", err
	}

	// Convert project path to Claude's naming format
	claudeDirName := ConvertToClaudeDirName(projectPath)

	// Look up the project
	project, ok := config.Projects[claudeDirName]
	if !ok {
		return "", nil
	}

	// Update cache
	sessionCacheMu.Lock()
	sessionCache[projectPath] = &SessionInfo{
		SessionID:   project.LastSessionID,
		ProjectPath: projectPath,
		DetectedAt:  time.Now(),
	}
	sessionCacheTime = time.Now()
	sessionCacheMu.Unlock()

	return project.LastSessionID, nil
}

// GetAllSessions returns all known Claude sessions from the config
func GetAllSessions() ([]SessionInfo, error) {
	configDir := GetClaudeConfigDir()
	if configDir == "" {
		return nil, nil
	}

	configPath := filepath.Join(configDir, ".claude.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var config ClaudeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	sessions := make([]SessionInfo, 0, len(config.Projects))
	for dirName, project := range config.Projects {
		if project.LastSessionID == "" {
			continue
		}
		sessions = append(sessions, SessionInfo{
			SessionID:   project.LastSessionID,
			ProjectPath: dirName, // This is the Claude-formatted path
			DetectedAt:  time.Now(),
		})
	}

	// Sort by project path for consistent ordering
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ProjectPath < sessions[j].ProjectPath
	})

	return sessions, nil
}

// MCPInfo contains MCP server information for a session
type MCPInfo struct {
	Global  []string `json:"global"`  // From global mcpServers config
	Project []string `json:"project"` // From project-specific mcpServers
	Local   []string `json:"local"`   // From .mcp.json files
}

// HasAny returns true if any MCPs are configured
func (m *MCPInfo) HasAny() bool {
	return len(m.Global) > 0 || len(m.Project) > 0 || len(m.Local) > 0
}

// Total returns total number of MCPs across all sources
func (m *MCPInfo) Total() int {
	return len(m.Global) + len(m.Project) + len(m.Local)
}

// AllNames returns a deduplicated, sorted list of all MCP names
func (m *MCPInfo) AllNames() []string {
	seen := make(map[string]bool)
	for _, name := range m.Global {
		seen[name] = true
	}
	for _, name := range m.Project {
		seen[name] = true
	}
	for _, name := range m.Local {
		seen[name] = true
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// claudeConfigForMCP is used for parsing MCP-related fields from .claude.json
type claudeConfigForMCP struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
	Projects   map[string]struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	} `json:"projects"`
}

// projectMCPConfig is used for parsing .mcp.json files
type projectMCPConfig struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

// MCP info cache
var (
	mcpInfoCache   = make(map[string]*MCPInfo)
	mcpInfoCacheMu sync.RWMutex
	mcpCacheTTL    = 30 * time.Second
	mcpCacheTimes  = make(map[string]time.Time)
)

// GetMCPInfo retrieves MCP server information for a project path (cached)
func GetMCPInfo(projectPath string) *MCPInfo {
	// Check cache first
	mcpInfoCacheMu.RLock()
	if cached, ok := mcpInfoCache[projectPath]; ok {
		if time.Since(mcpCacheTimes[projectPath]) < mcpCacheTTL {
			mcpInfoCacheMu.RUnlock()
			return cached
		}
	}
	mcpInfoCacheMu.RUnlock()

	info := &MCPInfo{}

	configDir := GetClaudeConfigDir()
	if configDir == "" {
		return info
	}

	// Read global config
	configPath := filepath.Join(configDir, ".claude.json")
	data, err := os.ReadFile(configPath)
	if err == nil {
		var config claudeConfigForMCP
		if json.Unmarshal(data, &config) == nil {
			// Global MCPs
			for name := range config.MCPServers {
				info.Global = append(info.Global, name)
			}
			sort.Strings(info.Global)

			// Project-specific MCPs
			claudeDirName := ConvertToClaudeDirName(projectPath)
			if proj, ok := config.Projects[claudeDirName]; ok {
				for name := range proj.MCPServers {
					info.Project = append(info.Project, name)
				}
				sort.Strings(info.Project)
			}
		}
	}

	// Read local .mcp.json (walk up from project path)
	info.Local = findLocalMCPs(projectPath)

	// Update cache
	mcpInfoCacheMu.Lock()
	mcpInfoCache[projectPath] = info
	mcpCacheTimes[projectPath] = time.Now()
	mcpInfoCacheMu.Unlock()

	return info
}

// findLocalMCPs walks up from the project path looking for .mcp.json files
func findLocalMCPs(projectPath string) []string {
	var mcps []string
	seen := make(map[string]bool)

	current := projectPath
	for {
		mcpPath := filepath.Join(current, ".mcp.json")
		data, err := os.ReadFile(mcpPath)
		if err == nil {
			var config projectMCPConfig
			if json.Unmarshal(data, &config) == nil {
				for name := range config.MCPServers {
					if !seen[name] {
						seen[name] = true
						mcps = append(mcps, name)
					}
				}
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			break // Reached root
		}
		current = parent
	}

	sort.Strings(mcps)
	return mcps
}
