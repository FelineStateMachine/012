package mcp

import (
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

func (s *Server) addViews() {}

func (s *Server) viewMeta() sdk.Meta { return nil }

func (s *Server) rangeView(*sdk.CallToolRequest, headless.Target) *sdk.CallToolResult { return nil }

func (s *Server) chartView(*sdk.CallToolRequest, *sheet.Workbook, headless.ChartMade) *sdk.CallToolResult {
	return nil
}
