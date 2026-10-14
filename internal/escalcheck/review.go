package escalcheck

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	maxReview     = 2 << 20
	bindWindow    = 12
	noteWindow    = 200
	maxNotes      = 20
	maxNoteBytes  = 400
	maxNotesTotal = 4 << 10
)

var (
	verdictRe = regexp.MustCompile(`^VERDICT: (PASS|PASS WITH NOTES|FAIL)$`)
	bindPRRe  = regexp.MustCompile(`^REVIEWED-PR: ([A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}#[1-9][0-9]{0,8})$`)
	bindHead  = regexp.MustCompile(`^REVIEWED-HEAD: ([0-9a-f]{40})$`)
	bindSHA   = regexp.MustCompile(`^REVIEWED-SCRIPT-SHA256: ([0-9a-f]{64})$`)
)

func nonEmptyLines(b []byte) []string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// checkReview reads a text review receipt. Only the trailing block counts:
// exact-format REVIEWED-* lines then the verdict as the last line. Narrative
// mentions of a commit anywhere else are ignored. The lines establish what the
// reviewer says it reviewed, not that the reviewer is who it claims to be.
func checkReview(o Options, pr *prEvidence, scriptDigest string) []Item {
	b, _, err := readRegular(o.Review, maxReview)
	if err != nil {
		return []Item{
			item("review.verdict", Unknown, "cannot read the review: %v", err),
			item("review.binds", Unknown, "cannot read the review"),
		}
	}
	lines := nonEmptyLines(b)
	var verdictItem Item
	if len(lines) == 0 {
		return []Item{item("review.verdict", Unknown, "the review is empty"), item("review.binds", Unknown, "the review is empty")}
	}
	m := verdictRe.FindStringSubmatch(lines[len(lines)-1])
	switch {
	case m == nil:
		verdictItem = item("review.verdict", Unknown, "the last non-empty line is not a VERDICT line")
	case m[1] == "FAIL":
		verdictItem = item("review.verdict", Fail, "the review's verdict is FAIL")
	default:
		verdictItem = item("review.verdict", Pass, "VERDICT: %s", m[1])
		if m[1] == "PASS WITH NOTES" {
			notes, trunc := extractNotes(lines)
			verdictItem.Data = map[string]any{"notes": notes, "notes_truncated": trunc}
		}
	}

	start := len(lines) - bindWindow
	if start < 0 {
		start = 0
	}
	window := lines[start:]
	vals := map[string][]string{}
	malformed := ""
	for _, l := range window {
		if !strings.HasPrefix(l, "REVIEWED-") {
			continue
		}
		switch {
		case bindPRRe.MatchString(l):
			vals["pr"] = append(vals["pr"], bindPRRe.FindStringSubmatch(l)[1])
		case bindHead.MatchString(l):
			vals["head"] = append(vals["head"], bindHead.FindStringSubmatch(l)[1])
		case bindSHA.MatchString(l):
			vals["sha"] = append(vals["sha"], bindSHA.FindStringSubmatch(l)[1])
		default:
			malformed = l
		}
	}
	if side := sidecarNote(o.Review, m); side != "" {
		verdictItem.Detail += "; " + side
	}
	return []Item{verdictItem, bindItem(o, pr, scriptDigest, vals, malformed)}
}

// sidecarNote reports a sibling <receipt>.verdict file that disagrees with the
// receipt's last line. The sidecar is dispatch-tool output, not the reviewer's,
// so it never changes the status; it only appears in the detail.
func sidecarNote(review string, m []string) string {
	b, _, err := readRegular(review+".verdict", 64<<10)
	if err != nil {
		return ""
	}
	t := string(b)
	switch {
	case m == nil:
		return ""
	case strings.Contains(t, "No verdict line"):
		return "a sibling .verdict file says \"No verdict line\" (ignored: the receipt's last line is the verdict)"
	case strings.Contains(t, "VERDICT: FAIL") && m[1] != "FAIL":
		return "a sibling .verdict file says FAIL (ignored: the receipt's last line is the verdict)"
	}
	return ""
}

func bindItem(o Options, pr *prEvidence, scriptDigest string, vals map[string][]string, malformed string) Item {
	const id = "review.binds"
	if malformed != "" {
		return item(id, Fail, "a REVIEWED-* line is malformed (full lowercase digests only): %.80s", malformed)
	}
	st := Pass
	var parts []string
	// need compares the review's claim for one key with the artifact it must be.
	need := func(key, label, actual string, have bool) {
		got := vals[key]
		switch {
		case len(got) == 0:
			st = worse(st, Unknown)
			parts = append(parts, label+": the review does not say what it bound")
		case !allSame(got):
			st = worse(st, Fail)
			parts = append(parts, label+": the review names more than one value")
		case !have:
			st = worse(st, Unknown)
			parts = append(parts, label+": the current value could not be read")
		case got[0] != actual:
			st = worse(st, Fail)
			parts = append(parts, fmt.Sprintf("%s: the review bound %s, the current value is %s", label, got[0], actual))
		default:
			parts = append(parts, label+" matches")
		}
	}
	if o.PR != "" {
		have := pr != nil
		repoNum, head := "", ""
		if have {
			repoNum, head = pr.Repo+"#"+pr.Number, pr.Head
		}
		need("pr", "pull request", repoNum, have)
		need("head", "head", head, have)
	}
	if o.Script != "" {
		need("sha", "script sha256", scriptDigest, scriptDigest != "")
	}
	return item(id, st, "%s", strings.Join(parts, "; "))
}

func allSame(s []string) bool {
	for _, v := range s {
		if v != s[0] {
			return false
		}
	}
	return true
}

// extractNotes returns the NOTE: lines in the noteWindow lines before the
// first binding line (or before the verdict when there is none).
func extractNotes(lines []string) ([]string, bool) {
	end := len(lines) - 1 // exclude the verdict
	for i := len(lines) - 2; i >= 0 && i >= len(lines)-1-bindWindow; i-- {
		if strings.HasPrefix(lines[i], "REVIEWED-") {
			end = i
		}
	}
	from := end - noteWindow
	if from < 0 {
		from = 0
	}
	var notes []string
	trunc := false
	total := 0
	for _, l := range lines[from:end] {
		if !strings.HasPrefix(l, "NOTE:") {
			continue
		}
		if len(l) > maxNoteBytes {
			l, trunc = l[:maxNoteBytes], true
		}
		if len(notes) >= maxNotes || total+len(l) > maxNotesTotal {
			trunc = true
			break
		}
		notes = append(notes, l)
		total += len(l)
	}
	if from > 0 {
		trunc = true
	}
	return notes, trunc
}
