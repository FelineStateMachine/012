package mcp

import (
	"context"
	"encoding/json"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/diff"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Live mode (docs/agents/live.md): the server runs inside a 012 session
// with its workbook on someone's screen, and the agent is one of the
// participants of the session's room. Every tool works on that one
// workbook, whose Backend is a Live: reads see it as it is on the
// screen, and a change is the agent's, a suggestion the person accepts
// or rejects unless they let agents edit directly. Live mode adds the
// tools only a live workbook has: focus, undo, ask and suggestions.

// Live is a workbook open in a session, as the agent attached to it
// works on it.
type Live interface {
	Backend
	// Make makes a change as the agent's, or suggests it.
	Make(ctx context.Context, m Making) (Made, error)
	// Focus moves the agent's pointer to ref, for the person to see
	// where it's looking, and says where that is.
	Focus(ctx context.Context, ref string) (string, error)
	// Undo takes back the agent's latest step, saying what it was.
	Undo(ctx context.Context) (string, error)
	// Ask asks the person a question, as an elicitation request asks a
	// host's user, and returns their answer.
	Ask(ctx context.Context, p *sdk.ElicitParams) (*sdk.ElicitResult, error)
	// Suggestions are the agent's suggestions and what became of them.
	Suggestions(ctx context.Context) ([]Suggestion, error)
	// RunCell has the person's session run a notebook cell, once they
	// allow it, and says what happens next.
	RunCell(ctx context.Context, notebook string, cell int) (string, error)
	// Scope is what the person let the agent change, in words.
	Scope() string
}

// Making is a change a tool asks for: its label, the agent's message
// shown with it, whether only to see it, and the change itself.
type Making struct {
	Label, Message string
	DryRun         bool
	Fn             func(*sheet.Workbook) error
}

// Made is what came of a change: what it changes, whether it was made
// on the workbook or waits as a suggestion, and news of the agent's
// earlier suggestions.
type Made struct {
	Changes    []diff.Change
	Applied    bool
	Suggestion int
	Notices    []string
}

// Suggestion is one of the agent's suggestions as the suggestions tool
// lists it.
type Suggestion struct {
	ID       int    `json:"id"`
	Label    string `json:"label"`
	Message  string `json:"message,omitempty"`
	State    string `json:"state" jsonschema:"pending, accepted, accepted in part or rejected"`
	Cells    int    `json:"cells" jsonschema:"the cells it sets"`
	Accepted int    `json:"accepted" jsonschema:"the cells the person accepted"`
	Rejected int    `json:"rejected" jsonschema:"the cells the person rejected"`
}

const liveInstructions = `This server is attached to a workbook open on a person's screen in 012 (live mode): every tool works on that one workbook, so leave path out. Reads see it as it is now, the person's edits included.
Your changes are suggestions unless the person lets agents edit directly: the cells you'd change are marked on their screen, and they accept or reject them, whole or cell by cell. A write returns the suggestion's number; the suggestions tool says what became of each, and news of them also comes back with your next write. Pass message with a write to tell the person why.
Call focus to move your pointer where you're working, so the person sees where you're looking. undo takes back your own latest change, never the person's. ask asks the person a question and waits for the answer, as elicitation does: use it before anything they might not expect.
You may change only what the person let you (the scope); a change outside it is refused with the reason.`

type focusIn struct {
	Ref string `json:"ref" jsonschema:"the cell or range to point at: B7, Q3!A1:C9, a named range or a table"`
}

type focusOut struct {
	At string `json:"at" jsonschema:"where the pointer is, with its sheet"`
}

type undoOut struct {
	Undone string `json:"undone" jsonschema:"what the step undone did"`
}

type askIn struct {
	Message         string         `json:"message" jsonschema:"the question, shown to the person"`
	RequestedSchema map[string]any `json:"requestedSchema,omitempty" jsonschema:"what to ask for, as an elicitation request's: an object schema of top-level string, number, integer and boolean properties, strings with enum for a choice; none for a yes or no question"`
}

type askOut struct {
	Action  string         `json:"action" jsonschema:"accept, decline or cancel, as an elicitation's answer"`
	Content map[string]any `json:"content,omitempty" jsonschema:"with accept, the answers by property"`
}

type suggestionsOut struct {
	Scope       string       `json:"scope" jsonschema:"what the person let you change"`
	Suggestions []Suggestion `json:"suggestions"`
}

type liveRunOut struct {
	Status string `json:"status"`
}

// SuggestionsURI is the resource listing the agent's suggestions, as
// the suggestions tool does; clients subscribed to it are told when the
// person settles one (SuggestionsChanged).
const SuggestionsURI = scheme + "live/suggestions"

// SuggestionsChanged tells clients subscribed to SuggestionsURI that
// the person settled a suggestion.
func (s *Server) SuggestionsChanged(ctx context.Context) {
	s.ResourceUpdated(ctx, &sdk.ResourceUpdatedNotificationParams{URI: SuggestionsURI})
}

// addLiveTools adds the tools of a live workbook, and its suggestions
// resource.
func (s *Server) addLiveTools(l Live) {
	s.AddResource(&sdk.Resource{URI: SuggestionsURI, Name: "suggestions", MIMEType: "application/json",
		Description: "Your suggestions and what became of each, as the suggestions tool lists them; subscribe to hear when the person settles one"},
		func(ctx context.Context, _ *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			list, err := l.Suggestions(ctx)
			if err != nil {
				return nil, err
			}
			data, _ := json.Marshal(suggestionsOut{Scope: l.Scope(), Suggestions: list})
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: SuggestionsURI, MIMEType: "application/json", Text: string(data)}}}, nil
		})
	tool(s, &sdk.Tool{Name: "focus", Annotations: &sdk.ToolAnnotations{IdempotentHint: true, OpenWorldHint: ptr(false)},
		Description: "Move your pointer to a cell or range, where the person sees it with your name, to show where you're working."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in focusIn) (*sdk.CallToolResult, focusOut, error) {
			at, err := l.Focus(ctx, in.Ref)
			return nil, focusOut{At: at}, err
		})
	tool(s, &sdk.Tool{Name: "undo", Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(false)},
		Description: "Take back your own latest change that was made (accepted, or made directly), as Edit > Undo does for the person: never theirs."},
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, undoOut, error) {
			label, err := l.Undo(ctx)
			return nil, undoOut{Undone: label}, err
		})
	tool(s, &sdk.Tool{Name: "ask", Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
		Description: "Ask the person a question on their screen and wait for the answer: yes or no, a choice, or values, as an elicitation request asks. Use it before a change they might not expect (\"Overwrite B2:B40?\")."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in askIn) (*sdk.CallToolResult, askOut, error) {
			res, err := l.Ask(ctx, &sdk.ElicitParams{Message: in.Message, RequestedSchema: in.RequestedSchema})
			if err != nil {
				return nil, askOut{}, err
			}
			return nil, askOut{Action: res.Action, Content: res.Content}, nil
		})
	tool(s, &sdk.Tool{Name: "suggestions", Annotations: readOnly,
		Description: "Your suggestions and what became of each: pending, accepted, accepted in part or rejected, with how many cells; and what the person let you change."},
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, suggestionsOut, error) {
			list, err := l.Suggestions(ctx)
			if list == nil {
				list = []Suggestion{}
			}
			return nil, suggestionsOut{Scope: l.Scope(), Suggestions: list}, err
		})
	tool(s, &sdk.Tool{Name: "run_notebook_cell", Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(true)},
		Description: "Have the person's session run a notebook tab's code cell with nushell, once they allow it: its output arrives in the workbook as when they run it, to read with describe or read_range."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in runCellIn) (*sdk.CallToolResult, liveRunOut, error) {
			status, err := l.RunCell(ctx, in.Notebook, in.Cell)
			return nil, liveRunOut{Status: status}, err
		})
}
