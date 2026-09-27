package fileio

import (
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Array formulas in XLSX. Excel 365 keeps a formula that spills (a
// dynamic array formula) as an array formula over the cells it spills
// into, <f t="array" ref="B2:C9">, on a cell whose metadata (cm) marks
// it dynamic, as metadata.xml defines; the cells it spills into hold
// their values. Older Excels read it as a legacy array formula ({=...})
// showing the same values. Formulas calling array functions are written
// this way even when they compute one value, so Excel doesn't read them
// with its implicit intersection (@).
//
// Names LET and LAMBDA bind are written with Excel's _xlpm. prefix, an
// ARRAYFORMULA around a whole formula is dropped (a dynamic array formula
// computes over arrays anyway), and array literals must hold constants,
// as Excel's do, or the formula is saved as its value.

// arrayFns are the functions that return arrays, in upper case.
var arrayFns = map[string]bool{
	"ARRAYFORMULA": true, "FILTER": true, "SORT": true, "SORTN": true, "UNIQUE": true, "SEQUENCE": true,
	"TRANSPOSE": true, "FLATTEN": true, "CHOOSECOLS": true, "CHOOSEROWS": true, "LET": true, "LAMBDA": true,
	"MAP": true, "SCAN": true, "BYROW": true, "BYCOL": true, "MAKEARRAY": true, "SPLIT": true,
}

// excelArrays rewrites a formula that may compute an array for Excel, as
// above, and reports whether it does compute one (a dynamic array
// formula), and false when Excel can't hold it.
func excelArrays(input string) (string, bool, bool) {
	if !mayHoldArrays(input) {
		return input, false, true
	}
	n, err := sheet.Parse(input)
	if err != nil {
		return input, false, true // the text is translated as it is
	}
	arrays, ok := false, true
	if c, isCall := n.(formula.Call); isCall && c.Fn.Signature().Name == "ARRAYFORMULA" {
		n, arrays = c.Args[0], true
	}
	var walk func(n formula.Node)
	walk = func(n formula.Node) {
		switch n := n.(type) {
		case formula.Call:
			arrays = arrays || arrayFns[n.Fn.Signature().Name]
		case formula.Array:
			arrays = true
			ok = ok && constants(n)
		}
		formula.EachChild(n, walk)
	}
	walk(n)
	n, _ = formula.Rewrite(n, formula.Rewriter{Local: func(l formula.Local) formula.Node {
		return formula.Name{Name: "_xlpm." + l.Name}
	}})
	return formula.Text(n), arrays, ok
}

// mayHoldArrays reports whether a formula's text may hold an array
// literal or call an array function, so only those are parsed.
func mayHoldArrays(input string) bool {
	for i := 0; i < len(input); i++ {
		c := input[i]
		if c == '{' {
			return true
		}
		if !isIdentByte(c) || i > 0 && isIdentByte(input[i-1]) {
			continue
		}
		j := i
		for j < len(input) && isIdentByte(input[j]) {
			j++
		}
		if j < len(input) && input[j] == '(' && arrayFns[upperASCII(input[i:j])] {
			return true
		}
		i = j - 1
	}
	return false
}

func upperASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		b[i] = upper(c)
	}
	return string(b)
}

// constants reports whether an array literal holds only constants.
func constants(a formula.Array) bool {
	for _, row := range a.Rows {
		for _, e := range row {
			if u, ok := e.(formula.Unary); ok && u.Op == "-" {
				e = u.X
			}
			switch e.(type) {
			case formula.Num, formula.Str, formula.Bool:
			default:
				return false
			}
		}
	}
	return true
}

// metadataXML defines cell metadata 1 as a dynamic array formula, as
// Excel writes it.
const metadataXML = xmlHead + `<metadata xmlns="` + sheetMain + `" xmlns:xda="http://schemas.microsoft.com/office/spreadsheetml/2017/dynamicarray">` +
	`<metadataTypes count="1"><metadataType name="XLDAPR" minSupportedVersion="120000" copy="1" pasteAll="1" pasteValues="1" merge="1" splitFirst="1" rowColShift="1" clearFormats="1" clearComments="1" assign="1" coerce="1" cellMeta="1"/></metadataTypes>` +
	`<futureMetadata name="XLDAPR" count="1"><bk><extLst><ext uri="{bdbb8cdc-fa1e-496e-a857-3c3f30c029c3}"><xda:dynamicArrayProperties fDynamic="1" fCollapsed="0"/></ext></extLst></bk></futureMetadata>` +
	`<cellMetadata count="1"><bk><rc t="1" v="0"/></bk></cellMetadata></metadata>`

// arrayRef is the ref of a dynamic array formula at a: the cells it
// spills into, or itself.
func arrayRef(a sheet.Addr, spill sheet.Rect) string {
	if spill == (sheet.Rect{}) || spill.From == spill.To {
		return a.String()
	}
	return spill.From.String() + ":" + spill.To.String()
}
