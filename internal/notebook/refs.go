package notebook

import (
	"slices"
	"strconv"
	"strings"
)

// What a pipeline reads. A code cell reads another's output as $name,
// the selection on a sheet as $selection, and a range of a sheet as
// $sheet.A1:C9 (or $sheet.Sales!A1:C9, $sheet.'Q1 data'!A1:B5). Each
// reaches nu as a variable holding a table, read from a NUON file, never
// as text spliced into the pipeline: a $sheet range is renamed to a
// variable of its own ($__sheet1) before the pipeline runs.

// Selection is the variable that holds the selection.
const Selection = "selection"

// sheetVar starts a range of a sheet.
const sheetVar = "$sheet."

// Refs returns the names a pipeline reads as $name, in the order it
// first names them, leaving out nushell's variables and the notebook's
// own ($selection, $sheet).
func Refs(pipeline string) []string {
	var out []string
	for i := 0; i < len(pipeline); i++ {
		if pipeline[i] != '$' {
			continue
		}
		j := i + 1
		for j < len(pipeline) && isWord(pipeline[j]) {
			j++
		}
		name := pipeline[i+1 : j]
		if name != "" && ValidName(name) == nil && !slices.Contains(out, name) {
			out = append(out, name)
		}
		i = j - 1
	}
	return out
}

// ReadsSelection reports whether a pipeline reads $selection.
func ReadsSelection(pipeline string) bool {
	for i := 0; ; {
		k := strings.Index(pipeline[i:], "$"+Selection)
		if k < 0 {
			return false
		}
		end := i + k + 1 + len(Selection)
		if end == len(pipeline) || !isWord(pipeline[end]) {
			return true
		}
		i = end
	}
}

// SheetRef is a range of a sheet a pipeline reads, $sheet.A1:C9.
type SheetRef struct {
	// Ref is the range as written after $sheet.: "A1:C9", "Sales!A1:C9".
	Ref string
	// Var is the variable it's read as once renamed: __sheet1.
	Var string
}

// Bind renames each range of a sheet the pipeline reads ($sheet.A1:C9)
// to a variable of its own, returning the pipeline to run and the ranges
// in the order they appear, the same range read twice once.
func Bind(pipeline string) (string, []SheetRef) {
	var b strings.Builder
	var refs []SheetRef
	rest := pipeline
	for {
		k := strings.Index(rest, sheetVar)
		if k < 0 || k > 0 && isWord(rest[k-1]) {
			if k < 0 {
				b.WriteString(rest)
				return b.String(), refs
			}
			b.WriteString(rest[:k+len(sheetVar)])
			rest = rest[k+len(sheetVar):]
			continue
		}
		b.WriteString(rest[:k])
		rest = rest[k+len(sheetVar):]
		n := refLen(rest)
		if n == 0 {
			b.WriteString(sheetVar)
			continue
		}
		ref := rest[:n]
		rest = rest[n:]
		i := slices.IndexFunc(refs, func(r SheetRef) bool { return r.Ref == ref })
		if i < 0 {
			refs = append(refs, SheetRef{Ref: ref, Var: "__sheet" + strconv.Itoa(len(refs)+1)})
			i = len(refs) - 1
		}
		b.WriteString("$" + refs[i].Var)
	}
}

// refLen is how long the range at the start of s is: a sheet's name,
// quoted in ” if it needs to be, then ! and cells, or cells alone. A
// dot ends it, so a cell path may follow: $sheet.A1:C9.name.
func refLen(s string) int {
	i := 0
	if strings.HasPrefix(s, "'") {
		for i = 1; i < len(s); i++ {
			if s[i] == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++ // '' is a quote in the name
					continue
				}
				break
			}
		}
		if i >= len(s) || i+1 >= len(s) || s[i+1] != '!' {
			return 0
		}
		i += 2
	}
	for i < len(s) && (isWord(s[i]) || strings.IndexByte("!:$", s[i]) >= 0) {
		i++
	}
	return i
}
