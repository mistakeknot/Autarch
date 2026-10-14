package escalcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// prEvidence is what the review binding is checked against.
type prEvidence struct {
	Repo   string // OWNER/REPO
	Number string
	Head   string
}

type rollupEntry struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

type prView struct {
	State             string        `json:"state"`
	IsDraft           bool          `json:"isDraft"`
	HeadRefOid        string        `json:"headRefOid"`
	BaseRefName       string        `json:"baseRefName"`
	StatusCheckRollup []rollupEntry `json:"statusCheckRollup"`
}

func readPR(ctx context.Context, env Env, repo, num string) (*prView, error) {
	res, err := env.Runner.Run(ctx, []string{"gh", "pr", "view", num, "--repo", "github.com/" + repo,
		"--json", "state,isDraft,headRefOid,baseRefName,statusCheckRollup"}, childEnv(env.Environ()))
	if err != nil {
		return nil, fmt.Errorf("gh did not run: %v", err)
	}
	if res.TimedOut {
		return nil, fmt.Errorf("gh timed out")
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("gh exit %d: %s", res.Code, firstLines(res.Stderr, 3))
	}
	if res.Truncated {
		return nil, fmt.Errorf("gh output was longer than the %d-byte cap", cmdOutputCap)
	}
	var v prView
	if err := json.Unmarshal([]byte(res.Stdout), &v); err != nil {
		return nil, fmt.Errorf("gh output is not the expected JSON: %v", err)
	}
	if !hex40.MatchString(v.HeadRefOid) {
		return nil, fmt.Errorf("gh returned no valid head commit")
	}
	return &v, nil
}

func checkPR(ctx context.Context, o Options, env Env) ([]Item, *prEvidence) {
	m := prURLRe.FindStringSubmatch(o.PR)
	if m == nil {
		return []Item{item("pr.url", Fail, "not a https://github.com/OWNER/REPO/pull/N URL")}, nil
	}
	repo, num := m[1]+"/"+m[2], m[3]
	items := []Item{item("pr.url", Pass, "github.com/%s pull %s", repo, num)}
	ids := []string{"pr.state", "pr.checks", "pr.head"}
	unknown := func(why string) ([]Item, *prEvidence) {
		for _, id := range ids {
			items = append(items, item(id, Unknown, "%s", why))
		}
		return items, nil
	}
	if o.Offline {
		return unknown("--offline: the pull request was not read")
	}
	first, err := readPR(ctx, env, repo, num)
	if err != nil {
		return unknown(err.Error())
	}
	// The evidence below comes from a second read taken after everything else
	// the PR group needs, so a draft flip, a closure or a new failing check on
	// an unchanged head is seen.
	final, err := readPR(ctx, env, repo, num)
	if err != nil {
		return unknown("second read: " + err.Error())
	}
	ev := &prEvidence{Repo: repo, Number: num, Head: final.HeadRefOid}

	want := "OPEN"
	if o.PostMerge {
		want = "MERGED"
	}
	switch {
	case final.State != want:
		items = append(items, item("pr.state", Fail, "state is %s, expected %s", final.State, want))
	case final.IsDraft:
		items = append(items, item("pr.state", Fail, "the pull request is a draft"))
	default:
		it := item("pr.state", Pass, "%s, base %s, observed at the final read", final.State, final.BaseRefName)
		items = append(items, it)
	}

	st, detail := evalChecks(final.StatusCheckRollup, o.RequireChecks)
	items = append(items, item("pr.checks", st, "%s", detail))

	if first.HeadRefOid != final.HeadRefOid {
		items = append(items, item("pr.head", Fail, "the head moved from %s to %s while checking (pr.head_moved)", first.HeadRefOid, final.HeadRefOid))
	} else {
		it := item("pr.head", Pass, "%s", final.HeadRefOid)
		it.Data = map[string]any{"repo": repo, "number": num, "head": final.HeadRefOid}
		items = append(items, it)
	}
	return items, ev
}

func worse(a, b Status) Status {
	rank := map[Status]int{Pass: 0, Unknown: 1, Fail: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func entryResult(e rollupEntry) (name string, st Status, why string) {
	switch e.Typename {
	case "CheckRun":
		name = e.Name
		switch e.Status {
		case "COMPLETED":
		case "QUEUED", "IN_PROGRESS", "PENDING", "WAITING", "REQUESTED":
			return name, Fail, "not finished (" + e.Status + ")"
		default:
			return name, Unknown, "unknown status " + e.Status
		}
		switch e.Conclusion {
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			return name, Pass, ""
		case "FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STALE", "STARTUP_FAILURE":
			return name, Fail, e.Conclusion
		}
		return name, Unknown, "unknown conclusion " + e.Conclusion
	case "StatusContext":
		name = e.Context
		switch e.State {
		case "SUCCESS":
			return name, Pass, ""
		case "PENDING", "EXPECTED", "FAILURE", "ERROR":
			return name, Fail, e.State
		}
		return name, Unknown, "unknown state " + e.State
	}
	return e.Name, Unknown, "unknown entry type " + e.Typename
}

// evalChecks: every reported check must be green and every required name must
// be present and green. Unknown shapes fail closed. The worst result for a
// repeated name wins.
func evalChecks(rollup []rollupEntry, required []string) (Status, string) {
	byName := map[string]Status{}
	var bad []string
	overall := Pass
	for _, e := range rollup {
		name, st, why := entryResult(e)
		if name == "" {
			st, why, name = Unknown, "an entry has no name", "(unnamed)"
		}
		if prev, ok := byName[name]; ok {
			st = worse(prev, st)
		}
		byName[name] = st
		if st != Pass {
			bad = append(bad, fmt.Sprintf("%s: %s", name, why))
		}
		overall = worse(overall, st)
	}
	var missing []string
	for _, r := range required {
		if _, ok := byName[r]; !ok {
			missing = append(missing, r)
		}
	}
	sort.Strings(missing)
	switch {
	case len(missing) > 0:
		return Fail, "required checks missing: " + strings.Join(missing, ", ")
	case len(rollup) == 0 && len(required) == 0:
		return Unknown, "the pull request reports no checks"
	case overall != Pass:
		return overall, "not green: " + strings.Join(bad, "; ")
	}
	return Pass, fmt.Sprintf("%d reported checks green, %d required present", len(byName), len(required))
}
