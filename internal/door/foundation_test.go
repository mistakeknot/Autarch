package door

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func foundationFixture(t *testing.T, root string) ProductBrief {
	t.Helper()
	return ReadProductBrief(context.Background(), root, func(context.Context, string, string) ProductBacklog {
		return ProductBacklog{Source: ProductSource{State: "read", Path: root}}
	})
}

func foundationArea(t *testing.T, b ProductBrief, key string) FoundationArea {
	t.Helper()
	for _, a := range b.Foundation {
		if a.Key == key {
			return a
		}
	}
	t.Fatalf("missing area %s", key)
	return FoundationArea{}
}

func TestFoundationReusesExistingSourcesWithoutInventingApproval(t *testing.T) {
	root := t.TempDir()
	productFile(t, root, "docs/canon/mission.md", "# Mission\nHelp editors keep context.")
	productFile(t, root, "docs/VISION.md", "# Vision\nA reading workspace that remembers.")
	productFile(t, root, "PHILOSOPHY.md", "# Philosophy\nKeep the original text accessible.")
	productFile(t, root, "docs/canon/personas.md", "# Personas\nEditors comparing long documents.")
	productFile(t, root, "docs/decisions/001-local.md", "# Local storage\nAccepted: keep reading history on device.")
	productFile(t, root, "docs/design/standards.md", "# Design standards\nKeyboard navigation.")
	productFile(t, root, "docs/roadmap.md", "# Roadmap\nNow: preserve reading context.")
	productFile(t, root, "docs/cujs/review.md", "# Review\nCompare two passages.")
	b := foundationFixture(t, root)
	if len(b.Foundation) != 9 {
		t.Fatalf("got %d areas", len(b.Foundation))
	}
	for _, a := range b.Foundation {
		if a.State() != "Sources found" {
			t.Fatalf("%s: %s", a.Key, a.State())
		}
	}
	if len(foundationArea(t, b, "decisions").Sources) != 1 {
		t.Fatal("ADR requires a product card")
	}
	brief := BuildOnboardingBrief(b)
	for _, want := range []string{"docs/canon/mission.md", "docs/decisions/001-local.md", "AskUserQuestion", "provisional", "mission", "persona", "backlog"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("brief lost %q", want)
		}
	}
	if strings.Contains(brief, "9/9") || strings.Contains(brief, "onboarding complete") {
		t.Fatal("file presence became completion")
	}
}

func TestFoundationEmptyFoldersAndFilesAreNotCoverage(t *testing.T) {
	root := t.TempDir()
	productFile(t, root, "MISSION.md", " \n")
	if err := os.MkdirAll(filepath.Join(root, "docs/decisions"), 0755); err != nil {
		t.Fatal(err)
	}
	b := foundationFixture(t, root)
	if got := foundationArea(t, b, "mission").State(); got != "Empty sources" {
		t.Fatal(got)
	}
	if got := foundationArea(t, b, "decisions").State(); got != "Not found" {
		t.Fatal(got)
	}
	if got := foundationArea(t, b, "backlog").State(); got != "Sources found" {
		t.Fatal("empty live backlog is still a valid source", got)
	}
}

func TestFoundationRejectsEscapingSourcesAndDisclosesLimits(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	productFile(t, out, "mission.md", "outside private text")
	if err := os.Symlink(filepath.Join(out, "mission.md"), filepath.Join(root, "MISSION.md")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		productFile(t, root, fmt.Sprintf("docs/adr/%03d.md", i), "# Decision\nLocal source.")
	}
	b := foundationFixture(t, root)
	if got := foundationArea(t, b, "mission").State(); got != "Needs attention" {
		t.Fatal(got)
	}
	adr := foundationArea(t, b, "decisions")
	if !adr.Partial || len(adr.Sources) > 32 {
		t.Fatalf("unbounded ADR discovery: %+v", adr)
	}
	if strings.Contains(BuildOnboardingBrief(b), "outside private text") {
		t.Fatal("escaped source copied")
	}
}

func TestOnboardingCarriesUnresolvedCardNeedsAndPreservesFiles(t *testing.T) {
	root := t.TempDir()
	productFile(t, root, "docs/why.md", productCardFixture)
	b := foundationFixture(t, root)
	brief := BuildOnboardingBrief(b)
	for _, want := range []string{root, "Editors reviewing long documents", "A measured reading trial", "declined", "docs/why.md", "Design systems / standards"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("missing %q: %s", want, brief)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "docs/why.md"))
	if err != nil || string(data) != productCardFixture {
		t.Fatal("onboarding rewrote the card")
	}
	if _, err := os.Stat(filepath.Join(root, "MISSION.md")); !os.IsNotExist(err) {
		t.Fatal("scan created boilerplate")
	}
}
