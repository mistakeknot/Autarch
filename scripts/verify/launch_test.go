//go:build linux

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLaunchSeamRefusesLiveAndShellCommands(t *testing.T) {
	bb := "/nonexistent/" + strings.Join([]string{"b", "b"}, "")
	for _, argv := range [][]string{
		{bb}, {"/usr/bin/env", "-i", bb, "home", "list"},
		{"/bin/sh", "-c", "exit 0"},
		{"/nonexistent/scripts/home-ui.sh", "preview"},
		{"/usr/bin/bwrap", "--unshare-net", "--", bb},
	} {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			cmd := exec.Command(argv[0], argv[1:]...)
			err, timedOut := waitGroup(cmd, time.Second)
			if err == nil || !strings.Contains(err.Error(), "launch policy") || timedOut || cmd.Process != nil {
				t.Fatalf("command reached exec: %v, %v, process=%v", err, timedOut, cmd.Process)
			}
		})
	}
	cmd := exec.Command("/usr/bin/true")
	cmd.Path = bb
	if err, _ := waitGroup(cmd, time.Second); err == nil || !strings.Contains(err.Error(), "launch policy") || cmd.Process != nil {
		t.Fatal("disguised executable reached exec", err)
	}
}

// Every process creation API in every non-test source must use the guarded
// lifetime seam. New executable helper files require an explicit policy review.
func TestNoLiveLaunchCommands(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.HasSuffix(path, "_test.go") || path == "map.tsv" {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			t.Errorf("unreviewed executable helper input: %s", path)
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		imports := map[string]string{}
		for _, imp := range file.Imports {
			pkg, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			alias := filepath.Base(pkg)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			if alias == "." {
				t.Errorf("dot import obscures process APIs: %s", path)
			}
			imports[alias] = pkg
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var guard token.Pos
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "checkLaunch" {
					guard = call.Pos()
				}
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "resolve" && fn.Name.Name == "nodeToolchain" {
					lit, ok := call.Args[0].(*ast.BasicLit)
					if !ok || (lit.Value != `"node"` && lit.Value != `"npm"`) {
						t.Errorf("unreviewed dynamic executable lookup: %s", path)
					}
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				id, _ := sel.X.(*ast.Ident)
				pkg := ""
				if id != nil {
					pkg = imports[id.Name]
				}
				switch {
				case pkg == "os/exec" && sel.Sel.Name == "LookPath":
					lit, constant := call.Args[0].(*ast.BasicLit)
					if constant {
						name, _ := strconv.Unquote(lit.Value)
						if name != "node" && name != "npm" && name != "go" && name != "claude" {
							t.Errorf("unreviewed executable lookup: %s: %s", path, name)
						}
					} else if fn.Name.Name != "nodeToolchain" {
						t.Errorf("opaque executable lookup: %s:%s", path, fn.Name.Name)
					}
				case pkg == "os/exec" && strings.HasPrefix(sel.Sel.Name, "Command"):
					if sel.Sel.Name != "Command" || (fn.Name.Name != "git" && fn.Name.Name != "runGroup") {
						t.Errorf("process constructor bypasses the guarded wrappers: %s:%s", path, fn.Name.Name)
					}
				case (pkg == "os" || pkg == "syscall") && (sel.Sel.Name == "StartProcess" || sel.Sel.Name == "ForkExec" || sel.Sel.Name == "Exec" || strings.Contains(sel.Sel.Name, "Syscall")):
					t.Errorf("raw process API: %s:%s", path, fn.Name.Name)
				case sel.Sel.Name == "Start" || sel.Sel.Name == "Run" || sel.Sel.Name == "Output" || sel.Sel.Name == "CombinedOutput":
					if fn.Name.Name != "waitGroup" || sel.Sel.Name != "Start" || guard == token.NoPos || guard >= call.Pos() {
						t.Errorf("unguarded process start: %s:%s", path, fn.Name.Name)
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
