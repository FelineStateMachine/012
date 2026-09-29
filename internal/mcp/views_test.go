package mcp

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectApps is connect with a client that declares the MCP Apps
// extension.
func connectApps(t *testing.T, b Backend) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	ss, err := New(b, Options{}).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	caps := &sdk.ClientCapabilities{}
	caps.AddExtension(uiExtension, map[string]any{"mimeTypes": []string{uiMIME}})
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "host", Version: "1"}, &sdk.ClientOptions{Capabilities: caps}).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func viewHTML(res *sdk.CallToolResult) string {
	v, _ := res.Meta[viewKey].(map[string]any)
	html, _ := v["html"].(string)
	return html
}

func TestViews(t *testing.T) {
	path := bookFile(t)
	cs := connectApps(t, &FileBackend{Path: path})
	ctx := context.Background()
	for tl, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		ui, _ := tl.Meta["ui"].(map[string]any)
		drawn := tl.Name == "read_range" || tl.Name == "create_chart"
		if (ui["resourceUri"] == viewURI) != drawn {
			t.Errorf("%s: _meta %v", tl.Name, tl.Meta)
		}
	}
	res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: viewURI})
	if err != nil {
		t.Fatal(err)
	}
	page := res.Contents[0]
	if page.MIMEType != uiMIME || !strings.Contains(page.Text, `"ui/initialize"`) || !strings.Contains(page.Text, "@font-face") ||
		!strings.Contains(page.Text, `<div id="o12-view"></div>`) {
		t.Errorf("view resource: %s %.200s", page.MIMEType, page.Text)
	}
	got := call(t, cs, "read_range", map[string]any{"ref": "A1:C4"}, nil)
	if html := viewHTML(got); !strings.Contains(html, `<table class="grid">`) || !strings.Contains(html, `data-a="A2"`) {
		t.Errorf("read_range view: %.300s", html)
	}
	got = call(t, cs, "create_chart", map[string]any{"data": "A1:B4", "title": "Units"}, nil)
	if html := viewHTML(got); !strings.Contains(html, `<figure class="chart solo"`) || !strings.Contains(html, "<svg") {
		t.Errorf("create_chart view: %.300s", html)
	}
	// A client without the extension gets no view.
	plain := connect(t, &FileBackend{Path: path}, Options{})
	if got := call(t, plain, "read_range", map[string]any{"ref": "A1"}, nil); got.Meta[viewKey] != nil {
		t.Error("a view for a client without the extension")
	}
}
