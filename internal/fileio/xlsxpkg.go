package fileio

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

// An XLSX file is a zip of XML parts (SpreadsheetML in an Open Packaging
// Conventions package). 012 reads it with archive/zip and encoding/xml,
// streaming each part a token at a time, so a worksheet is never held in
// memory. The file is untrusted: xlsxLimits bound what it can make the
// reader do.

// xlsxLimits bound the work a file can cause. encoding/xml expands no
// external or custom entities, so entity bombs can't happen; these cap
// zip bombs, deep nesting and huge single tokens.
type xlsxLimits struct {
	entries int   // files in the zip
	part    int64 // uncompressed bytes of one part
	total   int64 // uncompressed bytes of all parts read
	ratio   int64 // uncompressed / compressed, past slack bytes
	slack   int64
	depth   int   // XML nesting
	token   int64 // bytes of one XML token (a tag or a run of text)
	strings int   // shared strings
	styles  int   // cell formats, fonts and number formats, each
	sheets  int
}

var defaultXLSXLimits = xlsxLimits{
	entries: 10_000,
	part:    1 << 30,
	total:   2 << 30,
	ratio:   250,
	slack:   16 << 20,
	depth:   256,
	token:   32 << 20,
	strings: 1 << 24,
	styles:  1 << 16,
	sheets:  4096,
}

// errXLSXLimit is wrapped by every error for a file past a limit.
var errXLSXLimit = errors.New("past the reader's limits")

// xlsxPackage is an open XLSX zip: its parts by lower-case name (part
// names are case-insensitive) and what has been read of them so far.
type xlsxPackage struct {
	parts map[string]*zip.File
	lim   xlsxLimits
	read  int64 // uncompressed bytes read, all parts
}

func openXLSXPackage(r io.ReaderAt, size int64, lim xlsxLimits) (*xlsxPackage, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return nil, fmt.Errorf("not an Excel workbook, or an encrypted one (%w)", err)
	}
	if len(zr.File) > lim.entries {
		return nil, fmt.Errorf("the workbook holds %d files, more than %d: %w", len(zr.File), lim.entries, errXLSXLimit)
	}
	p := &xlsxPackage{parts: make(map[string]*zip.File, len(zr.File)), lim: lim}
	for _, f := range zr.File {
		name := f.Name
		if strings.HasSuffix(name, "/") {
			continue // a directory
		}
		if !fs.ValidPath(name) || strings.ContainsRune(name, '\\') {
			return nil, fmt.Errorf("the workbook holds a file with an unsafe name, %q", name)
		}
		key := strings.ToLower(name)
		if _, dup := p.parts[key]; dup {
			return nil, fmt.Errorf("the workbook holds %s twice", name)
		}
		p.parts[key] = f
	}
	return p, nil
}

// has reports whether the package holds a part.
func (p *xlsxPackage) has(name string) bool {
	_, ok := p.parts[strings.ToLower(name)]
	return ok
}

// open starts streaming a part's XML, or returns nil, nil when there is
// no such part.
func (p *xlsxPackage) open(name string) (*xmlStream, error) {
	f, ok := p.parts[strings.ToLower(name)]
	if !ok {
		return nil, nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	g := &guardReader{r: rc, c: rc, p: p, name: name, comp: int64(f.CompressedSize64)}
	d := xml.NewDecoder(g)
	return &xmlStream{d: d, g: g, name: name, maxDepth: p.lim.depth}, nil
}

// guardReader counts a part's uncompressed bytes as they are read and
// fails past the limits, so a zip bomb stops early whatever its headers
// claim.
type guardReader struct {
	r    io.Reader
	c    io.Closer
	p    *xlsxPackage
	name string
	comp int64 // compressed size
	n    int64 // uncompressed bytes read
	tok  int64 // n when the current XML token began
}

func (g *guardReader) Read(b []byte) (int, error) {
	k, err := g.r.Read(b)
	g.n += int64(k)
	g.p.read += int64(k)
	lim := g.p.lim
	switch {
	case g.n > lim.part:
		return k, fmt.Errorf("%s unpacks to more than %s: %w", g.name, mb(lim.part), errXLSXLimit)
	case g.p.read > lim.total:
		return k, fmt.Errorf("the workbook unpacks to more than %s: %w", mb(lim.total), errXLSXLimit)
	case g.n > lim.slack && g.n/max(g.comp, 1) > lim.ratio:
		return k, fmt.Errorf("%s is compressed over %d to 1, like a zip bomb: %w", g.name, lim.ratio, errXLSXLimit)
	case g.n-g.tok > lim.token:
		return k, fmt.Errorf("%s has a tag or text over %s: %w", g.name, mb(lim.token), errXLSXLimit)
	}
	return k, err
}

func mb(n int64) string { return fmt.Sprintf("%d MB", n>>20) }

// xmlStream is a part's XML as tokens, checking nesting depth and the
// size of each token.
type xmlStream struct {
	d        *xml.Decoder
	g        *guardReader
	name     string
	depth    int
	maxDepth int
}

// next returns the next token, or io.EOF at the end of the part. Char
// data is only valid until the following call.
func (x *xmlStream) next() (xml.Token, error) {
	x.g.tok = x.g.n
	t, err := x.d.Token()
	if err != nil {
		if err == io.EOF {
			return nil, err
		}
		return nil, fmt.Errorf("%s: %w", x.name, err)
	}
	switch t.(type) {
	case xml.StartElement:
		x.depth++
		if x.depth > x.maxDepth {
			return nil, fmt.Errorf("%s nests elements more than %d deep: %w", x.name, x.maxDepth, errXLSXLimit)
		}
	case xml.EndElement:
		x.depth--
	}
	return t, nil
}

// skip passes over the rest of the element just started.
func (x *xmlStream) skip() error {
	for depth := x.depth; ; {
		t, err := x.next()
		if err != nil {
			return eofAsUnexpected(err)
		}
		if _, ok := t.(xml.EndElement); ok && x.depth < depth {
			return nil
		}
	}
}

// text returns the text of the element just started, up to its end.
func (x *xmlStream) text(buf []byte) ([]byte, error) {
	for depth := x.depth; ; {
		t, err := x.next()
		if err != nil {
			return buf, eofAsUnexpected(err)
		}
		switch t := t.(type) {
		case xml.CharData:
			buf = append(buf, t...)
		case xml.EndElement:
			if x.depth < depth {
				return buf, nil
			}
		}
	}
}

func (x *xmlStream) close() error { return x.g.c.Close() }

func eofAsUnexpected(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

// attr returns the value of the attribute with local name name.
func attr(se xml.StartElement, name string) (string, bool) {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

// relAttr returns the r:id of an element: the id attribute in the
// relationships namespace (transitional or strict).
func relAttr(se xml.StartElement) string {
	for _, a := range se.Attr {
		if a.Name.Local == "id" && strings.HasSuffix(a.Name.Space, "/relationships") {
			return a.Value
		}
	}
	return ""
}

// xlsxRel is a relationship from a part to another.
type xlsxRel struct {
	typ    string // the last element of the type URI, e.g. "worksheet"
	target string // the part's name, resolved
}

// rels reads the relationships of part (from its _rels part), resolving
// targets to part names. Targets outside the package are left out.
func (p *xlsxPackage) rels(part string) (map[string]xlsxRel, error) {
	dir, base := path.Split(part)
	x, err := p.open(dir + "_rels/" + base + ".rels")
	if x == nil || err != nil {
		return nil, err
	}
	defer x.close()
	out := map[string]xlsxRel{}
	for {
		t, err := x.next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		se, ok := t.(xml.StartElement)
		if !ok || se.Name.Local != "Relationship" {
			continue
		}
		if mode, _ := attr(se, "TargetMode"); strings.EqualFold(mode, "External") {
			continue
		}
		id, _ := attr(se, "Id")
		typ, _ := attr(se, "Type")
		target, _ := attr(se, "Target")
		if name, ok := resolvePart(dir, target); ok && len(out) < p.lim.entries {
			out[id] = xlsxRel{typ: typ[strings.LastIndexByte(typ, '/')+1:], target: name}
		}
	}
}

// resolvePart resolves a relationship's target against the directory of
// its source part: /xl/x.xml is from the root, x.xml from dir. A target
// that climbs out of the package is refused.
func resolvePart(dir, target string) (string, bool) {
	if target == "" {
		return "", false
	}
	if strings.HasPrefix(target, "/") {
		target = target[1:]
	} else {
		target = dir + target
	}
	name := path.Clean(target)
	if !fs.ValidPath(name) || name == "." {
		return "", false
	}
	return name, true
}
