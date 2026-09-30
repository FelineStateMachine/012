package cowork

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Questions: an agent asks the person something with the ask tool,
// shaped as an MCP elicitation request (a message, and a flat object
// schema of what to answer), and the person answers on the context line
// of their session, one field at a time. Elicitation goes from a server
// to its host's user; in live mode the person is on the server's side,
// so the agent asks through a tool, and the answer comes back as an
// elicitation's result: accept with the values, decline or cancel.
// Grants (running notebook cells, JEV) are asked the same way.

// AskKind is what a question is for.
type AskKind int

const (
	// Question is the agent's own question (the ask tool).
	Question AskKind = iota
	// Grant asks leave to run notebook cells or enter JEV formulas.
	Grant
)

// FieldKind is the type of what a field asks for.
type FieldKind int

const (
	Text FieldKind = iota
	Number
	Integer
	Boolean
	Choice
)

// Field is one thing a question asks for.
type Field struct {
	Name, Title, Description string
	Kind                     FieldKind
	Choices                  []string // a Choice's values
	Labels                   []string // and how they're shown, when the schema names them
	Required                 bool
}

// Label is how the field is named to the person.
func (f Field) Label() string {
	if f.Title != "" {
		return f.Title
	}
	return f.Name
}

// Ask is a question waiting for a person's answer.
type Ask struct {
	ID      int
	Author  int // the agent's seat
	Agent   string
	Color   int
	Kind    AskKind
	Message string
	Fields  []Field
	// Grant is what a Grant asks for: "notebooks" or "jev".
	Grant string

	reply chan Answer
	taken int // the seat showing it, 0 for none yet
	done  bool
}

// Answer is the person's answer: the elicitation's action (accept,
// decline, cancel) and values by field, and for a grant whether it
// holds for the rest of the session.
type Answer struct {
	Action  string
	Content map[string]any
	Always  bool
}

// NewAsk makes a question, its fields read from an elicitation's
// requested schema (nil for a yes or no question).
func NewAsk(message string, schema any) (*Ask, error) {
	if strings.TrimSpace(message) == "" {
		return nil, errors.New("ask needs a message: the question shown to the person")
	}
	fields, err := Fields(schema)
	if err != nil {
		return nil, err
	}
	return &Ask{Kind: Question, Message: message, Fields: fields, reply: make(chan Answer, 1)}, nil
}

// NewGrant asks leave for what ("notebooks" or "jev").
func NewGrant(what, message string) *Ask {
	return &Ask{Kind: Grant, Grant: what, Message: message, reply: make(chan Answer, 1)}
}

// Reply is where the answer arrives, once.
func (a *Ask) Reply() <-chan Answer { return a.reply }

// Done reports whether the question was answered or withdrawn.
func (a *Ask) Done() bool { return a.done }

// schemaProp is a property of an elicitation's schema, as far as
// questions read it.
type schemaProp struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Enum        []string `json:"enum"`
	EnumNames   []string `json:"enumNames"`
	OneOf       []struct {
		Const string `json:"const"`
		Title string `json:"title"`
	} `json:"oneOf"`
}

// Fields reads an elicitation's requested schema: an object of
// top-level string, number, integer and boolean properties, strings with
// enum (or oneOf consts) being a choice, as MCP allows.
func Fields(schema any) ([]Field, error) {
	if schema == nil {
		return nil, nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var s struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("requestedSchema: %w", err)
	}
	if s.Type != "" && s.Type != "object" {
		return nil, errors.New("requestedSchema must be an object schema")
	}
	names := make([]string, 0, len(s.Properties))
	for n := range s.Properties {
		names = append(names, n)
	}
	slices.Sort(names) // JSON objects are unordered; required ones first below
	slices.SortStableFunc(names, func(a, b string) int {
		return boolRank(slices.Contains(s.Required, b)) - boolRank(slices.Contains(s.Required, a))
	})
	var out []Field
	for _, n := range names {
		f, err := field(n, s.Properties[n])
		if err != nil {
			return nil, err
		}
		f.Required = slices.Contains(s.Required, n)
		out = append(out, f)
	}
	return out, nil
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// field reads one property.
func field(name string, raw json.RawMessage) (Field, error) {
	var p schemaProp
	if err := json.Unmarshal(raw, &p); err != nil {
		return Field{}, fmt.Errorf("requestedSchema property %s: %w", name, err)
	}
	f := Field{Name: name, Title: p.Title, Description: p.Description}
	switch {
	case p.Type == "string" && len(p.Enum) > 0:
		f.Kind, f.Choices, f.Labels = Choice, p.Enum, p.EnumNames
	case p.Type == "string" && len(p.OneOf) > 0:
		f.Kind = Choice
		for _, o := range p.OneOf {
			f.Choices, f.Labels = append(f.Choices, o.Const), append(f.Labels, o.Title)
		}
	case p.Type == "string":
		f.Kind = Text
	case p.Type == "number":
		f.Kind = Number
	case p.Type == "integer":
		f.Kind = Integer
	case p.Type == "boolean":
		f.Kind = Boolean
	default:
		return Field{}, fmt.Errorf("requestedSchema property %s is a %q: questions take strings, numbers, integers, booleans and choices", name, p.Type)
	}
	if len(f.Choices) > 9 {
		return Field{}, fmt.Errorf("requestedSchema property %s has %d choices: at most 9, each a key", name, len(f.Choices))
	}
	return f, nil
}

// Parse reads what the person typed for a Text, Number or Integer
// field.
func (f Field) Parse(text string) (any, error) {
	switch f.Kind {
	case Number:
		n, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return nil, fmt.Errorf("%s takes a number", f.Label())
		}
		return n, nil
	case Integer:
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil {
			return nil, fmt.Errorf("%s takes a whole number", f.Label())
		}
		return n, nil
	}
	return text, nil
}

// ChoiceLabel is how choice i is shown.
func (f Field) ChoiceLabel(i int) string {
	if i < len(f.Labels) && f.Labels[i] != "" {
		return f.Labels[i]
	}
	return f.Choices[i]
}

// AddAsk puts a question on the board for a person to answer.
func (b *Board) AddAsk(a *Ask) {
	b.nextAsk++
	a.ID = b.nextAsk
	b.asks = append(b.asks, a)
}

// NextAsk is the first question waiting that the person at seat should
// answer: the host's, or in a room without one, anyone's not already
// shown to another who is here (present). It is then seat's to show.
func (b *Board) NextAsk(seat int, present func(id int) bool) *Ask {
	if b.Host != 0 && b.Host != seat && present(b.Host) {
		return nil
	}
	for _, a := range b.asks {
		if a.taken == seat || a.taken == 0 || !present(a.taken) {
			a.taken = seat
			return a
		}
	}
	return nil
}

// Answer answers a question and takes it off the board.
func (b *Board) Answer(a *Ask, ans Answer) {
	if a.done {
		return
	}
	a.done = true
	b.asks = slices.DeleteFunc(b.asks, func(x *Ask) bool { return x == a })
	if a.Kind == Grant && ans.Action == "accept" && ans.Always {
		switch a.Grant {
		case "notebooks":
			b.Grants.Notebooks = true
		case "jev":
			b.Grants.JEV = true
		}
	}
	a.reply <- ans
}

// Withdraw takes a question off the board unanswered: the agent gave
// up waiting, or left.
func (b *Board) Withdraw(a *Ask) {
	if a.done {
		return
	}
	a.done = true
	b.asks = slices.DeleteFunc(b.asks, func(x *Ask) bool { return x == a })
}

// Asks are the questions waiting, oldest first.
func (b *Board) Asks() []*Ask { return slices.Clone(b.asks) }
