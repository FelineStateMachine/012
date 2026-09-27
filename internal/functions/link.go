package functions

import (
	"github.com/FelineStateMachine/012/internal/value"
)

func init() {
	define(&FuncDef{Name: "HYPERLINK", Args: "url, [link_label]", Desc: "A link that opens url, shown as its label", Min: 1, Max: 2,
		eval: func(args []Node, get lookup) Value {
			url, err := textArg(args[0], get)
			if err != nil {
				return *err
			}
			if given(args, 1) {
				label, err := textArg(args[1], get)
				if err != nil {
					return *err
				}
				if label != "" {
					return Value{Kind: value.Text, Str: label}
				}
			}
			return Value{Kind: value.Text, Str: url}
		}})
}
