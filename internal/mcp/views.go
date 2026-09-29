package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Views are the MCP Apps extension (io.modelcontextprotocol/ui, spec
// 2026-01-26): read_range and create_chart name one UI resource,
// ui://012/view, which hosts that support the extension draw in a
// sandboxed frame beside the result. The resource is the HTML export's
// page with nothing in it yet (its fonts, palettes and pointer script);
// each result carries the fragment to show in its _meta, under viewKey,
// only for clients that declared the extension. The page asks the host
// for its theme, shows the fragment when the result arrives, and says
// how tall it is.
const (
	uiExtension = "io.modelcontextprotocol/ui"
	uiMIME      = "text/html;profile=mcp-app"
	viewURI     = "ui://012/view"
	viewKey     = "o12/view"
	// maxViewRows bounds the rows a range's view draws.
	maxViewRows = 500
)

func (s *Server) addViews() {
	meta := sdk.Meta{"ui": map[string]any{"prefersBorder": true, "csp": map[string]any{}}}
	s.AddResource(&sdk.Resource{URI: viewURI, Name: "012 view", Title: s.b.Name() + " in 012", MIMEType: uiMIME, Meta: meta,
		Description: "The range or chart a tool returned, drawn as 012 draws it"},
		func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			page := fileio.HTMLDocument(s.b.Name(), fileio.HTMLPanel(s.b.Name(), "", "")+`<div id="o12-view"></div>`, viewScript)
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: viewURI, MIMEType: uiMIME, Text: page, Meta: meta}}}, nil
		})
}

// viewMeta is what a tool drawn in the view says of it: the resource,
// under the key the spec names and the one its first drafts named.
func (s *Server) viewMeta() sdk.Meta {
	return sdk.Meta{"ui": map[string]any{"resourceUri": viewURI}, "ui/resourceUri": viewURI}
}

// wantsViews reports whether the client declared the extension.
func wantsViews(req *sdk.CallToolRequest) bool {
	if req == nil || req.Session == nil {
		return false
	}
	p := req.Session.InitializeParams()
	if p == nil || p.Capabilities == nil {
		return false
	}
	_, ok := p.Capabilities.Extensions[uiExtension]
	return ok
}

// viewResult is a result carrying a fragment of the view.
func viewResult(title, where, html string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Meta: sdk.Meta{viewKey: map[string]any{"title": title, "where": where, "html": html}}}
}

// rangeView draws what read_range read, at most maxViewRows rows.
func (s *Server) rangeView(req *sdk.CallToolRequest, t headless.Target) *sdk.CallToolResult {
	if !wantsViews(req) {
		return nil
	}
	r := t.Range
	if t.Whole {
		r, _ = t.Sheet.UsedRange()
	}
	r.To.Row = min(r.To.Row, r.From.Row+maxViewRows-1)
	snap := fileio.Snap(t.Sheet, r, t.Sheet.Name())
	grid, _ := fileio.HTMLGrid(snap)
	return viewResult(s.b.Name(), sheet.Qualified(t.Sheet.Name(), snap.Range), grid)
}

// chartView draws the chart create_chart made.
func (s *Server) chartView(req *sdk.CallToolRequest, w *sheet.Workbook, made headless.ChartMade) *sdk.CallToolResult {
	if !wantsViews(req) {
		return nil
	}
	c, ok := fileio.ChartSnap(w.Lookup(made.Sheet), made.Number-1)
	if !ok {
		return nil
	}
	return viewResult(made.Title, made.Sheet+" "+c.Data.String(), fileio.HTMLChart(c))
}

// viewScript is the view's side of the extension's messages, over
// postMessage with the host: ui/initialize, then the host's theme, the
// tool's result and teardown, and the page's height as it changes.
const viewScript = `(function(){
var next=1,host=window.parent;
function send(m){host.postMessage(Object.assign({jsonrpc:"2.0"},m),"*")}
function theme(t){var r=document.documentElement;r.classList.remove("dark","light");if(t==="dark"||t==="light")r.classList.add(t)}
function size(){send({method:"ui/notifications/size-changed",params:{width:document.documentElement.scrollWidth,height:document.documentElement.scrollHeight}})}
function show(res){
  var v=res&&res._meta&&res._meta["o12/view"];if(!v)return;
  document.getElementById("o12-view").innerHTML=v.html;
  var c=document.getElementById("o12-context");if(c)c.textContent=v.where;
  var first=document.querySelector("#o12-view td[data-a]");if(first&&window.o12point)window.o12point(first);
  size();
}
window.addEventListener("message",function(e){
  var m=e.data;if(!m||m.jsonrpc!=="2.0")return;
  if(m.id===1&&m.result){var hc=m.result.hostContext||{};theme(hc.theme);send({method:"ui/notifications/initialized",params:{}});return}
  switch(m.method){
  case "ui/notifications/tool-result":show(m.params);break;
  case "ui/notifications/host-context-changed":if(m.params)theme(m.params.theme);break;
  case "ui/resource-teardown":send({id:m.id,result:{}});break;
  }
});
send({id:next++,method:"ui/initialize",params:{protocolVersion:"2026-01-26",capabilities:{},clientInfo:{name:"012",version:"1"},appCapabilities:{availableDisplayModes:["inline","fullscreen"]}}});
new ResizeObserver(size).observe(document.body);
})();
`
