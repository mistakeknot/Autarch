package research

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func Create(dir, id string, now time.Time) (string, error) {
	name := fmt.Sprintf("%s-%s.md", id, now.UTC().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	body := fmt.Sprintf(`# Research for %s

## Market Summary
- Claim: "Replace with a specific market claim."
  - Evidence refs:
    - path: ".praude/research/%s-YYYYMMDD-HHMMSS.md"
    - anchor: "section-1"
    - note: "Source quote"

## Competitive Analysis
- Claim: "Replace with a specific competitor claim."
  - Evidence refs:
    - path: ".praude/research/%s-YYYYMMDD-HHMMSS.md"
    - anchor: "section-2"
    - note: "Source quote"

## OSS project scan
- Project: "Name the OSS project"
  - learnings: "Key takeaways"
  - bootstrapping: "What can be reused"
  - insights: "Product or UX notes"
  - Evidence refs:
    - path: ".praude/research/%s-YYYYMMDD-HHMMSS.md"
    - anchor: "section-3"
    - note: "Source quote"
`, id, id, id, id)
	return path, os.WriteFile(path, []byte(body), 0o644)
}
