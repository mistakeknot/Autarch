package door

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ProductSource records what was actually read. A source's own confirmed or
// validated declaration is separate from read status and from measured success.
type ProductSource struct {
	Path, State, Content, Error string
	Modified                    time.Time
}

type ProductField struct {
	State, Value, Reason, Needs, Ref, Path string
	Evidence                               []struct{ Path, Scope string }
}

type ProductCard struct {
	ArtifactType string `yaml:"artifact_type"`
	Project      string
	Status       string
	Line         string
	Fields       map[string]ProductField
	Decisions    []string
}

type ProductJourney struct {
	ID      string `json:"cuj_id"`
	Status  string `json:"status"`
	Actor   string `json:"actor"`
	Trigger string `json:"trigger"`
	Success string `json:"success_condition"`
	Steps   []struct {
		Step string `json:"step"`
	} `json:"steps"`
	Source ProductSource `json:"-"`
}

type ProductWork struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	SpecID      string `json:"spec_id"`
	Description string `json:"description"`
}

type ProductBacklog struct {
	Source ProductSource
	Label  string // only set when a shared ancestor tracker was filtered
	Items  []ProductWork
}

type ProductBrief struct {
	Root          string
	ReadAt        time.Time
	Card          ProductCard
	CardSource    ProductSource
	Journeys      []ProductJourney
	JourneySource ProductSource
	Roadmap       ProductSource
	Decisions     []ProductSource
	Backlog       ProductBacklog
}

const productFileLimit = 256 << 10

// productPath contains both lexical paths and symlinks within the project.
// Decisions are references, not permission to read arbitrary machine files.
func productPath(root, rel string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, rel)
	if filepath.IsAbs(rel) {
		path = filepath.Clean(rel)
	}
	inside := func(base, path string) bool {
		r, e := filepath.Rel(base, path)
		return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
	}
	if !inside(root, path) {
		return "", fmt.Errorf("reference leaves project: %s", rel)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path, err
	}
	if !inside(realRoot, realPath) {
		return "", fmt.Errorf("symlink leaves project: %s", rel)
	}
	return path, nil
}

func readProductSource(root, rel string) ProductSource {
	s := ProductSource{Path: rel, State: "unread"}
	path, err := productPath(root, rel)
	if err != nil {
		if os.IsNotExist(err) {
			s.State = "missing"
		}
		s.Error = err.Error()
		return s
	}
	f, err := os.Open(path)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		s.Error = "source is not a readable regular file"
		return s
	}
	s.Modified = info.ModTime()
	b, err := io.ReadAll(io.LimitReader(f, productFileLimit+1))
	if err != nil {
		s.Error = err.Error()
		return s
	}
	if len(b) > productFileLimit {
		s.Error = "source exceeds 256 KiB; open the file to read it"
		return s
	}
	s.State, s.Content = "read", string(b)
	return s
}

// ReadProductBrief transcribes existing artifacts. It neither ratifies a card
// nor scores a project, and never writes back to any of its sources.
func ReadProductBrief(ctx context.Context, root string, backlogReader func(context.Context, string, string) ProductBacklog) ProductBrief {
	b := ProductBrief{Root: root, ReadAt: time.Now()}
	b.CardSource = readProductSource(root, "docs/why.md")
	if b.CardSource.State == "read" {
		parts := strings.SplitN(strings.ReplaceAll(b.CardSource.Content, "\r\n", "\n"), "\n---", 2)
		var err error
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "---\n") {
			err = fmt.Errorf("expected YAML frontmatter")
		} else {
			err = yaml.Unmarshal([]byte(strings.TrimPrefix(parts[0], "---\n")), &b.Card)
			if err == nil && b.Card.ArtifactType != "card" {
				err = fmt.Errorf("expected artifact_type: card")
			}
		}
		if err != nil {
			b.CardSource.State, b.CardSource.Error = "unread", err.Error()
			b.Card = ProductCard{}
		}
	}
	b.Roadmap = readProductSource(root, "docs/roadmap.md")
	b.JourneySource = ProductSource{Path: "docs/cujs", State: "read"}
	dir, err := productPath(root, b.JourneySource.Path)
	var entries []os.DirEntry
	if err == nil {
		entries, err = os.ReadDir(dir)
	}
	if err != nil {
		b.JourneySource.State, b.JourneySource.Error = "unread", err.Error()
		if os.IsNotExist(err) {
			b.JourneySource.State = "missing"
		}
	}
	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if entry.IsDir() || (ext != ".json" && ext != ".md") || strings.EqualFold(entry.Name(), "README.md") {
			continue
		}
		if len(b.Journeys) >= 128 {
			b.JourneySource.State, b.JourneySource.Error = "unread", "showing the first 128 journeys; open docs/cujs for the rest"
			break
		}
		s := readProductSource(root, filepath.Join("docs/cujs", entry.Name()))
		j := ProductJourney{ID: strings.TrimSuffix(entry.Name(), ext), Source: s}
		if ext == ".json" && s.State == "read" {
			if err := json.Unmarshal([]byte(s.Content), &j); err != nil {
				j = ProductJourney{ID: entry.Name(), Source: s}
				j.Source.State, j.Source.Error = "unread", err.Error()
			}
		}
		b.Journeys = append(b.Journeys, j)
	}
	for _, path := range b.Card.Decisions {
		b.Decisions = append(b.Decisions, readProductSource(root, path))
	}
	label := b.Card.Project
	if label == "" {
		label = strings.ToLower(filepath.Base(root))
	}
	if backlogReader == nil {
		backlogReader = ReadProductBacklog
	}
	b.Backlog = backlogReader(ctx, root, label)
	return b
}

// limitedProductOutput bounds memory while draining a subprocess's output.
type limitedProductOutput struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedProductOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if n > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

// ReadProductBacklog uses the nearest tracker. A shared tracker is explicitly
// scoped by project label; zero matches says nothing about unlabeled work.
func ReadProductBacklog(ctx context.Context, root, label string) ProductBacklog {
	b := ProductBacklog{Source: ProductSource{State: "missing", Error: "no .beads tracker found in the project or its ancestors"}}
	root, err := filepath.Abs(root)
	if err != nil {
		b.Source.State, b.Source.Error = "unread", err.Error()
		return b
	}
	dir := root
	for {
		info, err := os.Stat(filepath.Join(dir, ".beads"))
		if err == nil && info.IsDir() {
			break
		}
		if err != nil && !os.IsNotExist(err) {
			b.Source.State, b.Source.Error = "unread", err.Error()
			return b
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return b
		}
		dir = parent
	}
	b.Source.Path = dir
	args := []string{"--readonly", "--sandbox", "--directory", dir, "list", "--json", "--limit", "0", "--sort", "priority"}
	if dir != root {
		if label == "" || strings.ContainsAny(label, ",\n\r") {
			b.Source.State, b.Source.Error = "unread", "shared tracker requires a single project label"
			return b
		}
		b.Label = label
		args = append(args, "--label", label)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", args...)
	cmd.WaitDelay = 250 * time.Millisecond
	var out = limitedProductOutput{limit: 4 << 20}
	var stderr = limitedProductOutput{limit: 8 << 10}
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		b.Source.State, b.Source.Error = "unread", fmt.Sprintf("bd: %v: %s", err, strings.TrimSpace(stderr.String()))
		return b
	}
	if out.overflow {
		b.Source.State, b.Source.Error = "unread", "backlog exceeds 4 MiB; inspect it with bd"
		return b
	}
	if err := json.Unmarshal(out.Bytes(), &b.Items); err != nil {
		b.Source.State, b.Source.Error = "unread", "bd returned invalid JSON: "+err.Error()
		return b
	}
	b.Source.State, b.Source.Error = "read", ""
	return b
}
