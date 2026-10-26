package web

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/internal/bigend/discovery"
)

// recordingAgg records the project path the /projects/ route resolves.
type recordingAgg struct {
	fakeAgg
	got string
}

func (r *recordingAgg) GetProject(path string) *discovery.Project {
	r.got = path
	return nil
}

// svgNode is a minimal DOM over the rendered scene so the test reads
// attributes instead of matching strings.
type svgNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []svgNode  `xml:",any"`
	Text     string     `xml:",chardata"`
}

func (n svgNode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (n svgNode) find(pred func(svgNode) bool) []svgNode {
	var out []svgNode
	if pred(n) {
		out = append(out, n)
	}
	for _, c := range n.Children {
		out = append(out, c.find(pred)...)
	}
	return out
}

func hasClass(n svgNode, class string) bool {
	for _, c := range strings.Fields(n.attr("class")) {
		if c == class {
			return true
		}
	}
	return false
}

func parseSVG(t *testing.T, body string) svgNode {
	t.Helper()
	start := strings.Index(body, "<svg")
	end := strings.LastIndex(body, "</svg>")
	if start < 0 || end < 0 {
		t.Fatalf("no <svg> in response:\n%s", body)
	}
	var root svgNode
	if err := xml.NewDecoder(strings.NewReader(body[start : end+len("</svg>")])).Decode(&root); err != nil && err != io.EOF {
		t.Fatalf("scene is not well-formed XML: %v", err)
	}
	return root
}

// colonyFixture has three projects: alpha has 60 open beads (more than bd's
// default page of 50) and one jam, beta has two fresh beads, gamma has none.
// Two agents are active in alpha and beta, one is idle in alpha, and one
// belongs to a project that is not on the map.
func colonyFixture() aggregator.State {
	var items []aggregator.Item
	for i := 0; i < 60; i++ {
		it := aggregator.Item{ID: fmt.Sprintf("a-%02d", i), Project: "/p/alpha", Status: "open", Age: time.Duration(i+1) * time.Hour}
		if i == 17 {
			it.Age = 9 * 24 * time.Hour
			it.Jam = true
		}
		items = append(items, it)
	}
	items = append(items,
		aggregator.Item{ID: "b-1", Project: "/p/beta", Status: "open", Age: 2 * time.Hour},
		aggregator.Item{ID: "b-2", Project: "/p/beta", Status: "in_progress", Age: 5 * time.Hour},
	)
	return aggregator.State{
		Projects: []discovery.Project{
			{Path: "/p/alpha", Name: "alpha"},
			{Path: "/p/beta", Name: "beta"},
			{Path: "/p/gamma", Name: "gamma"},
		},
		Agents: []aggregator.Agent{
			{Name: "ann", ProjectPath: "/p/alpha", SessionName: "ann-sess"},
			{Name: "bob", ProjectPath: "/p/beta", SessionName: "bob-sess"},
			{Name: "cy", ProjectPath: "/p/alpha"}, // no session: idle
			{Name: "dee", ProjectPath: "/p/elsewhere", SessionName: "dee-sess"},
		},
		Items: items,
	}
}

func getColony(t *testing.T, path string, state aggregator.State) string {
	t.Helper()
	srv := NewServer(config.ServerConfig{}, &fakeAgg{state: state})
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestColonySceneRendersFixture(t *testing.T) {
	body := getColony(t, "/colony/scene", colonyFixture())
	root := parseSVG(t, body)

	bases := root.find(func(n svgNode) bool { return hasClass(n, "base") })
	if len(bases) != 3 {
		t.Fatalf("want one base per project (3), got %d", len(bases))
	}
	byProject := map[string]svgNode{}
	for _, b := range bases {
		byProject[b.attr("data-project")] = b
	}
	for _, p := range []string{"/p/alpha", "/p/beta", "/p/gamma"} {
		if _, ok := byProject[p]; !ok {
			t.Errorf("no base for %s", p)
		}
	}

	// Click a base: the link goes to the existing project detail page.
	for p, b := range byProject {
		links := b.find(func(n svgNode) bool { return n.XMLName.Local == "a" })
		if len(links) == 0 || links[0].attr("href") != "/projects"+p {
			t.Errorf("base %s: want link /projects%s, got %+v", p, p, links)
		}
	}

	// Units: one per agent with a session, idle ones dim, unmapped project dropped.
	units := root.find(func(n svgNode) bool { return hasClass(n, "unit") })
	if len(units) != 3 {
		t.Fatalf("want 3 units (ann, bob, idle cy; dee has no base), got %d", len(units))
	}
	idle := map[string]bool{}
	for _, u := range units {
		idle[u.attr("data-agent")] = hasClass(u, "idle")
	}
	if idle["ann"] || idle["bob"] || !idle["cy"] {
		t.Errorf("want ann,bob active and cy idle, got idle=%v", idle)
	}
	if _, ok := idle["dee"]; ok {
		t.Errorf("agent in a project with no base must not be drawn")
	}

	// Belt items: one per open bead, each carrying its age.
	items := root.find(func(n svgNode) bool { return hasClass(n, "item") })
	if len(items) != 62 {
		t.Fatalf("want 62 belt items, got %d", len(items))
	}
	ages := map[string]string{}
	for _, it := range items {
		ages[it.attr("data-id")] = it.attr("data-age")
	}
	if ages["a-17"] != "216h" || ages["b-1"] != "2h" || ages["a-00"] != "1h" {
		t.Errorf("item ages wrong: a-17=%q b-1=%q a-00=%q", ages["a-17"], ages["b-1"], ages["a-00"])
	}
	jamItems := root.find(func(n svgNode) bool { return hasClass(n, "item") && hasClass(n, "jam") })
	if len(jamItems) != 1 || jamItems[0].attr("data-id") != "a-17" {
		t.Errorf("want exactly a-17 flagged as jam item, got %d", len(jamItems))
	}

	// Belts: red exactly where the fixture has a jam.
	belts := root.find(func(n svgNode) bool { return hasClass(n, "belt") })
	if len(belts) != 3 {
		t.Fatalf("want a belt per base (3), got %d", len(belts))
	}
	for _, b := range belts {
		p := b.attr("data-project")
		red := hasClass(b, "jam")
		if red != (p == "/p/alpha") {
			t.Errorf("belt %s jam=%v, want %v", p, red, p == "/p/alpha")
		}
		if red != (b.attr("data-jam") == "true") {
			t.Errorf("belt %s: class and data-jam disagree", p)
		}
	}
	if strings.Count(body, "#f7768e") == 0 {
		t.Errorf("jam belt must be drawn in red (#f7768e)")
	}
}

func TestColonySceneNoJamNoRedBelt(t *testing.T) {
	st := colonyFixture()
	for i := range st.Items {
		st.Items[i].Jam = false
	}
	root := parseSVG(t, getColony(t, "/colony/scene", st))
	if jam := root.find(func(n svgNode) bool { return hasClass(n, "jam") }); len(jam) != 0 {
		t.Errorf("no jam in fixture, but %d jam elements rendered", len(jam))
	}
}

func TestColonyPageEmbedsPolledScene(t *testing.T) {
	body := getColony(t, "/colony", colonyFixture())
	if !strings.Contains(body, `hx-get="/colony/scene"`) || !strings.Contains(body, "hx-trigger=") {
		t.Errorf("page must poll the scene fragment with htmx")
	}
	if strings.Count(body, `class="base`) != 3 {
		t.Errorf("page should render the scene inline for first paint")
	}
	if !strings.Contains(body, "<html") {
		t.Errorf("/colony must be a full page")
	}
	frag := getColony(t, "/colony/scene", colonyFixture())
	if strings.Contains(frag, "<html") {
		t.Errorf("/colony/scene must be a fragment, not a full page")
	}
}

func TestColonyEmptyState(t *testing.T) {
	body := getColony(t, "/colony/scene", aggregator.State{})
	if !strings.Contains(body, "No projects") {
		t.Errorf("empty state should say so")
	}
}

// Every base link must resolve, through the real router, to the project it was
// drawn for, whatever characters the path holds.
func TestColonyBaseLinkRoundTrip(t *testing.T) {
	paths := []string{
		"/p/alpha",
		"/p/a#b?c%25",
		"/p/with space/and ünï",
		"/p/100%/x&y=1;z+w",
	}
	var projects []discovery.Project
	for _, p := range paths {
		projects = append(projects, discovery.Project{Path: p, Name: "n"})
	}
	rec := &recordingAgg{fakeAgg: fakeAgg{state: aggregator.State{Projects: projects}}}
	srv := NewServer(config.ServerConfig{}, rec)
	scene := httptest.NewRecorder()
	srv.routes().ServeHTTP(scene, httptest.NewRequest(http.MethodGet, "/colony/scene", nil))
	root := parseSVG(t, scene.Body.String())

	for _, p := range paths {
		var href string
		for _, b := range root.find(func(n svgNode) bool { return hasClass(n, "base") && n.attr("data-project") == p }) {
			if l := b.find(func(n svgNode) bool { return n.XMLName.Local == "a" }); len(l) > 0 {
				href = l[0].attr("href")
			}
		}
		if href == "" {
			t.Fatalf("no link for %q", p)
		}
		u, err := url.Parse(href)
		if err != nil || u.Fragment != "" || u.RawQuery != "" {
			t.Fatalf("%q: href %q has a fragment or query (err=%v)", p, href, err)
		}
		rec.got = ""
		w := httptest.NewRecorder()
		srv.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, href, nil))
		if w.Code == http.StatusMovedPermanently || w.Code == http.StatusFound {
			t.Fatalf("%q: href %q redirects to %q", p, href, w.Header().Get("Location"))
		}
		if rec.got != p {
			t.Errorf("href %q resolved to project %q, want %q", href, rec.got, p)
		}
	}
}

// A project with many agents must keep its units above its belt and inside the
// SVG, and a belt below units must still have its items inside the SVG.
func TestColonyCrowdedProjectLayout(t *testing.T) {
	st := aggregator.State{Projects: []discovery.Project{
		{Path: "/p/empty", Name: "empty"},
		{Path: "/p/busy", Name: "busy"},
	}}
	for i := 0; i < 57; i++ {
		st.Agents = append(st.Agents, aggregator.Agent{Name: fmt.Sprintf("u%02d", i), ProjectPath: "/p/empty", SessionName: "s"})
	}
	st.Agents = append(st.Agents, aggregator.Agent{Name: "solo", ProjectPath: "/p/busy", SessionName: "s"})
	for i := 0; i < 30; i++ {
		st.Items = append(st.Items, aggregator.Item{ID: fmt.Sprintf("b-%d", i), Project: "/p/empty", Status: "open", Age: time.Hour})
	}
	root := parseSVG(t, getColony(t, "/colony/scene", st))
	vb := strings.Fields(root.attr("viewBox"))
	height, err := strconv.Atoi(vb[3])
	if err != nil {
		t.Fatalf("bad viewBox %v", vb)
	}
	num := func(n svgNode, a string) int {
		v, err := strconv.Atoi(n.attr(a))
		if err != nil {
			t.Fatalf("%s %s=%q: %v", n.attr("data-id"), a, n.attr(a), err)
		}
		return v
	}

	beltY := map[string]int{}
	for _, b := range root.find(func(n svgNode) bool { return hasClass(n, "belt") }) {
		line := b.find(func(n svgNode) bool { return n.XMLName.Local == "line" })[0]
		beltY[b.attr("data-project")] = num(line, "y1")
		for _, it := range b.find(func(n svgNode) bool { return hasClass(n, "item") }) {
			if bottom := num(it, "y") + num(it, "height"); bottom > height {
				t.Errorf("belt item %s bottom %d past SVG height %d", it.attr("data-id"), bottom, height)
			}
		}
	}
	units := root.find(func(n svgNode) bool { return hasClass(n, "unit") })
	if len(units) != 58 {
		t.Fatalf("want 58 units, got %d", len(units))
	}
	for _, u := range units {
		r := u.find(func(n svgNode) bool { return n.XMLName.Local == "rect" })[0]
		bottom := num(r, "y") + num(r, "height")
		project := "/p/empty"
		if u.attr("data-agent") == "solo" {
			project = "/p/busy"
		}
		if bottom > height {
			t.Errorf("unit %s bottom %d past SVG height %d", u.attr("data-agent"), bottom, height)
		}
		if bottom >= beltY[project] {
			t.Errorf("unit %s bottom %d overlaps belt at y=%d", u.attr("data-agent"), bottom, beltY[project])
		}
	}
}
