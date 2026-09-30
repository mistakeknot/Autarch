package serve

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"

	gserver "github.com/mistakeknot/autarch/internal/gurgeh/server"
)

// gurgehMounts serves /gurgeh/{project}/ from one Gurgeh server per resolved root.
type gurgehMounts struct {
	resolver *Resolver
	newSrv   func(root string) *gserver.Server

	mu    sync.Mutex
	byDir map[string]http.Handler
}

func newGurgehMounts(r *Resolver, newSrv func(root string) *gserver.Server) *gurgehMounts {
	return &gurgehMounts{resolver: r, newSrv: newSrv, byDir: map[string]http.Handler{}}
}

func (g *gurgehMounts) handler(root, prefix string) http.Handler {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := root + "\x00" + prefix
	h, ok := g.byDir[key]
	if !ok {
		h = http.StripPrefix(prefix, g.newSrv(root).Handler())
		g.byDir[key] = h
	}
	return h
}

func (g *gurgehMounts) count() int {
	n := 0
	for _, p := range g.resolver.Projects() {
		if fi, err := os.Stat(filepath.Join(p.Root, ".gurgeh")); err == nil && fi.IsDir() {
			n++
		}
	}
	return n
}

func (g *gurgehMounts) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	root, err := g.resolver.Resolve(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if fi, err := os.Stat(filepath.Join(root, ".gurgeh")); err != nil || !fi.IsDir() {
		http.NotFound(w, r)
		return
	}
	g.handler(root, "/gurgeh/"+name).ServeHTTP(w, r)
}
