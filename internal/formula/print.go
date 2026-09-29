package formula

import (
	"math"
	"strconv"
	"strings"
)

// Text prints a parsed formula back to text, with a leading "=".
// Rewritten formulas (after a paste or an inserted row) are stored this
// way, in Sheets' spelling: ranges as A1:B3, functions without @.
func Text(n Node) string {
	var b strings.Builder
	b.WriteByte('=')
	printNode(&b, n)
	return b.String()
}

// Expr prints an expression without the leading "=", e.g. to quote part
// of a formula.
func Expr(n Node) string {
	var b strings.Builder
	printNode(&b, n)
	return b.String()
}

// atomPower is the binding power of nodes that never need parentheses.
const atomPower = 100

// power returns how tightly n binds, using the parser's binding powers, so
// the printer adds parentheses exactly where the parser needs them.
func power(n Node) int {
	switch n := n.(type) {
	case Binary:
		return infixPower[n.Op]
	case Unary:
		switch n.Op {
		case "%":
			return percentPower
		case "#NOT#":
			return notPower
		}
		return unaryPower
	}
	return atomPower
}

func printNode(b *strings.Builder, n Node) {
	switch n := n.(type) {
	case Num:
		b.WriteString(numText(n.V))
	case Str:
		b.WriteString(`"` + strings.ReplaceAll(n.V, `"`, `""`) + `"`)
	case Bool:
		b.WriteString(strings.ToUpper(strconv.FormatBool(n.V)))
	case Ref:
		writeSheet(b, n.Sheet)
		b.WriteString(RefString(n.Addr, n.Abs))
	case Range:
		writeSheet(b, n.Sheet)
		b.WriteString(RangeString(n.Rect, n.Abs))
	case RefErr:
		b.WriteString(refErrorText)
	case Name:
		b.WriteString(n.Name)
	case TableRef:
		b.WriteString(n.String())
	case Unary:
		printUnary(b, n)
	case Binary:
		p := infixPower[n.Op]
		// Operators associate to the left, so a right operand of equal
		// power needs parentheses: 1-(2-3).
		printChild(b, n.L, power(n.L) < p)
		b.WriteString(n.Op)
		printChild(b, n.R, power(n.R) <= p)
	case Call:
		b.WriteString(n.Fn.Signature().Name)
		printArgs(b, n.Args)
	case Local:
		b.WriteString(n.Name)
	case Invoke:
		printNode(b, n.Fn)
		printArgs(b, n.Args)
	case Array:
		b.WriteByte('{')
		for i, row := range n.Rows {
			if i > 0 {
				b.WriteByte(';')
			}
			for j, e := range row {
				if j > 0 {
					b.WriteByte(',')
				}
				printNode(b, e)
			}
		}
		b.WriteByte('}')
	}
}

// printArgs writes a call's arguments in parentheses.
func printArgs(b *strings.Builder, args []Node) {
	b.WriteByte('(')
	for i, a := range args {
		if i > 0 {
			b.WriteByte(',')
		}
		printNode(b, a)
	}
	b.WriteByte(')')
}

func printUnary(b *strings.Builder, n Unary) {
	if n.Op == "%" {
		printChild(b, n.X, power(n.X) < percentPower)
		b.WriteByte('%')
		return
	}
	b.WriteString(n.Op)
	printChild(b, n.X, power(n.X) < power(n))
}

// writeSheet writes a reference's sheet and its "!", quoted as needed.
func writeSheet(b *strings.Builder, sheet string) {
	if sheet != "" {
		b.WriteString(QuoteSheet(sheet) + "!")
	}
}

func printChild(b *strings.Builder, n Node, paren bool) {
	if paren {
		b.WriteByte('(')
	}
	printNode(b, n)
	if paren {
		b.WriteByte(')')
	}
}

// numText writes a number literal so it parses back to the same float:
// plainly in everyday ranges, in E notation for very large or small ones.
func numText(v float64) string {
	if a := math.Abs(v); a == 0 || (a >= 1e-6 && a < 1e21) {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strconv.FormatFloat(v, 'E', -1, 64)
}
