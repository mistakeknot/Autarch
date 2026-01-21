package tui

type Router struct {
	active  string
	screens map[string]Screen
}

func NewRouter(screens map[string]Screen, active string) Router {
	return Router{screens: screens, active: active}
}

func (r *Router) Switch(name string) {
	if _, ok := r.screens[name]; ok {
		r.active = name
	}
}

func (r *Router) ActiveName() string {
	return r.active
}
