package contract

// SchemaVersion tracks the contract schema version for migration purposes.
const SchemaVersion = "1.0.0"

// SchemaVersionMajor, Minor, Patch components
const (
	SchemaVersionMajor = 1
	SchemaVersionMinor = 0
	SchemaVersionPatch = 0
)

// VersionInfo provides schema version metadata
type VersionInfo struct {
	Version string `json:"version"`
	Major   int    `json:"major"`
	Minor   int    `json:"minor"`
	Patch   int    `json:"patch"`
}

// GetVersionInfo returns the current schema version info
func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version: SchemaVersion,
		Major:   SchemaVersionMajor,
		Minor:   SchemaVersionMinor,
		Patch:   SchemaVersionPatch,
	}
}

// IsCompatible checks if the given version is compatible with the current schema.
// Returns true if the major version matches.
func IsCompatible(version string) bool {
	info := parseVersion(version)
	return info.Major == SchemaVersionMajor
}

// parseVersion parses a semver string into VersionInfo
func parseVersion(version string) VersionInfo {
	var major, minor, patch int
	// Simple parsing for semver format "X.Y.Z"
	n := scanVersion(version, &major, &minor, &patch)
	if n < 1 {
		return VersionInfo{Version: version}
	}
	return VersionInfo{
		Version: version,
		Major:   major,
		Minor:   minor,
		Patch:   patch,
	}
}

func scanVersion(s string, major, minor, patch *int) int {
	// Manual parsing to avoid fmt.Sscanf overhead
	i := 0
	count := 0

	// Parse major
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > start {
		*major = atoi(s[start:i])
		count++
	}
	if i >= len(s) || s[i] != '.' {
		return count
	}
	i++ // skip '.'

	// Parse minor
	start = i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > start {
		*minor = atoi(s[start:i])
		count++
	}
	if i >= len(s) || s[i] != '.' {
		return count
	}
	i++ // skip '.'

	// Parse patch
	start = i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > start {
		*patch = atoi(s[start:i])
		count++
	}

	return count
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
