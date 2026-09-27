package sheet

import "testing"

func TestSplitAndRegex(t *testing.T) {
	checkArrays(t, fixture(t), []arrCase{
		{`=SPLIT("a,b,,c", ",")`, []string{"a|b|c"}},
		{`=SPLIT("a,b,,c", ",", TRUE, FALSE)`, []string{"a|b||c"}},
		{`=SPLIT("a-b c", "- ")`, []string{"a|b|c"}},
		{`=SPLIT("a--b", "--", FALSE)`, []string{"a|b"}},
		{`=SPLIT("1,2", ",")`, []string{"1|2"}},
		{`=SUM(SPLIT("1,2", ","))`, []string{"3"}},
		{`=SPLIT("abc", "")`, []string{"#VALUE!"}},
		{`=ARRAYFORMULA(SPLIT({"a b";"c d e"}, " "))`, []string{"a|b|", "c|d|e"}},
		{`=REGEXMATCH("order 42", "[0-9]+")`, []string{"TRUE"}},
		{`=REGEXMATCH("order", "^[0-9]+$")`, []string{"FALSE"}},
		{`=REGEXMATCH(42, "4")`, []string{"#VALUE!"}},
		{`=REGEXMATCH("a", "(")`, []string{"#VALUE!"}},
		{`=REGEXEXTRACT("order 42", "[0-9]+")`, []string{"42"}},
		{`=REGEXEXTRACT("key=val", "(\w+)=(\w+)")`, []string{"key|val"}},
		{`=REGEXEXTRACT("key=val", "=(\w+)")`, []string{"val"}},
		{`=REGEXEXTRACT("abc", "[0-9]")`, []string{"#N/A"}},
		{`=REGEXREPLACE("2026-09-27", "(\d+)-(\d+)-(\d+)", "$3/$2/$1")`, []string{"27/09/2026"}},
		{`=REGEXREPLACE("a  b", " +", " ")`, []string{"a b"}},
		{`=ARRAYFORMULA(REGEXMATCH(D1:D3, "an"))`, []string{"FALSE", "TRUE", "FALSE"}},
		{`=FILTER(D1:D5, REGEXMATCH(D1:D5, "^A"))`, []string{"Apple", "Apple"}},
	})
}
