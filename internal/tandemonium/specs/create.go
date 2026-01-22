package specs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	fileutil "github.com/mistakeknot/vauxpraudemonium/internal/file"
)

func CreateQuickSpec(dir, raw string, now time.Time) (string, error) {
	id := NewID(dir, now)
	path := filepath.Join(dir, id+".yaml")
	payload := fmt.Sprintf("id: %q\n", id)
	payload += fmt.Sprintf("title: %q\n", firstLine(raw))
	payload += fmt.Sprintf("created_at: %q\n", now.Format(time.RFC3339))
	payload += "status: assigned\n"
	payload += "quick_mode: true\n"
	payload += "summary: |\n"
	payload += "  " + strings.TrimSpace(raw) + "\n\n"
	payload += "  (Quick task - no PM refinement performed)\n"
	return path, fileutil.AtomicWriteFile(path, []byte(payload), 0o644)
}

func firstLine(raw string) string {
	parts := strings.SplitN(strings.TrimSpace(raw), "\n", 2)
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func NewID(dir string, now time.Time) string {
	max := 0
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, "TAND-") {
				continue
			}
			base := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
			if len(base) <= len("TAND-") {
				continue
			}
			num, err := strconv.Atoi(base[len("TAND-"):])
			if err != nil {
				continue
			}
			if num > max {
				max = num
			}
		}
	}
	if max > 0 {
		return fmt.Sprintf("TAND-%03d", max+1)
	}
	return fmt.Sprintf("TAND-%03d", now.Day())
}
