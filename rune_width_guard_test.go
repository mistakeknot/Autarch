package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A []rune conversion in the same file as a lipgloss.Width call is the pattern that slices ANSI-styled strings by
// rune count where visual columns are meant. Use ansi.Truncate/TruncateLeft from charmbracelet/x/ansi instead.
// The check parses the file, so comments and string literals never count, lipgloss imported under another name
// still counts, and a []rune literal such as []rune{','} is not a conversion.

const lipglossPath = "github.com/charmbracelet/lipgloss"

// runeWidthPattern reports whether src converts a value with []rune(...) and also calls lipgloss.Width(...).
func runeWidthPattern(src string) (bool, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
	if err != nil {
		return false, err
	}
	lipgloss := ""
	for _, imp := range f.Imports {
		p, uerr := strconv.Unquote(imp.Path.Value)
		if uerr != nil || p != lipglossPath {
			continue
		}
		lipgloss = "lipgloss"
		if imp.Name != nil {
			lipgloss = imp.Name.Name
		}
	}
	var conv, width bool
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := ast.Unparen(call.Fun).(type) {
		case *ast.ArrayType:
			if id, ok := fn.Elt.(*ast.Ident); ok && id.Name == "rune" && fn.Len == nil {
				conv = true
			}
		case *ast.SelectorExpr:
			if id, ok := fn.X.(*ast.Ident); ok && lipgloss != "" && id.Name == lipgloss && fn.Sel.Name == "Width" {
				width = true
			}
		}
		return true
	})
	return conv && width, nil
}

func TestRuneWidthPatternDetector(t *testing.T) {
	const head = "package x\nimport lg \"github.com/charmbracelet/lipgloss\"\nvar _ = lg.Width\n"
	cases := []struct {
		name, src string
		want      bool
	}{
		{"conversion and width", "package x\nimport \"github.com/charmbracelet/lipgloss\"\nfunc f(s string) { _ = []rune(s); _ = lipgloss.Width(s) }\n", true},
		{"comment between type and parens", "package x\nimport \"github.com/charmbracelet/lipgloss\"\nfunc f(s string) { _ = []rune /* c */ (s)[:lipgloss.Width(s)] }\n", true},
		{"import alias", "package x\nimport lg \"github.com/charmbracelet/lipgloss\"\nfunc f(s string) { _ = []rune(s)[:lg.Width(s)] }\n", true},
		{"rune literal is not a conversion", "package x\nimport \"github.com/charmbracelet/lipgloss\"\nvar k = []rune{','}\nvar _ = lipgloss.Width(\"a\")\n", false},
		{"comment only", "package x\nimport \"github.com/charmbracelet/lipgloss\"\n// Avoid []rune(s); use lipgloss.Width(s).\nvar _ = 1\n", false},
		{"string only", "package x\nvar _ = \"[]rune(s) lipgloss.Width(s)\"\n", false},
		{"conversion alone", "package x\nfunc f(s string) { _ = []rune(s) }\n", false},
		{"width alone", head, false},
		{"another package named lipgloss", "package x\nimport lipgloss \"example.com/other\"\nfunc f(s string) { _ = []rune(s); _ = lipgloss.Width(s) }\n", false},
	}
	for _, c := range cases {
		got, err := runeWidthPattern(c.src)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNoRuneSlicingWithLipglossWidth(t *testing.T) {
	var bad []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "vendor", "testdata", ".claude", ".clavain", ".playwright-mcp":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hit, perr := runeWidthPattern(string(b))
		if perr != nil {
			t.Errorf("%s does not parse: %v", path, perr)
			return nil
		}
		if hit {
			bad = append(bad, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Fatalf("[]rune conversion alongside lipgloss.Width in %v: slice visual columns with ansi.Truncate/TruncateLeft from charmbracelet/x/ansi", bad)
	}
}
