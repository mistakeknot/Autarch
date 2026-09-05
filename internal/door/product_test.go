package door

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const productCardFixture = `---
artifact_type: card
project: reader
status: confirmed
line: A reader for people reviewing long documents
fields:
  persona: {state: confirmed, value: Editors reviewing long documents}
  pain: {state: confirmed, value: Losing their place while comparing sources}
  cuj: {state: confirmed, ref: reader-01, path: docs/cujs/reader-01.json}
  success: {state: declined, needs: A measured reading trial}
  guardrail: {state: confirmed, value: Keep annotations with their sources}
decisions: [docs/decisions/layout.md]
---
Product intent stays in this file.
`

func productFile(t *testing.T, root, path, body string) {
	t.Helper()
	name := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestProductBriefReadsExistingSourcesWithoutInventingSuccess(t *testing.T) {
	root := t.TempDir()
	productFile(t, root, "docs/why.md", productCardFixture)
	productFile(t, root, "docs/cujs/reader-01.json", `{"cuj_id":"reader-01","status":"validated","actor":"Editor","trigger":"Return to a document","success_condition":"Find the next passage in a minute","steps":[{"step":"Open the reader"}]}`)
	productFile(t, root, "docs/roadmap.md", "# Roadmap\n\n> Generated on 2026-02-25\n\n## Next\n- Source comparison\n")
	productFile(t, root, "docs/decisions/layout.md", "# Layout\n\nKeep the document alongside its references.\n")
	brief := ReadProductBrief(context.Background(), root, func(_ context.Context, r, label string) ProductBacklog {
		if r != root || label != "reader" {
			t.Fatalf("wrong project scope: %q %q", r, label)
		}
		return ProductBacklog{Source: ProductSource{State: "read", Path: root}, Items: []ProductWork{{ID: "reader-1", Title: "Implement comparison", Status: "in_progress", SpecID: "reader-01"}}}
	})
	if brief.Card.Fields["persona"].Value != "Editors reviewing long documents" || brief.Card.Fields["success"].State != "declined" {
		t.Fatalf("lost product intent: %+v", brief.Card)
	}
	if len(brief.Journeys) != 1 || brief.Journeys[0].Success != "Find the next passage in a minute" || brief.Journeys[0].Status != "validated" {
		t.Fatalf("lost journey: %+v", brief.Journeys)
	}
	if !strings.Contains(brief.Roadmap.Content, "2026-02-25") || len(brief.Decisions) != 1 || brief.Backlog.Items[0].SpecID != "reader-01" {
		t.Fatal("sources or explicit links lost")
	}
}

func TestProductBriefDistinguishesMissingUnreadableAndEscapingSources(t *testing.T) {
	root := t.TempDir()
	productFile(t, root, "docs/why.md", strings.Replace(productCardFixture, "docs/decisions/layout.md", "../../outside.md", 1))
	productFile(t, root, "docs/cujs/broken.json", `{broken`)
	brief := ReadProductBrief(context.Background(), root, func(context.Context, string, string) ProductBacklog {
		return ProductBacklog{Source: ProductSource{State: "missing"}}
	})
	if brief.Roadmap.State != "missing" {
		t.Fatalf("missing roadmap: %+v", brief.Roadmap)
	}
	if len(brief.Journeys) != 1 || brief.Journeys[0].Source.State != "unread" {
		t.Fatal("malformed CUJ disappeared or became missing")
	}
	if len(brief.Decisions) != 1 || brief.Decisions[0].State != "unread" {
		t.Fatal("escaping reference was followed")
	}
	productFile(t, root, "docs/why.md", "---\nfields: [\n---\n")
	brief = ReadProductBrief(context.Background(), root, nil)
	if brief.CardSource.State != "unread" {
		t.Fatal("malformed card became an empty valid card")
	}
}

func TestProductBacklogUsesReadOnlyLiveCLIAndExplicitSharedScope(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "apps", "reader")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, ".beads"), 0700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	args := filepath.Join(bin, "args")
	t.Setenv("PRODUCT_ARGS", args)
	writeExec(t, filepath.Join(bin, "bd"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$PRODUCT_ARGS\"\nprintf '%s' '[{\"id\":\"reader-1\",\"title\":\"Keep context\",\"status\":\"in_progress\",\"priority\":1,\"spec_id\":\"reader-01\"}]'\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	b := ReadProductBacklog(context.Background(), root, "reader")
	got, _ := os.ReadFile(args)
	for _, flag := range []string{"--readonly\n", "--sandbox\n", "--directory\n" + base + "\n", "--label\nreader\n", "--limit\n0\n"} {
		if !strings.Contains(string(got), flag) {
			t.Fatalf("missing safe scope %q: %s", flag, got)
		}
	}
	if b.Source.State != "read" || b.Label != "reader" || len(b.Items) != 1 {
		t.Fatalf("wrong live backlog: %+v", b)
	}
	writeExec(t, filepath.Join(bin, "bd"), "#!/bin/sh\necho 'database unavailable' >&2\nexit 1\n")
	b = ReadProductBacklog(context.Background(), root, "reader")
	if b.Source.State != "unread" || !strings.Contains(b.Source.Error, "database unavailable") {
		t.Fatal("backlog failure became no work")
	}
}

func TestProductSourceRejectsNamedPipeWithoutBlocking(t *testing.T) {
	if _, err := exec.LookPath("mkfifo"); err != nil {
		t.Skip("mkfifo not available")
	}
	root := t.TempDir()
	if err := exec.Command("mkfifo", filepath.Join(root, "pipe")).Run(); err != nil {
		t.Fatal(err)
	}
	done := make(chan ProductSource, 1)
	go func() { done <- readProductSource(root, "pipe") }()
	select {
	case s := <-done:
		if s.State != "unread" || !strings.Contains(s.Error, "regular file") {
			t.Fatal(s)
		}
	case <-time.After(time.Second):
		t.Fatal("reader blocked opening a named pipe")
	}
}
