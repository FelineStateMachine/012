package sheet

func init() {
	define(
		&FuncDef{Name: "IFS", Args: "condition1, value1, [condition2, value2, ...]", Desc: "The value of the first true condition", Min: 2, Max: -1, step: 2,
			eval: func(args []Node, get lookup) Value {
				for i := 0; i+1 < len(args); i += 2 {
					c, err := numArg(args[i], get)
					if err != nil {
						return *err
					}
					if c != 0 {
						return eval(args[i+1], get)
					}
				}
				return ErrNA
			}, format: func(args []Node, infer func(Node) Format) Format {
				var vals []Node
				for i := 1; i < len(args); i += 2 {
					vals = append(vals, args[i])
				}
				return inherit(vals, infer)
			}},
		&FuncDef{Name: "SWITCH", Args: "expression, case1, value1, [case2, value2, ...], [default]", Desc: "The value for the first case equal to an expression", Min: 3, Max: -1,
			eval: func(args []Node, get lookup) Value {
				x := eval(args[0], get)
				if x.Kind == Error {
					return x
				}
				i := 1
				for ; i+1 < len(args); i += 2 {
					c := eval(args[i], get)
					if c.Kind == Error {
						return c
					}
					if sameKind(x, c) && compare(x, c) == 0 {
						return eval(args[i+1], get)
					}
				}
				if i < len(args) {
					return eval(args[i], get)
				}
				return ErrNA
			}},
		&FuncDef{Name: "XOR", Args: "logical1, [logical2, ...]", Desc: "TRUE if an odd number of arguments are true", Min: 1, Max: -1,
			eval: logical(func(t, _ int) bool { return t%2 == 1 })},
		&FuncDef{Name: "IFNA", Args: "value, value_if_na", Desc: "A fallback when a value is #N/A", Min: 2, Max: 2,
			eval: func(args []Node, get lookup) Value {
				v := eval(args[0], get)
				if v == ErrNA {
					return eval(args[1], get)
				}
				return v
			}, format: inherit},
		&FuncDef{Name: "ISBLANK", Args: "value", Desc: "TRUE if a cell is empty", Min: 1, Max: 1,
			eval: is(func(v Value) bool { return v.Kind == Empty })},
		&FuncDef{Name: "ISNUMBER", Args: "value", Desc: "TRUE if a value is a number", Min: 1, Max: 1,
			eval: is(func(v Value) bool { return v.Kind == Number })},
		&FuncDef{Name: "ISTEXT", Args: "value", Desc: "TRUE if a value is text", Min: 1, Max: 1,
			eval: is(func(v Value) bool { return v.Kind == Text })},
		&FuncDef{Name: "ISLOGICAL", Args: "value", Desc: "TRUE if a value is TRUE or FALSE", Min: 1, Max: 1,
			eval: is(func(v Value) bool { return v.Kind == Bool })},
		&FuncDef{Name: "ISERROR", Args: "value", Desc: "TRUE if a value is any error", Min: 1, Max: 1,
			eval: is(func(v Value) bool { return v.Kind == Error })},
		&FuncDef{Name: "ISERR", Args: "value", Desc: "TRUE if a value is an error other than #N/A", Min: 1, Max: 1,
			eval: is(func(v Value) bool { return v.Kind == Error && v != ErrNA })},
		&FuncDef{Name: "ISNA", Args: "value", Desc: "TRUE if a value is #N/A", Min: 1, Max: 1,
			eval: is(func(v Value) bool { return v == ErrNA })},
	)
}

// is builds the IS functions, which never return an error themselves.
func is(test func(Value) bool) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		return boolean(test(eval(args[0], get)))
	}
}

// sameKind reports whether two values can be equal in SWITCH and exact
// lookups: numbers only equal numbers, text only text. A blank is 0 or "".
func sameKind(a, b Value) bool {
	ka, kb := a.Kind, b.Kind
	if ka == Empty || kb == Empty {
		return true
	}
	return ka == kb
}
