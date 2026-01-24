// Package shell provides the unified shell framework for composing tool panes.
package shell

import (
	"path/filepath"

	"github.com/mistakeknot/vauxpraudemonium/pkg/toolpane"
)

// Context holds shared state across the unified shell
type Context struct {
	// Currently selected project
	ProjectPath string
	ProjectName string

	// Window dimensions
	Width  int
	Height int

	// Layout dimensions
	ProjectsWidth int
	ContentWidth  int
	ContentHeight int
}

// NewContext creates a new shell context
func NewContext() *Context {
	return &Context{}
}

// SetProject updates the selected project
func (c *Context) SetProject(path string) {
	c.ProjectPath = path
	if path == "" {
		c.ProjectName = "All Projects"
	} else {
		c.ProjectName = filepath.Base(path)
	}
}

// SetSize updates the window dimensions and recalculates layout
func (c *Context) SetSize(width, height int) {
	c.Width = width
	c.Height = height
	c.recalculateLayout()
}

func (c *Context) recalculateLayout() {
	// Reserve space for header and footer
	c.ContentHeight = c.Height - 4

	// Calculate pane widths
	minLeft := 22
	minRight := 40
	gap := 2

	if c.Width < minLeft+minRight+gap {
		// Single pane mode
		c.ProjectsWidth = 0
		c.ContentWidth = c.Width
	} else {
		c.ProjectsWidth = c.Width / 4
		if c.ProjectsWidth < minLeft {
			c.ProjectsWidth = minLeft
		}
		c.ContentWidth = c.Width - c.ProjectsWidth - gap
	}
}

// ToToolpaneContext converts to a toolpane.Context for passing to panes
func (c *Context) ToToolpaneContext() toolpane.Context {
	return toolpane.Context{
		ProjectPath: c.ProjectPath,
		ProjectName: c.ProjectName,
		Width:       c.ContentWidth,
		Height:      c.ContentHeight,
	}
}

// IsSinglePane returns true if we're in single-pane mode
func (c *Context) IsSinglePane() bool {
	return c.ProjectsWidth == 0
}
