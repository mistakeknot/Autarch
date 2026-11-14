package homeask

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CheckUI requires explicit proof for explicitly marked UI play and merge cards.
func (f *CardFiler) CheckUI(r CardRequest) error {
	valid := false
	for _, p := range r.Attachments {
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("%w: attach an existing non-empty screenshot or video file: %s", ErrInvalid, p)
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".mp4", ".webm":
			valid = true
		}
	}
	if r.UI && (r.Type == "play" || r.Type == "merge") && !valid {
		return fmt.Errorf("%w: UI %s card needs a screenshot or video attachment; use --attach with an existing non-empty png/jpg/webp/gif/mp4/webm file", ErrInvalid, r.Type)
	}
	return nil
}
