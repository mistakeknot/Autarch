package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
)

func StoreAttachment(storeDir, srcPath string) (string, int64, string, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", 0, "", err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	size := int64(len(data))
	mime := http.DetectContentType(data)

	subdir := filepath.Join(storeDir, hash[:2])
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		return "", 0, "", err
	}
	dst := filepath.Join(subdir, hash)
	if _, err := os.Stat(dst); err == nil {
		return hash, size, mime, nil
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", 0, "", err
	}
	return hash, size, mime, nil
}
