package fileio

import (
	"context"
	"io"
	"path/filepath"
	"strings"
)

// Kind is an external file format.
type Kind int

const (
	CSV Kind = iota + 1
	TSV
	XLSX
	SQLite
	Parquet
	WK1
	JSON
	NUON
)

// A fileFormat is everything about a Kind: its names, its extensions,
// how the UI offers it, and the functions reading and writing it. Import,
// Export, KindOf and every list of formats the UI shows come from the
// kinds table, so adding a format is a file with its importer (and
// exporter) and a row there.
type fileFormat struct {
	kind  Kind
	name  string   // short name, e.g. in "Download as CSV"
	noun  string   // what people call it in a sentence, e.g. "Excel"
	label string   // what the file is, for the import picker
	exts  []string // recognized extensions, the first one written
	read  importer

	// Exporting; write is nil for formats 012 only imports.
	write exporter
	menu  string // the Download menu's title, without the extension
	about string // what downloading writes, for the command's description
	// encode is set for text formats, which Encode writes to a stream
	// (012 --pipe's standard output); it returns the rows written.
	encode func(io.Writer, *Snapshot) (int, error)

	// book is set for formats that hold named sheets of their own:
	// every sheet is exported, and an imported file keeps its sheets'
	// names. A file of any other kind has one sheet, named after it.
	book bool
	// tables is set for databases: an import picks a table, and an
	// export writes the sheet or the selection as a named table.
	tables bool
}

// An importer reads a file into a new workbook, returning the sheet to
// show. It checks ctx between rows and updates opt.Progress.
type importer func(ctx context.Context, name string, opt Options) (*Result, error)

// An exporter writes a snapshot to a file.
type exporter func(ctx context.Context, name string, snap *Snapshot, opt ExportOptions) (*ExportResult, error)

// kinds are the formats, in Kind order, which is menu order.
var kinds = []fileFormat{{
	kind: CSV, name: "CSV", noun: "CSV", label: "Comma-separated values", exts: []string{".csv"},
	read: importCSV, write: exportCSV, menu: "Comma-separated values", encode: encodeCSV,
	about: "Save the values as shown, as comma-separated values (.csv)",
}, {
	kind: TSV, name: "TSV", noun: "TSV", label: "Tab-separated values", exts: []string{".tsv", ".tab"},
	read: importTSV, write: exportTSV, menu: "Tab-separated values", encode: encodeTSV,
	about: "Save the values as shown, as tab-separated values (.tsv)",
}, {
	kind: XLSX, name: "XLSX", noun: "Excel", label: "Excel workbook", exts: []string{".xlsx", ".xlsm"},
	read: importXLSX, write: exportXLSX, menu: "Microsoft Excel",
	about: "Save as an Excel workbook (.xlsx) with formulas, formats and widths",
	book:  true,
}, {
	kind: SQLite, name: "SQLite", noun: "SQLite", label: "SQLite database", exts: []string{".sqlite", ".sqlite3", ".db"},
	read: importSQLite, write: exportSQLite, menu: "SQLite database",
	about:  "Save the sheet, or the selection, as a table in a SQLite database",
	tables: true,
}, {
	kind: Parquet, name: "Parquet", noun: "Parquet", label: "Parquet file", exts: []string{".parquet"},
	read: importParquet,
}, {
	kind: WK1, name: "WK1", noun: "1-2-3", label: "Lotus 1-2-3 worksheet", exts: []string{".wk1", ".wks"},
	read: importWK1,
}, {
	kind: JSON, name: "JSON", noun: "JSON", label: "JSON list of records", exts: []string{".json"},
	read: importJSON, write: exportJSON, menu: "JSON list of records", encode: encodeJSON,
	about: "Save the sheet as a JSON list of records, named by its first row",
}, {
	kind: NUON, name: "NUON", noun: "NUON", label: "Nushell table (NUON)", exts: []string{".nuon"},
	read: importNUON, write: exportNUON, menu: "Nushell table", encode: encodeNUON,
	about: "Save the sheet as a nushell table that keeps its types, named by its first row",
}}

// unknownFormat is what an invalid Kind reports.
var unknownFormat = fileFormat{name: "unknown", exts: []string{""}}

// format returns the kind's row of the table.
func (k Kind) format() *fileFormat {
	if k < 1 || int(k) > len(kinds) {
		return &unknownFormat
	}
	return &kinds[k-1]
}

// Kinds lists every format, in menu order.
func Kinds() []Kind {
	out := make([]Kind, len(kinds))
	for i, k := range kinds {
		out[i] = k.kind
	}
	return out
}

// KindOf recognizes a file's format by its extension.
func KindOf(name string) (Kind, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	for _, k := range kinds {
		for _, e := range k.exts {
			if e == ext {
				return k.kind, true
			}
		}
	}
	return 0, false
}

// String is the format's short name, e.g. "XLSX".
func (k Kind) String() string { return k.format().name }

// Noun is what the format is called in a sentence, e.g. "Excel" in
// "Import a CSV, TSV, Excel ... file".
func (k Kind) Noun() string { return k.format().noun }

// Label says what a file of this kind is, e.g. "Excel workbook".
func (k Kind) Label() string { return k.format().label }

// Ext is the extension written for this kind, e.g. ".xlsx".
func (k Kind) Ext() string { return k.format().exts[0] }

// CanExport reports whether sheets can be written in this format.
func (k Kind) CanExport() bool { return k.format().write != nil }

// MenuTitle is the format's entry in the Download menu, e.g. "Microsoft
// Excel (.xlsx)"; empty for formats that can't be exported.
func (k Kind) MenuTitle() string {
	if !k.CanExport() {
		return ""
	}
	return k.format().menu + " (" + k.Ext() + ")"
}

// About says what downloading in this format writes; empty for formats
// that can't be exported.
func (k Kind) About() string { return k.format().about }

// HoldsSheets reports whether a file of this kind holds several named
// sheets, so a download writes the whole workbook (see SnapBook).
func (k Kind) HoldsSheets() bool { return k.format().book }

// IsText reports whether this kind is text Encode writes to a stream:
// CSV, TSV, JSON or NUON.
func (k Kind) IsText() bool { return k.format().encode != nil }

// KindNamed finds a kind by its short name, ignoring case: "nuon".
func KindNamed(name string) (Kind, bool) {
	for _, k := range kinds {
		if strings.EqualFold(k.name, name) {
			return k.kind, true
		}
	}
	return 0, false
}

// HasTables reports whether a file of this kind is a database of tables:
// importing picks one (see Tables), and a download writes the sheet or
// the selection as a named table (ExportOptions.Table).
func (k Kind) HasTables() bool { return k.format().tables }
