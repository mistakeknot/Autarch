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
// Known limitation: each file is parsed alone, so a package-level declaration such as "type rune = byte" in a sibling
// file is not seen and the file is still reported (see the "rune redeclared in a sibling file" case). Nothing
// in the repository redeclares rune; if that ever changes, resolve package-level types here.
// The check parses the file, so comments and string literals never count, lipgloss imported under another name
// still counts, and a []rune literal such as []rune{','} is not a conversion.

const lipglossPath = "github.com/charmbracelet/lipgloss"

// runeWidthPattern reports whether src converts a value with []rune(...) and also calls lipgloss.Width(...).
func runeWidthPattern(src string) (bool, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
	if err != nil {
		return false, err
	}
	names := map[string]bool{}
	dot := false
	for _, imp := range f.Imports {
		p, uerr := strconv.Unquote(imp.Path.Value)
		if uerr != nil || p != lipglossPath {
			continue
		}
		name := "lipgloss"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			dot = true
		} else {
			names[name] = true
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
			if id, ok := ast.Unparen(fn.Elt).(*ast.Ident); ok && id.Name == "rune" && fn.Len == nil && isRuneType(id) {
				conv = true
			}
		case *ast.SelectorExpr:
			if id, ok := fn.X.(*ast.Ident); ok && id.Obj == nil && names[id.Name] && fn.Sel.Name == "Width" {
				width = true // an identifier with a local binding (Obj set) shadows the import
			}
		case *ast.Ident:
			if dot && fn.Name == "Width" && fn.Obj == nil {
				width = true
			}
		}
		return true
	})
	return conv && width, nil
}

// isRuneType reports whether id names the builtin rune, or a file-local declaration that is int32 under another
// spelling ("type rune = int32" or "type rune int32"), which still converts a string to a slice of runes.
func isRuneType(id *ast.Ident) bool {
	if id.Obj == nil {
		return true
	}
	ts, ok := id.Obj.Decl.(*ast.TypeSpec)
	if !ok {
		return false
	}
	base, ok := ast.Unparen(ts.Type).(*ast.Ident)
	return ok && base.Name == "int32" && base.Obj == nil
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
		{"dot import", "package x\nimport . \"github.com/charmbracelet/lipgloss\"\nfunc f(s string) { _ = []rune(s)[:Width(s)] }\n", true},
		{"parenthesized element type", "package x\nimport \"github.com/charmbracelet/lipgloss\"\nfunc f(s string) { _ = [](rune)(s)[:lipgloss.Width(s)] }\n", true},
		{"shadowed alias", "package x\nimport lg \"github.com/charmbracelet/lipgloss\"\nvar _ = lg.Width\ntype w struct{}\nfunc (w) Width(string) int { return 0 }\nfunc f(s string) { lg := w{}; _ = []rune(s)[:lg.Width(s)] }\n", false},
		{"two imports of lipgloss", "package x\nimport lg \"github.com/charmbracelet/lipgloss\"\nimport other \"github.com/charmbracelet/lipgloss\"\nvar _ = other.Width\nfunc f(s string) { _ = []rune(s)[:lg.Width(s)] }\n", true},
		{"rune redeclared in a sibling file (limitation: one file is parsed at a time, so this is still reported)", "package x\nimport lg \"github.com/charmbracelet/lipgloss\"\nfunc f(s string) { _ = []rune(s)[:lg.Width(s)] }\n", true},
		{"rune alias to int32", "package x\nimport lg \"github.com/charmbracelet/lipgloss\"\ntype rune = int32\nfunc f(s string) { _ = []rune(s)[:lg.Width(s)] }\n", true},
		{"rune defined as int32", "package x\nimport lg \"github.com/charmbracelet/lipgloss\"\ntype rune int32\nfunc f(s string) { _ = []rune(s)[:lg.Width(s)] }\n", true},
		{"rune shadowed by an unrelated type in the same file", "package x\nimport lg \"github.com/charmbracelet/lipgloss\"\ntype rune struct{}\nfunc f(r []rune, s string) { _ = lg.Width(s); _ = []rune(r) }\n", false},
		{"rune shadowed by a local variable", "package x\nimport lg \"github.com/charmbracelet/lipgloss\"\nfunc f(s string) { rune := 1; _ = rune; _ = lg.Width(s) }\n", false},
		{"dot-imported Width shadowed in the same file", "package x\nimport . \"github.com/charmbracelet/lipgloss\"\nfunc Width(string) int { return 0 }\nfunc f(s string) { _ = []rune(s)[:Width(s)] }\n", false},
		{"another package named lipgloss", "package x\nimport lipgloss \"example.com/other\"\nfunc f(s string) { _ = []rune(s); _ = lipgloss.Width(s) }\n", false},
	}
	if _, err := runeWidthPattern("package x\nfunc f( {"); err == nil {
		t.Error("malformed source: want a parse error, got none")
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
