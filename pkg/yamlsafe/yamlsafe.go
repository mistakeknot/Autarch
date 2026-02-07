package yamlsafe

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const DefaultMaxBytes = int64(1 << 20) // 1 MiB

// ReadFile loads a YAML file with safety guards.
func ReadFile(path string) ([]byte, error) {
	return readFileWithLimit(path, DefaultMaxBytes)
}

// UnmarshalFile loads and unmarshals a YAML file with safety guards.
// It returns the raw bytes for callers that need hashing/fingerprints.
func UnmarshalFile(path string, out interface{}) ([]byte, error) {
	data, err := ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("yaml decode failed: %w", err)
	}
	return data, nil
}

func readFileWithLimit(path string, limit int64) ([]byte, error) {
	meta, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if meta.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to read symlinked yaml: %s", path)
	}
	if !meta.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing to read non-regular yaml: %s", path)
	}
	if meta.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("refusing to read writable yaml: %s", path)
	}
	if meta.Size() > limit {
		return nil, fmt.Errorf("yaml file too large: %d bytes (limit %d)", meta.Size(), limit)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("yaml file too large: %d bytes (limit %d)", len(data), limit)
	}
	return data, nil
}
