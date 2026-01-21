package tui

import "github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"

func LoadDiffFiles(r git.Runner, rev string) ([]string, error) {
	return git.DiffNameOnly(r, rev)
}
