package sheet

import (
	"math"
	"strconv"
	"strings"
)

// formulaText prints a parsed formula back to text, with a leading "=".
// Rewritten formulas (after a paste or an inserted row) are stored this
// way, in Sheets' spelling: ranges as A1:B3, functions without @.
func formulaText(n Node) string {
	var b strings.Builder
	b.WriteByte('=')
	printNode(&b, n)
	return b.String()
}

// atomPower is the binding power of nodes that never need parentheses.
const atomPower = 100

// power returns how tightly n binds, using the parser's binding powers, so
// the printer adds parentheses exactly where the parser needs them.
func power(n Node) int {
	switch n := n.(type) {
	case binaryNode:
		return infixPower[n.op]
	case unaryNode:
		switch n.op {
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
	case numLit:
		b.WriteString(numText(n.v))
	case strLit:
		b.WriteString(`"` + strings.ReplaceAll(n.v, `"`, `""`) + `"`)
	case boolLit:
		b.WriteString(strings.ToUpper(strconv.FormatBool(n.v)))
	case refNode:
		b.WriteString(refString(n.a, n.abs))
	case rangeNode:
		b.WriteString(refString(n.r.From, n.abs[0]) + ":" + refString(n.r.To, n.abs[1]))
	case refErrNode:
		b.WriteString("#REF!")
	case nameNode:
		b.WriteString(n.name)
	case unaryNode:
		if n.op == "%" {
			printChild(b, n.x, power(n.x) < percentPower)
			b.WriteByte('%')
			return
		}
		b.WriteString(n.op)
		printChild(b, n.x, power(n.x) < power(n))
	case binaryNode:
		p := infixPower[n.op]
		// Operators associate to the left, so a right operand of equal
		// power needs parentheses: 1-(2-3).
		printChild(b, n.l, power(n.l) < p)
		b.WriteString(n.op)
		printChild(b, n.r, power(n.r) <= p)
	case callNode:
		b.WriteString(n.fn.Name + "(")
		for i, a := range n.args {
			if i > 0 {
				b.WriteByte(',')
			}
			printNode(b, a)
		}
		b.WriteByte(')')
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
