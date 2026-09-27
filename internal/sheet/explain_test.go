package sheet

import "testing"

func TestHyperlink(t *testing.T) {
	checkFormulas(t, fixture(t), []fnCase{
		{`=HYPERLINK("https://example.com")`, txt("https://example.com")},
		{`=HYPERLINK("https://example.com", "Example")`, txt("Example")},
		{`=HYPERLINK("https://example.com", "")`, txt("https://example.com")},
		{`=HYPERLINK(1/0, "x")`, ErrDiv0},
	})
}

func TestLink(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"A1": "https://example.com/a?b=1",
		"A2": "mailto:ann@example.com",
		"A3": "see https://example.com",
		"A4": "http://",
		"A5": `=HYPERLINK("example.com", "Example")`,
		"A6": `=HYPERLINK(A1, "Docs")`,
		"A7": `=HYPERLINK("ftp://x", "x")`,
		"A8": `=UPPER(A1)`,
		"A9": "12",
		"B1": `="https://"&"example.com"`,
	})
	tests := map[string]string{
		"A1": "https://example.com/a?b=1",
		"A2": "mailto:ann@example.com",
		"A3": "",
		"A4": "",
		"A5": "https://example.com",
		"A6": "https://example.com/a?b=1",
		"A7": "",
		"A8": "HTTPS://EXAMPLE.COM/A?B=1",
		"A9": "",
		"B1": "https://example.com",
		"C1": "",
	}
	for a, want := range tests {
		if got := s.Link(at(a)); got != want {
			t.Errorf("Link(%s) = %q, want %q", a, got, want)
		}
	}
}

func TestExplainError(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"A1": "10", "A2": "0", "A3": "text",
		"B1": "=A1/A2",
		"B2": "=B1+1",
		"B3": "=SUM(A1:B2)",
		"B4": "=A1+A3",
		"B5": "=FOO+1",
		"B6": `=VLOOKUP(99, A1:A2, 1, FALSE)`,
		"B7": "=SQRT(-1)",
		"B8": "=C8", "C8": "=B8",
		"B9":  "=A1:A2",
		"B10": "=IFERROR(B1, 0)",
		"B11": "=AVERAGE(A3)",
		"B12": "5",
	})
	tests := map[string]string{
		"B1":  "Division by zero in A1/A2",
		"B2":  "From B1: division by zero in A1/A2",
		"B3":  "From B1: division by zero in A1/A2",
		"B4":  "Wrong type of value in A1+A3",
		"B5":  "Unknown name FOO",
		"B6":  "No match found by VLOOKUP(99,A1:A2,1,FALSE)",
		"B7":  "Number out of range in SQRT(-1)",
		"B8":  "Circular reference: B8 → C8 → B8",
		"B9":  "A range where one value is expected: A1:A2",
		"B10": "",
		"B11": "Division by zero in AVERAGE(A3)",
		"B12": "",
		"A3":  "",
	}
	for a, want := range tests {
		if got := s.ExplainError(at(a)); got != want {
			t.Errorf("ExplainError(%s) = %q, want %q", a, got, want)
		}
	}
}
