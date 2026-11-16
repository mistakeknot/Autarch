package homeask

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// MaxRootRunScriptBytes bounds the local script snapshot pinned by a card.
const MaxRootRunScriptBytes = 4 << 20

// HashRootRunScript accepts only regular files and bounds the read even when
// a file grows after Stat. Neither filing surface may read devices or FIFOs.
func HashRootRunScript(path string) (string, error) {
	if !validRootScript(path) {
		return "", fmt.Errorf("root-run script must be an absolute path without control characters or parent traversal")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("root-run script is not readable: %w", err)
	}
	check := func(info os.FileInfo) error {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("root-run script must be a regular file")
		}
		if info.Size() > MaxRootRunScriptBytes {
			return fmt.Errorf("root-run script exceeds %d bytes", MaxRootRunScriptBytes)
		}
		return nil
	}
	if err := check(info); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return "", err
	}
	if err := check(info); err != nil {
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(file, MaxRootRunScriptBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > MaxRootRunScriptBytes {
		return "", fmt.Errorf("root-run script exceeds %d bytes", MaxRootRunScriptBytes)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
