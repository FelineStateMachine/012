package ui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Rules in macros: what the rules panel does is recorded as the commands
// that do it from a line of the file, and those commands are how scripts
// change rules. A rule is numbered from 1 in the panel's order.
//
//	run("format.conditional_add", answer={"ranges": "B2:B9", ...})   a rule added
//	run("format.conditional_set", answer={"rule": 2, "ranges": ...})  rule 2 replaced
//	run("format.conditional_remove", answer=2)                        rule 2 removed
//	run("format.conditional_move", answer={"rule": 2, "to": 1})       rule 2 moved first
//
// and the same for data validation, whose rules have no order.

func init() {
	hasFormats := func(m *Model) bool { return len(m.sheet.CondFormats()) > 0 }
	hasValidations := func(m *Model) bool { return len(m.sheet.Validations()) > 0 }
	register(
		&command{id: "format.conditional_set", title: "Replace conditional format rule", enabled: hasFormats,
			desc: `Replace a rule with one written as a line of the file, numbered: {"rule":2,"ranges":"B2:B9",...}`,
			run: func(m *Model) tea.Cmd {
				m.askRule(func(m *Model, text string) error {
					i, err := ruleNumber(text, len(m.sheet.CondFormats()))
					if err != nil {
						return err
					}
					f, err := sheet.ParseCondFormat(text)
					if err != nil {
						return err
					}
					return m.host().SaveFormat(i, f)
				})
				return nil
			}},
		&command{id: "format.conditional_remove", title: "Remove conditional format rule", enabled: hasFormats,
			desc: "Remove a rule by its number in the list, counting from 1",
			run: func(m *Model) tea.Cmd {
				m.askRule(func(m *Model, text string) error {
					i, err := ruleNumber(text, len(m.sheet.CondFormats()))
					if err == nil {
						m.sheet.DeleteCondFormat(i)
						m.changed = true
					}
					return err
				})
				return nil
			}},
		&command{id: "format.conditional_move", title: "Move conditional format rule", enabled: hasFormats,
			desc: `Move a rule to another place in the list, where the first rule that matches wins: {"rule":2,"to":1}`,
			run: func(m *Model) tea.Cmd {
				m.askRule(func(m *Model, text string) error {
					n := len(m.sheet.CondFormats())
					i, err := ruleNumber(text, n)
					if err != nil {
						return err
					}
					var a struct {
						To int `json:"to"`
					}
					if json.Unmarshal([]byte(text), &a); a.To < 1 || a.To > n {
						return fmt.Errorf(`give "to", a place from 1 to %d`, n)
					}
					m.sheet.MoveCondFormat(i, a.To-1)
					m.changed = true
					return nil
				})
				return nil
			}},
		&command{id: "data.validation_set", title: "Replace data validation rule", enabled: hasValidations,
			desc: `Replace a rule with one written as a line of the file, numbered: {"rule":2,"ranges":"D2:D9",...}`,
			run: func(m *Model) tea.Cmd {
				m.askRule(func(m *Model, text string) error {
					i, err := ruleNumber(text, len(m.sheet.Validations()))
					if err != nil {
						return err
					}
					v, err := sheet.ParseValidation(text)
					if err != nil {
						return err
					}
					return m.host().SaveValidation(i, v)
				})
				return nil
			}},
		&command{id: "data.validation_remove", title: "Remove data validation rule", enabled: hasValidations,
			desc: "Remove a rule by its number in the list, counting from 1",
			run: func(m *Model) tea.Cmd {
				m.askRule(func(m *Model, text string) error {
					i, err := ruleNumber(text, len(m.sheet.Validations()))
					if err == nil {
						m.sheet.DeleteValidation(i)
						m.changed = true
					}
					return err
				})
				return nil
			}},
	)
}

// askRule asks for a rule's number, or a rule, on the context line, and
// hands it to do.
func (m *Model) askRule(do func(m *Model, text string) error) {
	m.openText("Rule:", "", func(m *Model, text string) tea.Cmd {
		m.failOn(do(m, text))
		return nil
	})
}

// ruleNumber reads which of n rules an answer names, counting from 1:
// the number itself, or "rule" in a line of the file.
func ruleNumber(text string, n int) (int, error) {
	i, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		var a struct {
			Rule int `json:"rule"`
		}
		if json.Unmarshal([]byte(text), &a) != nil || a.Rule == 0 {
			return 0, fmt.Errorf(`give the rule's number, from 1 to %d: {"rule":1,...}`, n)
		}
		i = a.Rule
	}
	if i < 1 || i > n {
		return 0, fmt.Errorf("there's no rule %d: the sheet has %d", i, n)
	}
	return i - 1, nil
}

// numbered is a rule's line with its number first, as the commands that
// replace or move a rule take it.
func numbered(i int, line string) string {
	rest := strings.TrimPrefix(line, "{")
	if rest != "}" {
		rest = "," + rest
	}
	return `{"rule":` + strconv.Itoa(i+1) + rest
}
