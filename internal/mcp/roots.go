// The roots capability is deprecated (SEP-2577), but every protocol
// version keeps it for a year after 2026-07-28 and hosts in use share
// their project folders through it (Claude Code among them), so the
// server follows a client's roots where the client offers them, and
// Options.Roots otherwise. Clients of 2026-07-28 and later can't be
// asked in the middle of a call, so they get Options.Roots.
//
//lint:file-ignore SA1019 roots are deprecated, and followed for the hosts that still share them

package mcp

import (
	"context"
	"net/url"
	"path/filepath"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/confine"
)

// roots are the folders workbooks may be in for the client of ss: its
// roots when it shares any, else the server's.
func (s *Server) roots(ctx context.Context, ss *sdk.ServerSession) []confine.Root {
	if ss == nil {
		return s.fallback
	}
	s.mu.Lock()
	cached, ok := s.clientRoots[ss]
	s.mu.Unlock()
	if ok {
		return cached
	}
	var roots []confine.Root
	if p := ss.InitializeParams(); p != nil && p.Capabilities != nil && p.Capabilities.RootsV2 != nil {
		if res, err := ss.ListRoots(ctx, nil); err == nil {
			roots = rootsOf(res.Roots)
		}
	}
	if len(roots) == 0 {
		roots = s.fallback
	}
	s.mu.Lock()
	s.clientRoots[ss] = roots
	s.mu.Unlock()
	return roots
}

// forgetRoots drops the roots kept for a client, which said they
// changed.
func (s *Server) forgetRoots(_ context.Context, req *sdk.RootsListChangedRequest) {
	s.mu.Lock()
	delete(s.clientRoots, req.Session)
	s.mu.Unlock()
}

// serverOptions are the SDK's options for the server: its instructions,
// its MCP Apps extension, and forgetRoots.
func (s *Server) serverOptions() *sdk.ServerOptions {
	return &sdk.ServerOptions{Instructions: instructions, RootsListChangedHandler: s.forgetRoots,
		Capabilities: &sdk.ServerCapabilities{Extensions: map[string]any{uiExtension: map[string]any{}}}}
}

// rootsOf are the folders of a client's file:// roots that exist.
func rootsOf(list []*sdk.Root) []confine.Root {
	var out []confine.Root
	for _, r := range list {
		u, err := url.Parse(r.URI)
		if err != nil || u.Scheme != "file" {
			continue
		}
		if root, err := confine.New(filepath.FromSlash(u.Path)); err == nil {
			out = append(out, root)
		}
	}
	return out
}
