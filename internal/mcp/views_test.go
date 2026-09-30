package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// connectApps is connect with a client that declares the MCP Apps
// extension.
func connectApps(t *testing.T, o Options) *sdk.ClientSession {
	t.Helper()
	caps := &sdk.ClientCapabilities{}
	caps.AddExtension(uiExtension, map[string]any{"mimeTypes": []string{uiMIME}})
	return connectClient(t, o, sdk.NewClient(&sdk.Implementation{Name: "host", Version: "1"}, &sdk.ClientOptions{Capabilities: caps}), nil)
}

// resultView is the view in a result's _meta.
func resultView(t *testing.T, res *sdk.CallToolResult) View {
	t.Helper()
	var out View
	data, _ := json.Marshal(res.Meta[viewKey])
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// forModel is what of a result the model reads: its structured content
// and its text.
func forModel(t *testing.T, res *sdk.CallToolResult) string {
	t.Helper()
	data, _ := json.Marshal(res.StructuredContent)
	return string(data) + res.Content[0].(*sdk.TextContent).Text
}

func TestViewResources(t *testing.T) {
	cs := connectApps(t, Options{Default: bookFile(t)})
	ctx := context.Background()
	for tl, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		ui, _ := tl.Meta["ui"].(map[string]any)
		drawn := tl.Name == "read_range" || tl.Name == "create_chart"
		if (ui["resourceUri"] == viewURI) != drawn || (tl.Meta["openai/outputTemplate"] == skybridgeURI) != drawn {
			t.Errorf("%s: _meta %v", tl.Name, tl.Meta)
		}
	}
	for uri, mime := range map[string]string{viewURI: uiMIME, skybridgeURI: skybridgeMIME} {
		res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatal(err)
		}
		page := res.Contents[0]
		if page.MIMEType != mime || !strings.Contains(page.Text, `"ui/initialize"`) || !strings.Contains(page.Text, "openai:set_globals") ||
			!strings.Contains(page.Text, "@font-face") || !strings.Contains(page.Text, `<div id="o12-view"></div>`) {
			t.Errorf("%s: %s %.200s", uri, page.MIMEType, page.Text)
		}
		if ui, _ := page.Meta["ui"].(map[string]any); ui["prefersBorder"] != true || page.Meta["openai/widgetPrefersBorder"] != true {
			t.Errorf("%s: _meta %v", uri, page.Meta)
		}
	}
}

// TestViewsForEveryClient checks the view is in the result's _meta
// whether or not the client declared the extension (the Apps SDK's
// hosts don't), and left out of the structured content and text models
// read.
func TestViewsForEveryClient(t *testing.T) {
	path := bookFile(t)
	for name, cs := range map[string]*sdk.ClientSession{
		"apps":  connectApps(t, Options{Default: path}),
		"plain": connect(t, Options{Default: path}),
	} {
		got := call(t, cs, "read_range", map[string]any{"ref": "A1:C4"}, nil)
		v := resultView(t, got)
		if !strings.Contains(v.HTML, `<table class="grid">`) || !strings.Contains(v.HTML, `data-a="A2"`) || v.Where != "Sheet1!A1:C4" || v.Title != "book" {
			t.Errorf("%s: read_range view: %+v", name, v)
		}
		if text := forModel(t, got); strings.Contains(text, "html") || strings.Contains(text, "<table") || !strings.Contains(text, `"range":"Sheet1!A1:C4"`) {
			t.Errorf("%s: what the model reads: %s", name, text)
		}
		got = call(t, cs, "read_range", map[string]any{"ref": "A1:C4", "max_cells": 6}, nil)
		if v := resultView(t, got); v.Where != "Sheet1!A1:C2" {
			t.Errorf("%s: the view draws %s, not the rows read", name, v.Where)
		}
	}
	cs := connect(t, Options{Default: path})
	got := call(t, cs, "create_chart", map[string]any{"data": "A1:B4", "title": "Units"}, nil)
	if v := resultView(t, got); !strings.Contains(v.HTML, `<figure class="chart solo"`) || !strings.Contains(v.HTML, "<svg") || v.Title != "book" || v.Name != "A1:B4" || v.Where != "Column chart of Sheet1!A1:B4" {
		t.Errorf("create_chart view: %.300s", v.HTML)
	}
	if text := forModel(t, got); strings.Contains(text, "<svg") || !strings.Contains(text, `"saved":true`) {
		t.Errorf("create_chart, what the model reads: %s", text)
	}
}

// TestViewLeavesTheModelSmall checks what the model reads of a 60-row
// read, typed values and the text shown, stays about ten kilobytes
// while the view gets the grid.
func TestViewLeavesTheModelSmall(t *testing.T) {
	w := sheet.NewBook()
	s := w.Sheet(0)
	for r := range 60 {
		s.Set(sheet.Addr{Row: r}, fmt.Sprintf("Week %d", r+1))
		s.Set(sheet.Addr{Col: 1, Row: r}, fmt.Sprintf("$%d,%03d", 10+r%7, r*37%1000))
		s.Set(sheet.Addr{Col: 2, Row: r}, fmt.Sprintf("$%d,%03d", 6+r%3, r*53%1000))
		s.Set(sheet.Addr{Col: 3, Row: r}, fmt.Sprintf("=B%d-C%d", r+1, r+1))
	}
	path := filepath.Join(t.TempDir(), "weeks.012")
	writeBook(t, w, path)
	got := call(t, connect(t, Options{Default: path}), "read_range", map[string]any{"ref": "A1:D60"}, nil)
	structured, _ := json.Marshal(got.StructuredContent)
	if len(structured) > 12000 || strings.Contains(string(structured), "<") {
		t.Errorf("structuredContent of 60 rows: %d bytes", len(structured))
	}
	if v := resultView(t, got); strings.Count(v.HTML, "<tr") != 61 {
		t.Errorf("the view has %d rows", strings.Count(v.HTML, "<tr"))
	}
}
