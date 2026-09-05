package door

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type productMsg struct {
	generation int
	brief      ProductBrief
}

// NewProductModel opens a project directly without scanning the estate or
// changing the last-visit preference. The same view is available on rows via i.
func NewProductModel(root string) Model {
	return Model{screen: screenProduct, productRoot: root, productLoading: true, productStandalone: true, productGeneration: 1}
}

func (m Model) loadProduct() tea.Cmd {
	root, generation := m.productRoot, m.productGeneration
	return func() tea.Msg {
		return productMsg{generation: generation, brief: ReadProductBrief(context.Background(), root, nil)}
	}
}

func (m Model) enterProduct(root string) (tea.Model, tea.Cmd) {
	m.productFrom, m.screen = m.screen, screenProduct
	m.productRoot, m.productSection, m.productOffset = root, 0, 0
	m.product, m.productLoading, m.status = ProductBrief{}, true, ""
	m.productGeneration++
	return m, m.loadProduct()
}

func (m Model) handleProductKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, m.quit()
	case "esc":
		if m.productStandalone {
			return m, m.quit()
		}
		m.screen, m.status = m.productFrom, ""
		return m, nil
	case "r":
		if !m.productLoading {
			m.productLoading, m.status = true, ""
			m.productGeneration++
			return m, m.loadProduct()
		}
	case "1", "2", "3", "4", "5":
		m.productSection = int(key[0] - '1')
		m.productOffset, m.status = 0, ""
	case "tab", "right":
		m.productSection = (m.productSection + 1) % 5
		m.productOffset, m.status = 0, ""
	case "shift+tab", "left":
		m.productSection = (m.productSection + 4) % 5
		m.productOffset, m.status = 0, ""
	case "j", "down":
		m.productOffset++
	case "k", "up":
		m.productOffset--
	case "pgdown", "ctrl+d":
		m.productOffset += m.productRoom()
	case "pgup", "ctrl+u":
		m.productOffset -= m.productRoom()
	case "g", "home":
		m.productOffset = 0
	case "G", "end":
		m.productOffset = len(m.productLines())
	case "o":
		return m, m.openProductSource()
	}
	m.productOffset = max(0, min(m.productOffset, len(m.productLines())-m.productRoom()))
	return m, nil
}

func productSourceLines(s ProductSource) []string {
	line := s.Path + " · " + s.State
	if !s.Modified.IsZero() {
		line += " · file modified " + s.Modified.Local().Format("2006-01-02 15:04")
	}
	lines := []string{line}
	if s.Error != "" {
		lines = append(lines, s.Error)
	}
	return lines
}

func productScope(b ProductBacklog) string {
	if b.Source.Path == "" {
		return "Beads · " + b.Source.State
	}
	s := "Beads · " + b.Source.Path
	if b.Label != "" {
		s += " · label " + b.Label + " (shared tracker; unlabeled work excluded)"
	} else {
		s += " · project tracker"
	}
	return s
}

func productWorkLine(w ProductWork) string {
	return fmt.Sprintf("%s · P%d · %s · %s", w.ID, w.Priority, w.Status, w.Title)
}

func (m Model) productLines() []string {
	if m.productLoading {
		return []string{"Reading product sources and live backlog…"}
	}
	p := m.product
	var lines []string
	add := func(s ...string) { lines = append(lines, s...) }
	source := func(s ProductSource) { add(productSourceLines(s)...) }
	field := func(key, title string) {
		f, ok := p.Card.Fields[key]
		if !ok {
			add(title+" · not declared", "")
			return
		}
		add(title + " · " + f.State)
		for _, value := range []string{f.Value, f.Reason} {
			if value != "" {
				add(value)
			}
		}
		if f.Needs != "" {
			add("Needs: " + f.Needs)
		}
		if f.Path != "" || f.Ref != "" {
			add("Reference: " + f.Ref + " · " + f.Path)
		}
		add("")
	}
	switch m.productSection {
	case 0:
		if p.CardSource.State != "read" {
			source(p.CardSource)
			add("")
		}
		add("CURRENT WORK")
		if p.Backlog.Source.State != "read" {
			add("Backlog " + p.Backlog.Source.State + ": " + p.Backlog.Source.Error)
		} else {
			active := 0
			for _, w := range p.Backlog.Items {
				if w.Status == "in_progress" {
					add(productWorkLine(w))
					active++
				}
			}
			if active == 0 {
				add("No in-progress items in this scope.")
			}
			add(fmt.Sprintf("%d non-closed items · 3 opens backlog and explicit spec links", len(p.Backlog.Items)))
		}
		add(productScope(p.Backlog), "", "PRODUCT INTENT · source declarations, not measured outcomes")
		field("persona", "For whom")
		field("cuj", "Primary journey")
		primary := p.Card.Fields["cuj"]
		found := false
		for _, j := range p.Journeys {
			if primary.Path == j.Source.Path || (primary.Ref != "" && primary.Ref == j.ID) {
				found = true
				if j.Source.State != "read" {
					source(j.Source)
				} else if j.Success != "" {
					add("Journey success condition: "+j.Success, "")
				}
			}
		}
		if !found && (primary.Path != "" || primary.Ref != "") {
			add("Primary journey reference not resolved in docs/cujs.", "")
		}
		field("success", "Project success measure")
		field("pain", "Problem")
		field("guardrail", "Guardrail")
		source(p.CardSource)
		add("Card state: "+p.Card.Status, "", "SOURCE COVERAGE")
		source(p.Roadmap)
		source(p.JourneySource)
		add(fmt.Sprintf("%d journeys · %d decision references", len(p.Journeys), len(p.Decisions)))
	case 1:
		add("ROADMAP · source document", "Dates below are from the document; file modification is not a freshness check.", "")
		source(p.Roadmap)
		if p.Roadmap.State == "read" {
			add("", p.Roadmap.Content)
		}
	case 2:
		add("BACKLOG · live read at "+p.ReadAt.Local().Format("15:04:05"), productScope(p.Backlog), "")
		if p.Backlog.Source.State != "read" {
			add(p.Backlog.Source.State + ": " + p.Backlog.Source.Error)
			break
		}
		add(fmt.Sprintf("%d non-closed items, ordered by priority. Spec references are explicit links.", len(p.Backlog.Items)), "")
		if len(p.Backlog.Items) == 0 {
			add("No matching items in this scope.")
		}
		for _, w := range p.Backlog.Items {
			add(productWorkLine(w))
			if w.SpecID == "" {
				add("  spec: not linked")
			} else {
				add("  spec: " + w.SpecID)
			}
			if w.Description != "" {
				add(w.Description)
			}
			add("")
		}
	case 3:
		add("JOURNEYS · validation status is declared by each source", "")
		source(p.JourneySource)
		for _, j := range p.Journeys {
			add("", j.ID+" · "+j.Status)
			source(j.Source)
			if j.Source.State != "read" {
				continue
			}
			if strings.EqualFold(filepath.Ext(j.Source.Path), ".md") {
				add(j.Source.Content)
				continue
			}
			add("Actor: "+j.Actor, "Trigger: "+j.Trigger, "Success condition: "+j.Success)
			for i, step := range j.Steps {
				add(fmt.Sprintf("%d. %s", i+1, step.Step))
			}
		}
	case 4:
		add("DECISIONS · references declared in docs/why.md", "")
		if p.CardSource.State != "read" {
			source(p.CardSource)
		} else if len(p.Decisions) == 0 {
			add("No decision references declared in the card.")
		}
		for _, s := range p.Decisions {
			source(s)
			if s.State == "read" {
				add("", s.Content)
			}
			add("")
		}
	}
	var wrapped []string
	for _, line := range lines {
		if m.density == DensityCompact && strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(ansi.Wrap(cleanEvidence(line), m.dashboardContentWidth(), ""), "\n")
		for _, part := range parts {
			if strings.HasPrefix(line, "CURRENT WORK") || strings.HasPrefix(line, "PRODUCT INTENT") || strings.HasPrefix(line, "SOURCE COVERAGE") || strings.Contains(line, " · confirmed") || strings.Contains(line, " · declined") {
				part = styleTitle.Render(part)
			}
			wrapped = append(wrapped, part)
		}
	}
	return wrapped
}

func (m Model) productRoom() int { return m.dashboardRoom() }

func (m Model) productView() string {
	room := m.productRoom()
	all := m.productLines()
	start := max(0, min(m.productOffset, len(all)-room))
	name := m.product.Card.Project
	if name == "" {
		name = filepath.Base(m.productRoot)
	}
	m.status = fmt.Sprintf("%d–%d / %d · %s", start+1, min(len(all), start+room), len(all), m.status)
	return m.dashboardFrame("Project · "+oneLine(name), all[start:min(len(all), start+room)], "1–5/tab sections · ↑↓ scroll · o source · r refresh · d View · Esc back · q Quit")
}

func (m Model) openProductSource() tea.Cmd {
	rel := "docs/why.md"
	switch m.productSection {
	case 1:
		rel = "docs/roadmap.md"
	case 2:
		return func() tea.Msg {
			return statusMsg("Backlog is read from Beads; use bd in the displayed tracker with the displayed label.")
		}
	case 3:
		rel = "docs/cujs"
	case 4:
		rel = "docs"
	}
	root := m.productRoot
	return func() tea.Msg {
		path, err := productPath(root, rel)
		if err != nil {
			return statusMsg("Cannot open source: " + err.Error())
		}
		cmd := exec.Command("zed", path)
		if err := cmd.Start(); err != nil {
			return statusMsg("Zed failed: " + err.Error())
		}
		go func() { _ = cmd.Wait() }()
		return statusMsg("Opened " + rel + " in Zed")
	}
}
