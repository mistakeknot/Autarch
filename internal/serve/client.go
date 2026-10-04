package serve

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrNotRunning means nothing answered at serve's address (refused, timed out or unreachable).
var ErrNotRunning = errors.New("autarch serve is not running")

// FetchProjects asks the running serve for its project list (GET /api/projects with the bearer
// token). It never creates the token. A serve that does not answer within timeout is reported
// as ErrNotRunning with its address, so a caller can stop before it files anything.
func FetchProjects(baseURL, tokenPath string, timeout time.Duration) ([]ProjectInfo, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("bad serve URL %q", baseURL)
	}
	if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("serve URL %q is not loopback", baseURL)
	}
	tok, err := readToken(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("serve token: %w", err)
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/projects", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	// A redirect would leave the loopback-checked destination, so none is followed.
	hc := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := hc.Do(req)
	if err != nil {
		var ne net.Error
		var oe *net.OpError
		if errors.As(err, &oe) || (errors.As(err, &ne) && ne.Timeout()) {
			return nil, fmt.Errorf("%w (%s): %v", ErrNotRunning, u.Host, err)
		}
		return nil, fmt.Errorf("ask serve for projects: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("serve /api/projects: %s", resp.Status)
	}
	var out []ProjectInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode serve projects: %w", err)
	}
	return out, nil
}

// CheckRoot accepts root only when it is exactly a root serve resolves. A path that differs only
// by case (or by trailing slash) from a real root is refused with that root named.
func CheckRoot(projects []ProjectInfo, root string) error {
	for _, p := range projects {
		if p.Root == root {
			return nil
		}
	}
	clean := strings.TrimRight(root, "/")
	for _, p := range projects {
		if strings.EqualFold(strings.TrimRight(p.Root, "/"), clean) {
			return fmt.Errorf("project_root %q is not a project root serve resolves; did you mean %q?", root, p.Root)
		}
	}
	return fmt.Errorf("project_root %q is not a project root serve resolves (%d known); this checkout is not listed by serve: set project_root to the serve-listed root of the project, or list this worktree in serve's project dirs", root, len(projects))
}

// readToken reads serve's token without ever creating it (LoadOrCreateToken would on a race).
func readToken(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("token file %s has mode %04o; refusing (want 0600)", path, fi.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	tok := strings.TrimSpace(string(b))
	if _, err := hex.DecodeString(tok); err != nil || len(tok) != 64 {
		return "", fmt.Errorf("token file %s is not a 64 hex-character token", path)
	}
	return tok, nil
}
