package sheet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Rules in the file are two optional fields after a sheet's cells and
// charts, one rule per line, which older builds ignore (losing the
// rules, as they lose charts), so they need no version:
//
//	"conditionalFormats": [
//	  {"ranges": "B2:B20", "condition": "gt", "values": ["100"], "fill": "green", "bold": true},
//	  {"ranges": "C2:C20", "scale": [{"type": "min", "color": "red"}, {"type": "max", "color": "green"}]}
//	],
//	"validations": [
//	  {"ranges": "D2:D20", "criteria": "list", "items": ["Yes", "No"], "reject": true}
//	]
//
// The same lines are the answers of the commands that add rules, so a
// recorded macro adds a rule as the file would.

type fileCondFormat struct {
	Ranges    string       `json:"ranges"`
	Condition string       `json:"condition,omitempty"`
	Values    []string     `json:"values,omitempty"`
	Text      string       `json:"text,omitempty"`
	Fill      string       `json:"fill,omitempty"`
	Bold      bool         `json:"bold,omitempty"`
	Italic    bool         `json:"italic,omitempty"`
	Underline bool         `json:"underline,omitempty"`
	Strike    bool         `json:"strikethrough,omitempty"`
	Scale     []fileScaled `json:"scale,omitempty"`
}

type fileScaled struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
	Color string `json:"color"`
}

type fileValidation struct {
	Ranges    string   `json:"ranges"`
	Criteria  string   `json:"criteria"`
	Condition string   `json:"condition,omitempty"`
	Values    []string `json:"values,omitempty"`
	Items     []string `json:"items,omitempty"`
	Source    string   `json:"source,omitempty"`
	Reject    bool     `json:"reject,omitempty"`
	Help      string   `json:"help,omitempty"`
}

// values lists the first n of args, as files keep them.
func values(args [2]string, n int) []string {
	if n == 0 {
		return nil
	}
	return args[:n:n]
}

// JSON writes the rule as a line of the file, e.g.
// {"ranges":"B2:B20","condition":"gt","values":["100"],"fill":"green"}.
func (f CondFormat) JSON() string {
	fc := fileCondFormat{Ranges: rangesText(f.Ranges)}
	if f.IsScale() {
		for _, p := range f.Scale {
			sp := fileScaled{Type: p.Kind.String(), Color: p.Color.String()}
			if p.Kind.TakesValue() {
				sp.Value = p.Value
			}
			fc.Scale = append(fc.Scale, sp)
		}
	} else {
		st := f.Style
		fc.Condition, fc.Values = f.Op.String(), values(f.Args, f.Op.Args())
		fc.Text, fc.Fill = st.Text.String(), st.Fill.String()
		fc.Bold, fc.Italic, fc.Underline, fc.Strike = st.Bold, st.Italic, st.Underline, st.Strikethrough
	}
	raw, _ := json.Marshal(fc)
	return string(raw)
}

// ParseCondFormat reads a rule written by JSON, checking it.
func ParseCondFormat(line string) (CondFormat, error) {
	var fc fileCondFormat
	if err := json.Unmarshal([]byte(line), &fc); err != nil {
		return CondFormat{}, err
	}
	f, err := fc.rule()
	if err != nil {
		return CondFormat{}, err
	}
	return f, f.Check()
}

func (fc fileCondFormat) rule() (CondFormat, error) {
	rs, ok := ParseRanges(fc.Ranges)
	if !ok {
		return CondFormat{}, fmt.Errorf("invalid ranges %q", fc.Ranges)
	}
	f := CondFormat{Ranges: rs}
	for _, sp := range fc.Scale {
		k, ok := ParsePointKind(sp.Type)
		c, cok := ParseColor(sp.Color)
		if !ok || !cok {
			return CondFormat{}, fmt.Errorf("invalid color scale point %q %q", sp.Type, sp.Color)
		}
		f.Scale = append(f.Scale, ScalePoint{Kind: k, Value: sp.Value, Color: c})
	}
	if f.IsScale() {
		return f, nil
	}
	op, ok := ParseRuleOp(fc.Condition)
	text, tok := ParseColor(fc.Text)
	fill, fok := ParseColor(fc.Fill)
	if !ok || !tok || !fok || len(fc.Values) > 2 {
		return CondFormat{}, fmt.Errorf("invalid rule %q", fc.Condition)
	}
	f.Op = op
	copy(f.Args[:], fc.Values)
	f.Style = RuleStyle{Text: text, Fill: fill, Bold: fc.Bold, Italic: fc.Italic, Underline: fc.Underline, Strikethrough: fc.Strike}
	return f, nil
}

// JSON writes the rule as a line of the file, e.g.
// {"ranges":"D2:D20","criteria":"list","items":["Yes","No"]}.
func (v Validation) JSON() string {
	fv := fileValidation{Ranges: rangesText(v.Ranges), Criteria: v.Kind.String(), Reject: v.Reject, Help: v.Help}
	switch {
	case v.Kind == ValidList:
		fv.Items = v.Items
	case v.Kind == ValidRange:
		fv.Source = v.Source
	case v.Kind == ValidFormula:
		fv.Values = values(v.Args, 1)
	case v.Kind.Compares():
		fv.Condition, fv.Values = v.Op.String(), values(v.Args, v.Op.Args())
	}
	raw, _ := json.Marshal(fv)
	return string(raw)
}

// ParseValidation reads a rule written by JSON, checking it.
func ParseValidation(line string) (Validation, error) {
	var fv fileValidation
	if err := json.Unmarshal([]byte(line), &fv); err != nil {
		return Validation{}, err
	}
	v, err := fv.rule()
	if err != nil {
		return Validation{}, err
	}
	return v, v.Check()
}

func (fv fileValidation) rule() (Validation, error) {
	rs, ok := ParseRanges(fv.Ranges)
	if !ok {
		return Validation{}, fmt.Errorf("invalid ranges %q", fv.Ranges)
	}
	k, ok := ParseValidKind(fv.Criteria)
	op, opOK := ParseRuleOp(fv.Condition)
	if !ok || !opOK || len(fv.Values) > 2 {
		return Validation{}, fmt.Errorf("invalid validation %q %q", fv.Criteria, fv.Condition)
	}
	v := Validation{Ranges: rs, Kind: k, Op: op, Items: fv.Items, Source: fv.Source, Reject: fv.Reject, Help: fv.Help}
	copy(v.Args[:], fv.Values)
	return v, nil
}

// writeRules writes the sheet's rules, one per line, after its other
// fields.
func (s *Sheet) writeRules(b *bytes.Buffer, indent string) {
	list := func(key string, lines []string) {
		if len(lines) == 0 {
			return
		}
		fmt.Fprintf(b, ",\n%s%q: [", indent, key)
		for i, l := range lines {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(b, "\n%s  %s", indent, l)
		}
		b.WriteString("\n" + indent + "]")
	}
	var fs, vs []string
	for _, f := range s.rules.formats {
		fs = append(fs, f.JSON())
	}
	for _, v := range s.rules.validations {
		vs = append(vs, v.JSON())
	}
	list("conditionalFormats", fs)
	list("validations", vs)
}

// readRules restores the sheet's rules from a file. A rule that doesn't
// make sense (a later build's criteria, say) is an error, as a bad cell
// is.
func (s *Sheet) readRules(fcs []fileCondFormat, fvs []fileValidation) error {
	for i, fc := range fcs {
		f, err := fc.rule()
		if err == nil {
			err = f.Check()
		}
		if err != nil {
			return fmt.Errorf("conditional format %d: %s", i+1, strings.TrimSuffix(err.Error(), "."))
		}
		s.rules.formats = append(s.rules.formats, f)
	}
	for i, fv := range fvs {
		v, err := fv.rule()
		if err == nil {
			err = v.Check()
		}
		if err != nil {
			return fmt.Errorf("data validation %d: %s", i+1, strings.TrimSuffix(err.Error(), "."))
		}
		s.rules.validations = append(s.rules.validations, v)
	}
	s.looks.reset()
	return nil
}
