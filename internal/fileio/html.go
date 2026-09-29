package fileio

import (
	"context"
	"fmt"
	"html"
	"io"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// HTML exports are static pages in 012's look (see htmlstyle.go): the
// control panel's lines above the grid, the grid with its headers and
// charts, and a small script that puts the pointer on a cell clicked or
// moved to with the arrow keys, showing its address in the name box and
// what it holds in the formula bar, as the screen does. The page needs
// nothing beyond itself: fonts, styles, SVG charts and script are in
// the file.

// HTMLPage is what an HTML page shows: a sheet or range, or one chart.
type HTMLPage struct {
	Title string    // the page's title, and the first line's
	Snap  *Snapshot // the cells, when Chart is nil
	Chart *SnapChart
}

// WriteHTML writes p as a page. The note, when there is one, says what
// the page left out.
func WriteHTML(w io.Writer, p HTMLPage) (note string, err error) {
	body, name, where := "", "", ""
	switch {
	case p.Chart != nil:
		body, name, where = HTMLChart(*p.Chart), p.Chart.Data.String(), p.Chart.Type.Title()+" chart of "+p.Chart.Data.String()
	case p.Snap != nil:
		body, note = HTMLGrid(p.Snap)
		where = p.Snap.Name + "!" + p.Snap.Range.String()
	}
	_, err = io.WriteString(w, HTMLDocument(p.Title, HTMLPanel(p.Title, name, where)+body, ""))
	return note, err
}

// HTMLDocument is a whole page: title, style sheet, body and script,
// with extra added to the script (the MCP view's messages).
func HTMLDocument(title, body, extra string) string {
	return "<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">" +
		"<meta name=\"generator\" content=\"012\"><title>" + html.EscapeString(title) + "</title>\n<style>\n" +
		HTMLStyle() + "</style></head>\n<body><div class=\"o12\">" + body +
		"</div>\n<script>\n" + htmlScript + extra + "</script></body></html>\n"
}

// HTMLPanel is the control panel's three lines: the title with the
// mode indicator, the name box (holding name until a cell is pointed
// at) and formula bar, and the context line saying what the page
// shows.
func HTMLPanel(title, name, where string) string {
	return fmt.Sprintf(`<header class="panel"><div><span class="book">%s</span><span class="mode">READY</span></div>`+
		`<div><span class="namebox" id="o12-name">%s</span><span class="input" id="o12-input"></span></div>`+
		`<div class="context" id="o12-context">%s</div></header>`, html.EscapeString(title), html.EscapeString(name), html.EscapeString(where))
}

// exportHTML writes a sheet or range as a page.
func exportHTML(_ context.Context, name string, snap *Snapshot, _ ExportOptions) (*ExportResult, error) {
	var note string
	err := writeFile(name, func(w io.Writer) error {
		var err error
		note, err = WriteHTML(w, HTMLPage{Title: snap.Name, Snap: snap})
		return err
	})
	if err != nil {
		return nil, err
	}
	res := &ExportResult{Rows: snap.Range.To.Row - snap.Range.From.Row + 1}
	if len(snap.Cells) == 0 {
		res.Rows = 0
	}
	if note != "" {
		res.Notes = append(res.Notes, note)
	}
	return res, nil
}

// ChartSnap is chart i of s with the values it draws, for a page of
// its own.
func ChartSnap(s *sheet.Sheet, i int) (SnapChart, bool) {
	charts := s.Charts()
	if i < 0 || i >= len(charts) {
		return SnapChart{}, false
	}
	return SnapChart{Chart: charts[i], Values: s.ChartData(charts[i])}, true
}

// htmlScript moves the pointer: a click or an arrow key puts it on a
// cell, whose address goes in the name box, what was typed in it in
// the formula bar, and whose row and column headers light up. It works
// on grids added to the page later, as the MCP view adds them.
const htmlScript = `(function(){
var cur=null;
function col(s){var n=0;for(var i=0;i<s.length;i++)n=n*26+s.charCodeAt(i)-64;return n}
function name(n){var s="";while(n>0){n--;s=String.fromCharCode(65+n%26)+s;n=Math.floor(n/26)}return s}
function split(a){var m=/^([A-Z]+)(\d+)$/.exec(a);return m?[col(m[1]),+m[2]]:null}
function point(td){
  if(cur){cur.classList.remove("ptr")}
  document.querySelectorAll("th.on").forEach(function(t){t.classList.remove("on")});
  cur=td;if(!td)return;
  td.classList.add("ptr");
  var a=td.getAttribute("data-a"),p=split(a),g=td.closest("table");
  var nb=document.getElementById("o12-name"),inp=document.getElementById("o12-input");
  if(nb)nb.textContent=a;
  if(inp)inp.textContent=td.hasAttribute("data-f")?td.getAttribute("data-f"):td.textContent;
  if(p&&g){var c=g.querySelector('th[data-c="'+name(p[0])+'"]'),r=g.querySelector('th[data-r="'+p[1]+'"]');
    if(c)c.classList.add("on");if(r)r.classList.add("on")}
}
document.addEventListener("click",function(e){var td=e.target.closest&&e.target.closest("td[data-a]");if(td)point(td)});
document.addEventListener("keydown",function(e){
  if(!cur)return;var d={ArrowLeft:[-1,0],ArrowRight:[1,0],ArrowUp:[0,-1],ArrowDown:[0,1]}[e.key];if(!d)return;
  var p=split(cur.getAttribute("data-a")),g=cur.closest("table");if(!p||!g)return;
  for(var k=1;k<64;k++){var n=g.querySelector('td[data-a="'+name(p[0]+d[0]*k)+(p[1]+d[1]*k)+'"]');
    if(n){point(n);n.scrollIntoView({block:"nearest",inline:"nearest"});e.preventDefault();return}
    if(p[0]+d[0]*k<1||p[1]+d[1]*k<1)return}
});
window.o12point=point;
var first=document.querySelector("td[data-a]");if(first)point(first);
})();
`
