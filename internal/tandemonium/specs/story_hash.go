package specs

import (
	"crypto/sha256"
	"encoding/hex"
)

func StoryHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:8]
}
