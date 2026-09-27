package sheet

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// JEV functions ask TypeSafe's hosted model (jev) typed questions about a
// value. They share one shape, like Sheets' own functions: the value to
// ask about (a cell, a range or text), the question in plain words, then
// what the answers mean. Each maps to a familiar spreadsheet or dataframe
// idea:
//
//	JEV.TEST     yes/no, like IF's condition or a boolean mask
//	JEV.PROB     the probability of yes, like predict_proba
//	JEV.CLASSIFY one label from a list, like SWITCH or pd.cut
//	JEV.SCORE    a score on an ordered rubric, like a rating scale

func init() {
	define(
		&FuncDef{Name: "JEV.TEST", Args: "value, question, [yes_means], [no_means]",
			Desc: "TRUE or FALSE, answered by the JEV model", Min: 2, Max: 4, Volatile: true,
			remote: yesNoCall, eval: remoteEval(yesNoCall, func(a RemoteAnswer) Value { return boolean(a.Noul >= 0.5) })},
		&FuncDef{Name: "JEV.PROB", Args: "value, question, [yes_means], [no_means]",
			Desc: "The probability the answer is yes, from the JEV model", Min: 2, Max: 4, Volatile: true,
			remote: yesNoCall, eval: remoteEval(yesNoCall, func(a RemoteAnswer) Value { return num(a.Noul) }),
			format: func([]Node, func(Node) Format) Format { return Format{Kind: FmtPercent} }},
		&FuncDef{Name: "JEV.CLASSIFY", Args: "value, question, labels, [descriptions]",
			Desc: "The label that fits best, chosen by the JEV model", Min: 3, Max: 4, Volatile: true,
			remote: choiceCall, eval: remoteEval(choiceCall, func(a RemoteAnswer) Value { return Value{Kind: Text, Str: a.Choice} })},
		&FuncDef{Name: "JEV.SCORE", Args: "value, question, levels",
			Desc: "A score on an ordered rubric, from the JEV model", Min: 3, Max: 3, Volatile: true,
			remote: scoreCall, eval: remoteEval(scoreCall, func(a RemoteAnswer) Value { return num(a.Score) })},
	)
}

// Limits the JEV service enforces (typesafe.MaxChoiceOptions and
// MaxScoreLevels; internal/jev tests keep them in step). Checking them here
// shows #VALUE! at once instead of a request that is bound to fail.
const (
	maxChoiceLabels = 255
	maxScoreLevels  = 10
)

// ErrRemote is a question the model couldn't answer, e.g. a network
// failure; the context line says why.
var ErrRemote = Value{Kind: Error, Str: "#ERROR!"}

// remoteEval evaluates a JEV function through the workbook's
// RemoteSource, converting the
// answer with result. Errors in the inputs come back as-is so they can be
// fixed; they're never sent.
func remoteEval(build func([]Node, lookup) (RemoteCall, error), result func(RemoteAnswer) Value) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		call, err := build(args, get)
		if err != nil {
			return err.(inputError).v
		}
		remote := get.w.remote
		if remote == nil {
			return ErrNoRemote
		}
		ans, ok := remote.Lookup(call)
		switch {
		case !ok:
			return Pending
		case ans.Failed != "":
			return ErrRemote
		}
		return result(ans)
	}
}

// inputError carries a spreadsheet error found in a JEV function's inputs.
type inputError struct{ v Value }

func (e inputError) Error() string { return e.v.Str }

// jevState turns the value argument into what the model sees: text,
// numbers and booleans as themselves, a range as rows of them.
func jevState(n Node, get lookup) (any, error) {
	if rn, ok := n.(formula.Range); ok {
		var rows [][]any
		for r := rn.Rect.From.Row; r <= rn.Rect.To.Row; r++ {
			var row []any
			for c := rn.Rect.From.Col; c <= rn.Rect.To.Col; c++ {
				v := get.cell(rn.Sheet, Addr{Col: c, Row: r})
				if v.Kind == Error {
					return nil, inputError{v}
				}
				row = append(row, plain(v))
			}
			rows = append(rows, row)
		}
		return rows, nil
	}
	v := eval(n, get)
	if v.Kind == Error {
		return nil, inputError{v}
	}
	return plain(v), nil
}

// plain converts a value to a JSON-friendly Go value.
func plain(v Value) any {
	switch v.Kind {
	case Number:
		return v.Num
	case Bool:
		return v.Num != 0
	case Text:
		return v.Str
	}
	return ""
}

// jevText evaluates an argument that must be text, such as the question.
func jevText(n Node, get lookup) (string, error) {
	v := eval(n, get)
	if v.Kind == Error {
		return "", inputError{v}
	}
	return strings.TrimSpace(text(v)), nil
}

// jevQuestion reads the question, which must not be empty.
func jevQuestion(n Node, get lookup) (string, error) {
	q, err := jevText(n, get)
	if err == nil && q == "" {
		err = inputError{ErrValue}
	}
	return q, err
}

// jevList reads a list argument: a range of cells, or one text of
// comma-separated items like "positive, negative, neutral". Blanks are
// skipped.
func jevList(n Node, get lookup) ([]string, error) {
	var out []string
	if rn, ok := n.(formula.Range); ok {
		for r := rn.Rect.From.Row; r <= rn.Rect.To.Row; r++ {
			for c := rn.Rect.From.Col; c <= rn.Rect.To.Col; c++ {
				v := get.cell(rn.Sheet, Addr{Col: c, Row: r})
				if v.Kind == Error {
					return nil, inputError{v}
				}
				if s := strings.TrimSpace(text(v)); s != "" {
					out = append(out, s)
				}
			}
		}
		return out, nil
	}
	s, err := jevText(n, get)
	if err != nil {
		return nil, err
	}
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out, nil
}

// yesNoCall is shared by JEV.TEST and JEV.PROB, so asking both about the
// same thing is one request.
func yesNoCall(args []Node, get lookup) (RemoteCall, error) {
	state, err := jevState(args[0], get)
	if err != nil {
		return RemoteCall{}, err
	}
	q, err := jevQuestion(args[1], get)
	if err != nil {
		return RemoteCall{}, err
	}
	var means [2]string
	for i := range 2 {
		if len(args) > 2+i {
			if means[i], err = jevText(args[2+i], get); err != nil {
				return RemoteCall{}, err
			}
		}
	}
	return RemoteCall{Kind: "noul", State: state, Instructions: q, Criteria: means}, nil
}

func choiceCall(args []Node, get lookup) (RemoteCall, error) {
	state, err := jevState(args[0], get)
	if err != nil {
		return RemoteCall{}, err
	}
	q, err := jevQuestion(args[1], get)
	if err != nil {
		return RemoteCall{}, err
	}
	labels, err := jevList(args[2], get)
	if err != nil {
		return RemoteCall{}, err
	}
	var descs []string
	if len(args) > 3 {
		if descs, err = jevList(args[3], get); err != nil {
			return RemoteCall{}, err
		}
	}
	if len(labels) < 2 || len(labels) > maxChoiceLabels {
		return RemoteCall{}, inputError{ErrValue} // nothing to choose between, or too many
	}
	criteria := make(map[string]string, len(labels))
	for i, l := range labels {
		criteria[l] = l
		if i < len(descs) {
			criteria[l] = descs[i]
		}
	}
	return RemoteCall{Kind: "choice", State: state, Instructions: q, Criteria: criteria}, nil
}

func scoreCall(args []Node, get lookup) (RemoteCall, error) {
	state, err := jevState(args[0], get)
	if err != nil {
		return RemoteCall{}, err
	}
	q, err := jevQuestion(args[1], get)
	if err != nil {
		return RemoteCall{}, err
	}
	levels, err := jevList(args[2], get)
	if err != nil {
		return RemoteCall{}, err
	}
	if len(levels) < 2 || len(levels) > maxScoreLevels {
		return RemoteCall{}, inputError{ErrValue}
	}
	return RemoteCall{Kind: "score", State: state, Instructions: q, Criteria: levels}, nil
}
