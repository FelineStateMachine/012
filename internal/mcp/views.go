package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Views are the range read_range read and the chart create_chart made,
// drawn as 012 draws them in a sandboxed frame beside the result, by
// hosts that implement either of the two ways of doing that:
//
//   - MCP Apps (the io.modelcontextprotocol/ui extension, spec
//     2026-01-26): the tool's _meta.ui.resourceUri names ui://012/view,
//     text/html;profile=mcp-app, and the page speaks the extension's
//     messages over postMessage (ui/initialize, the tool's result, the
//     host's theme and room, the page's size).
//   - The OpenAI Apps SDK (ChatGPT, Codex): the tool's
//     _meta["openai/outputTemplate"] names ui://012/view.skybridge, the
//     same page as text/html+skybridge, which reads window.openai
//     (toolResponseMetadata, theme, maxHeight, displayMode) instead.
//
// The page is the HTML export's with nothing in it yet (fonts,
// palettes, the pointer's script). What it draws is in the result's
// _meta under viewKey, for every client, since both kinds of hosts hand
// a result's _meta to the view and neither shows it to the model: its
// title, where it is and the grid's or chart's HTML. structuredContent
// stays the tool's result, what the model reads.
const (
	uiExtension   = "io.modelcontextprotocol/ui"
	uiMIME        = "text/html;profile=mcp-app"
	skybridgeMIME = "text/html+skybridge"
	viewURI       = "ui://012/view"
	viewKey       = "o12/view"
	skybridgeURI  = "ui://012/view.skybridge"
	// maxViewRows bounds the rows a range's view draws.
	maxViewRows = 200
)

// View is what a view draws of a result, in its _meta under viewKey:
// the workbook's name, what the name box shows before a cell is
// pointed at (a chart's data), the range or the chart's kind, sheet and
// data, and the grid or chart as the HTML export draws it.
type View struct {
	Title string `json:"title"`
	Name  string `json:"name,omitempty"`
	Where string `json:"where"`
	HTML  string `json:"html"`
}

// viewResourceMeta says how hosts should frame the view, in both
// dialects: a border, and no network (an empty content security
// policy's lists).
var viewResourceMeta = sdk.Meta{
	"ui":                         map[string]any{"prefersBorder": true, "csp": map[string]any{}},
	"openai/widgetPrefersBorder": true,
	"openai/widgetCSP":           map[string]any{"connect_domains": []string{}, "resource_domains": []string{}},
	"openai/widgetDescription":   "The range or chart the tool returned, drawn as 012's grid: the model need not repeat it.",
}

func (s *Server) addViews() {
	for _, r := range []struct{ uri, mime string }{{viewURI, uiMIME}, {skybridgeURI, skybridgeMIME}} {
		s.AddResource(&sdk.Resource{URI: r.uri, Name: "012 view", Title: "012", MIMEType: r.mime, Meta: viewResourceMeta,
			Description: "The range or chart a tool returned, drawn as 012 draws it"},
			func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
				return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: r.uri, MIMEType: r.mime, Text: viewPage(), Meta: viewResourceMeta}}}, nil
			})
	}
}

// viewPage is the view's page: the panel, an empty place for the view
// and the scripts.
func viewPage() string {
	return fileio.HTMLDocument("012", "<style>"+viewCSS+"</style>"+fileio.HTMLPanel("012", "", "")+`<div id="o12-view"></div>`, viewScript)
}

// viewMeta is what a tool drawn in the view says of it: the resource
// under MCP Apps' key and its first drafts' flat one, and the Apps
// SDK's template and the status lines it shows while the tool runs.
// The view calls no tools, so openai/widgetAccessible stays false.
func viewMeta(invoking, invoked string) sdk.Meta {
	return sdk.Meta{"ui": map[string]any{"resourceUri": viewURI}, "ui/resourceUri": viewURI,
		"openai/outputTemplate": skybridgeURI, "openai/toolInvocation/invoking": invoking, "openai/toolInvocation/invoked": invoked}
}

// viewResult is a result carrying v in its _meta, nil without one.
func viewResult(v *View) *sdk.CallToolResult {
	if v == nil {
		return nil
	}
	return &sdk.CallToolResult{Meta: sdk.Meta{viewKey: v}}
}

// rangeView draws the rows read_range returned of t, at most
// maxViewRows.
func rangeView(b *book, t headless.Target, rows int) *View {
	r := t.Range
	if t.Whole {
		var ok bool
		if r, ok = t.Sheet.UsedRange(); !ok {
			r = sheet.Rect{}
		}
	}
	r.To.Row = min(r.To.Row, r.From.Row+max(rows, 1)-1, r.From.Row+maxViewRows-1)
	snap := fileio.Snap(t.Sheet, r, t.Sheet.Name())
	grid, _ := fileio.HTMLGrid(snap)
	return &View{Title: b.Name(), Where: sheet.Qualified(t.Sheet.Name(), snap.Range), HTML: grid}
}

// chartView draws the chart create_chart made in b.
func chartView(b *book, w *sheet.Workbook, made headless.ChartMade) *View {
	c, ok := fileio.ChartSnap(w.Lookup(made.Sheet), made.Number-1)
	if !ok {
		return nil
	}
	data := sheet.Qualified(made.Sheet, c.Data)
	return &View{Title: b.Name(), Name: c.Data.String(), Where: c.Type.Title() + " chart of " + data, HTML: fileio.HTMLChart(c)}
}

// viewCSS fits the export's page to a frame: the panel on top, the
// grid scrolling in what room the host gives, its headers sticking to
// the grid's edges.
const viewCSS = `html,body{overflow:hidden}
.o12{width:auto;min-width:0}
#o12-view{overflow-x:auto}
#o12-view .sheet{overflow:auto;max-height:480px;overscroll-behavior:contain}
#o12-view .grid thead th{top:0}
#o12-view:empty::before{content:"Waiting for the tool's result";display:block;color:var(--muted);padding:0 9px;height:21px}
`
