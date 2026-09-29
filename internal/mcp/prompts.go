package mcp

import (
	"context"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Prompts are starting points a host offers people (as slash commands
// in Claude Code): each a message asking the model to do a common job
// with the tools, the way the tools are best used.
var prompts = []struct {
	prompt *sdk.Prompt
	text   string // with {arg} for each argument
}{{
	prompt: &sdk.Prompt{Name: "summarize_workbook", Title: "Summarize the workbook",
		Description: "What each sheet holds and what its numbers say"},
	text: "Call describe, then read the main ranges it points at with read_range (a few hundred rows at most). " +
		"Summarize what each sheet holds, how the sheets relate (formulas between them, tables, pivots, charts), and what the numbers say. Don't change anything.",
}, {
	prompt: &sdk.Prompt{Name: "fix_errors", Title: "Fix formula errors",
		Description: "Find the formulas showing errors, explain them and fix them"},
	text: "Call list_errors. For each formula showing an error, read the cells it reads, and explain the cause. " +
		"Propose a fix, check it with evaluate at the same cell, and show it with write_cells and dry_run. Make the fixes once they're agreed.",
}, {
	prompt: &sdk.Prompt{Name: "add_column", Title: "Add a computed column",
		Description: "A new column of formulas beside a table of data",
		Arguments: []*sdk.PromptArgument{
			{Name: "range", Description: "the data, e.g. Sales!A1:F99, or a table's name", Required: true},
			{Name: "column", Description: "what the new column computes, e.g. profit as revenue minus cost", Required: true},
		}},
	text: "Add a column to {range} that computes {column}. Call describe and read_range to find the header row and the columns involved. " +
		"Write the header, then the formula for the first data row: check it with evaluate at that cell, then write every row's formula with write_cells (dry_run first), relative references moving with the row. " +
		"Follow the columns' number formats. Finish with list_errors.",
}, {
	prompt: &sdk.Prompt{Name: "chart", Title: "Chart some data",
		Description: "A chart that answers a question about a range",
		Arguments: []*sdk.PromptArgument{
			{Name: "range", Description: "the data, e.g. Sales!A1:C13", Required: true},
			{Name: "question", Description: "what the chart should show, e.g. how sales grew by month"},
		}},
	text: "Read {range} with read_range and pick the chart that best shows {question}: line or area for change over time, column or bar to compare, pie for shares of a whole, scatter for two measures. " +
		"If the data needs summarizing first, use create_pivot and chart its result. Create it with create_chart, with a title that says what it shows.",
}}

func (s *Server) addPrompts() {
	for _, p := range prompts {
		text := p.text
		s.AddPrompt(p.prompt, func(_ context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
			msg := text
			for k, v := range req.Params.Arguments {
				msg = strings.ReplaceAll(msg, "{"+k+"}", v)
			}
			msg = strings.ReplaceAll(msg, "{question}", "the data's main point")
			return &sdk.GetPromptResult{Description: req.Params.Name,
				Messages: []*sdk.PromptMessage{{Role: "user", Content: &sdk.TextContent{Text: msg}}}}, nil
		})
	}
}
