package sheet

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestFileDocNamesFields checks docs/files/format.md names every field the .012
// format has, and docs/reference/macro-api.md a macro's, so a new field comes with its
// documentation.
func TestFileDocNamesFields(t *testing.T) {
	files := readDoc(t, "../../docs/files/format.md")
	macros := readDoc(t, "../../docs/reference/macro-api.md")
	seen := map[reflect.Type]bool{reflect.TypeFor[fileMacro](): true}
	text := files
	var walk func(reflect.Type, string)
	walk = func(ty reflect.Type, path string) {
		for ty.Kind() == reflect.Pointer || ty.Kind() == reflect.Slice || ty.Kind() == reflect.Map {
			ty = ty.Elem()
		}
		if ty.Kind() != reflect.Struct || seen[ty] {
			return
		}
		seen[ty] = true
		for f := range ty.Fields() {
			if f.Anonymous {
				walk(f.Type, path)
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			if !strings.Contains(text, "`"+name+"`") && !strings.Contains(text, `"`+name+`"`) {
				t.Errorf("the docs don't name the field %s.%s", path, name)
			}
			walk(f.Type, path+"."+name)
		}
	}
	for _, v := range []any{fileFormat{}, fileCell{}, fileChart{}} {
		walk(reflect.TypeOf(v), reflect.TypeOf(v).Name())
	}
	text, seen = macros, map[reflect.Type]bool{}
	walk(reflect.TypeFor[fileMacro](), "fileMacro")
}

func readDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
