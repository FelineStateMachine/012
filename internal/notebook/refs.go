package notebook

import "strings"

// What a pipeline reads. A code cell reads another's output as $name,
// the selection on a sheet as $selection, and a range of a sheet as
// $sheet.A1:C9 (or $sheet.Sales!A1:C9, $sheet.'Q1 data'!A1:B5). Each
// reaches nu as a variable holding a table, read from a NUON file, never
// as text spliced into the pipeline: a $sheet range is renamed to a
// variable of its own ($__sheet1) before the pipeline runs
// (Source.Command).

// Selection is the variable that holds the selection.
const Selection = "selection"

// sheetVar starts a range of a sheet.
const sheetVar = "$sheet."

// SheetRef is a range of a sheet a pipeline reads, $sheet.A1:C9.
type SheetRef struct {
	// Ref is the range as written after $sheet.: "A1:C9", "Sales!A1:C9".
	Ref string
	// Var is the variable it's read as once renamed: __sheet1.
	Var string
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
