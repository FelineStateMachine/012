package functions

import (
	"flag"
	"fmt"
	"go/ast"
	goparser "go/parser"
	gotoken "go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var updateDocs = flag.Bool("update-docs", false, "rewrite docs/functions.md from the function table")

// docsPath is the generated function reference.
const docsPath = "../../docs/functions.md"

// categories names the group of the functions each file defines, in the
// order the reference lists them.
var categories = []struct{ file, title string }{
	{"everyday.go", "Everyday"},
	{"math.go", "Math"},
	{"stats.go", "Statistics"},
	{"logic.go", "Logic and information"},
	{"text.go", "Text"},
	{"lookup.go", "Lookup"},
	{"date.go", "Date and time"},
	{"finance.go", "Finance"},
	{"link.go", "Links"},
	{"jev.go", "JEV (hosted model)"},
}

// TestFunctionsDoc keeps docs/functions.md in step with the function
// table, as help is: run `go test ./internal/functions -run FunctionsDoc
// -update-docs` after adding a function.
func TestFunctionsDoc(t *testing.T) {
	byFile, err := definingFiles()
	if err != nil {
		t.Fatal(err)
	}
	got := functionsDoc(byFile)
	if *updateDocs {
		if err := os.WriteFile(docsPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(docsPath)
	if err != nil || string(want) != got {
		t.Errorf("docs/functions.md is stale; run go test ./internal/functions -run FunctionsDoc -update-docs")
	}
}

// definingFiles maps each function name to the file whose FuncDef
// literal defines it.
func definingFiles() (map[string]string, error) {
	fset := gotoken.NewFileSet()
	out := map[string]string{}
	files, err := filepath.Glob("*.go")
	if err != nil {
		return nil, err
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := goparser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			kv, ok := n.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Name" {
				if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == gotoken.STRING {
					if name, err := strconv.Unquote(lit.Value); err == nil {
						out[name] = path
					}
				}
			}
			return true
		})
	}
	return out, nil
}

func functionsDoc(byFile map[string]string) string {
	var b strings.Builder
	b.WriteString("# Functions\n\n")
	b.WriteString("<!-- Generated from the engine's function table by TestFunctionsDoc; do not edit. -->\n\n")
	fmt.Fprintf(&b, "012 has %d functions. They follow Google Sheets' names, arguments and\n", len(Funcs()))
	b.WriteString("semantics; `[brackets]` mark optional arguments. Function names are\n")
	b.WriteString("case-insensitive, and 1-2-3's `@SUM(A1..A5)` spelling still works.\n\n")
	var aliasList []string
	for a, canon := range aliases {
		aliasList = append(aliasList, fmt.Sprintf("`%s` for `%s`", a, canon))
	}
	slices.Sort(aliasList)
	if len(aliasList) > 0 {
		fmt.Fprintf(&b, "Aliases: %s.\n\n", strings.Join(aliasList, ", "))
	}
	listed := map[string]bool{}
	section := func(title string, fns []*FuncDef) {
		if len(fns) == 0 {
			return
		}
		fmt.Fprintf(&b, "## %s\n\n| Function | Description |\n|---|---|\n", title)
		for _, f := range fns {
			sig := f.Name + "(" + f.Args + ")"
			desc := f.Desc
			if f.Volatile {
				desc += " (recalculates on every change)"
			}
			fmt.Fprintf(&b, "| `%s` | %s |\n", strings.ReplaceAll(sig, "|", `\|`), desc)
			listed[f.Name] = true
		}
		b.WriteString("\n")
	}
	for _, c := range categories {
		var fns []*FuncDef
		for _, f := range Funcs() {
			if byFile[f.Name] == c.file {
				fns = append(fns, f)
			}
		}
		section(c.title, fns)
	}
	var rest []*FuncDef
	for _, f := range Funcs() {
		if !listed[f.Name] {
			rest = append(rest, f)
		}
	}
	section("Other", rest)
	return b.String()
}
