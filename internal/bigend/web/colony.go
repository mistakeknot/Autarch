package web

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
)

// Colony scene geometry, in SVG user units.
const (
	colonyCols     = 3
	colonyCellW    = 360
	colonyPad      = 20
	colonyItemSize = 14
	colonyItemGap  = 4
	colonyPerRow   = 18 // belt items per row before wrapping
	colonyHeadH    = 150
	colonyBeltH    = 26
)

// Tokyo Night palette used by the scene.
const (
	colonyGreen  = "#9ece6a"
	colonyYellow = "#e0af68"
	colonyRed    = "#f7768e"
	colonyGrey   = "#565f89"
	colonyBelt   = "#3b4261"
)

// neglectedAfter is the age of the oldest open item past which a base is
// coloured as neglected (yellow). A base with a jam item is red.
const neglectedAfter = 48 * time.Hour

type colonyScene struct {
	Width, Height int
	Bases         []colonyBase
	UpdatedAt     time.Time
}

type colonyBase struct {
	Project, Name  string
	X, Y           int // top-left of this project's cell
	CX, CY, R      int // base circle
	Fill           string
	Open           int
	Oldest         string
	Units          []colonyUnit
	BeltX1, BeltX2 int
	BeltY          int
	BeltStroke     string
	Jam            bool
	Items          []colonyItem
}

type colonyUnit struct {
	Agent string
	X, Y  int
	Idle  bool
}

type colonyItem struct {
	ID, Status, Age string
	X, Y            int
	Jam             bool
}

// compactAge renders a duration as whole hours, or whole minutes under an hour.
func compactAge(d time.Duration) string {
	if d >= time.Hour {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%dm", int(d/time.Minute))
}

// buildColonyScene lays out one base per project, a unit per agent of that
// project and a belt item per open bead. Agents whose project is not in the
// state have no base and are not drawn.
func buildColonyScene(state aggregator.State) colonyScene {
	itemsByProject := map[string][]aggregator.Item{}
	for _, it := range state.Items {
		if it.Status == "closed" {
			continue
		}
		itemsByProject[it.Project] = append(itemsByProject[it.Project], it)
	}
	agentsByProject := map[string][]aggregator.Agent{}
	for _, a := range state.Agents {
		agentsByProject[a.ProjectPath] = append(agentsByProject[a.ProjectPath], a)
	}

	scene := colonyScene{UpdatedAt: state.UpdatedAt}
	rowTop, rowH := 0, 0
	for i, p := range state.Projects {
		col := i % colonyCols
		if col == 0 {
			rowTop += rowH
			rowH = 0
		}
		items := itemsByProject[p.Path]
		sort.SliceStable(items, func(a, b int) bool { return items[a].Age > items[b].Age }) // oldest at the front

		b := colonyBase{
			Project: p.Path,
			Name:    p.Name,
			X:       col * colonyCellW,
			Y:       rowTop,
			Open:    len(items),
			Fill:    colonyGrey,
			Oldest:  "-",
		}
		b.R = 18 + int(math.Min(28, 4*math.Sqrt(float64(len(items)))))
		b.CX, b.CY = b.X+colonyPad+48, b.Y+colonyPad+48
		b.BeltX1, b.BeltX2 = b.X+colonyPad, b.X+colonyCellW-colonyPad
		b.BeltY = b.Y + colonyHeadH
		b.BeltStroke = colonyBelt

		var oldest time.Duration
		for _, it := range items {
			if it.Age > oldest {
				oldest = it.Age
			}
			if it.Jam {
				b.Jam = true
			}
		}
		switch {
		case len(items) == 0:
		case b.Jam:
			b.Fill = colonyRed
		case oldest >= neglectedAfter:
			b.Fill = colonyYellow
		default:
			b.Fill = colonyGreen
		}
		if len(items) > 0 {
			b.Oldest = compactAge(oldest)
		}
		if b.Jam {
			b.BeltStroke = colonyRed
		}

		for k, a := range agentsByProject[p.Path] {
			b.Units = append(b.Units, colonyUnit{
				Agent: a.Name,
				X:     b.CX + b.R + 24 + (k%8)*20,
				Y:     b.CY - 12 + (k/8)*20,
				Idle:  a.SessionName == "",
			})
		}

		for k, it := range items {
			b.Items = append(b.Items, colonyItem{
				ID:     it.ID,
				Status: it.Status,
				Age:    compactAge(it.Age),
				Jam:    it.Jam,
				X:      b.BeltX1 + (k%colonyPerRow)*(colonyItemSize+colonyItemGap),
				Y:      b.BeltY + 8 + (k/colonyPerRow)*(colonyItemSize+colonyItemGap),
			})
		}

		h := colonyHeadH + colonyBeltH + (len(items)+colonyPerRow-1)/colonyPerRow*(colonyItemSize+colonyItemGap)
		if h > rowH {
			rowH = h
		}
		scene.Bases = append(scene.Bases, b)
	}
	scene.Width = colonyCols * colonyCellW
	scene.Height = rowTop + rowH
	return scene
}

func (s *Server) colonyData() map[string]any {
	return map[string]any{"Scene": buildColonyScene(s.agg.GetState())}
}

func (s *Server) handleColony(w http.ResponseWriter, r *http.Request) {
	s.render(w, "colony.html", s.colonyData())
}

// handleColonyScene serves the scene fragment that the page polls with htmx.
func (s *Server) handleColonyScene(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	tmpl, ok := s.templates["colony.html"]
	if !ok {
		http.Error(w, "Template not found: colony.html", http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "colony_scene", s.colonyData()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
