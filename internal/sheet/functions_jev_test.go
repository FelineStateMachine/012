package sheet

import (
	"reflect"
	"testing"
)

// fakeRemote answers from a table and records every call it's asked.
type fakeRemote struct {
	answers map[string]RemoteAnswer // by Instructions
	asked   []RemoteCall
}

func (f *fakeRemote) Lookup(c RemoteCall) (RemoteAnswer, bool) {
	f.asked = append(f.asked, c)
	a, ok := f.answers[c.Instructions]
	return a, ok
}

func TestJEVFunctions(t *testing.T) {
	f := &fakeRemote{answers: map[string]RemoteAnswer{
		"Is this a complaint?": {Noul: 0.83},
		"Sentiment":            {Choice: "negative", Confidence: 0.9},
		"Urgency":              {Score: 1.6, Confidence: 0.7},
		"Broken":               {Failed: "HTTP 500"},
	}}
	s := New()
	s.SetRemote(f)
	s.Set(at("A1"), "The package arrived crushed and support never replied.")
	s.Set(at("B1"), "positive")
	s.Set(at("B2"), "negative")
	s.Set(at("B3"), "neutral")

	tests := []struct {
		formula string
		want    Value
	}{
		{`=JEV.TEST(A1, "Is this a complaint?")`, boolean(true)},
		{`=JEV.PROB(A1, "Is this a complaint?")`, num(0.83)},
		{`=JEV.CLASSIFY(A1, "Sentiment", B1:B3)`, Value{Kind: Text, Str: "negative"}},
		{`=JEV.CLASSIFY(A1, "Sentiment", "positive, negative, neutral")`, Value{Kind: Text, Str: "negative"}},
		{`=JEV.SCORE(A1, "Urgency", "low, medium, high")`, num(1.6)},
		{`=JEV.TEST(A1, "Not answered yet")`, Pending},
		{`=JEV.TEST(A1, "Broken")`, ErrRemote},
		{`=JEV.TEST(1/0, "Is this a complaint?")`, ErrDiv0},      // bad input isn't sent
		{`=JEV.CLASSIFY(A1, "Sentiment", "only one")`, ErrValue}, // nothing to choose
		{`=IF(JEV.TEST(A1, "Is this a complaint?"), "escalate", "ok")`, Value{Kind: Text, Str: "escalate"}},
	}
	for _, tt := range tests {
		if err := s.Set(at("C1"), tt.formula); err != nil {
			t.Fatalf("Set(%q): %v", tt.formula, err)
		}
		if got := s.Value(at("C1")); got != tt.want {
			t.Errorf("%s = %+v, want %+v", tt.formula, got, tt.want)
		}
	}
	// JEV.PROB shows as a percent.
	s.Set(at("C1"), `=JEV.PROB(A1, "Is this a complaint?")`)
	if got := s.DisplayFormat(at("C1")).Kind; got != FmtPercent {
		t.Errorf("JEV.PROB format %v", got)
	}
}

func TestJEVCalls(t *testing.T) {
	f := &fakeRemote{}
	s := New()
	s.SetRemote(f)
	s.Set(at("A1"), "text")
	s.Set(at("A2"), "42")
	s.Set(at("B1"), "good")
	s.Set(at("B2"), "bad")
	s.Set(at("C1"), "It works")
	s.Set(at("C2"), "It doesn't")
	s.Set(at("D1"), `=JEV.CLASSIFY(A1:A2, "Quality", B1:B2, C1:C2)`)
	calls := s.RemoteCalls(at("D1"))
	want := RemoteCall{
		Kind:         "choice",
		State:        [][]any{{"text"}, {42.0}},
		Instructions: "Quality",
		Criteria:     map[string]string{"good": "It works", "bad": "It doesn't"},
	}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], want) {
		t.Errorf("calls %#v", calls)
	}
	s.Set(at("D2"), `=JEV.TEST(A1, "Q", "yes means this")`)
	if c := s.RemoteCalls(at("D2")); len(c) != 1 || c[0].Criteria != [2]string{"yes means this", ""} {
		t.Errorf("noul criteria %#v", c)
	}
	// Answers arriving later show up after RecalcVolatile.
	f.answers = map[string]RemoteAnswer{"Q": {Noul: 0.1}}
	if s.Value(at("D2")) != Pending {
		t.Fatalf("D2 = %+v before answers", s.Value(at("D2")))
	}
	s.Set(at("E1"), "=IF(D2, 1, 2)")
	state := s.StateID()
	s.RecalcVolatile()
	if s.Value(at("D2")) != boolean(false) || s.Value(at("E1")).Num != 2 {
		t.Errorf("after answers: D2 %+v E1 %+v", s.Value(at("D2")), s.Value(at("E1")))
	}
	if s.StateID() != state {
		t.Error("answers arriving counted as an edit")
	}
}

func TestJEVWithoutKey(t *testing.T) {
	s := New()
	s.Set(at("A1"), `=JEV.TEST("x", "Q")`)
	if s.Value(at("A1")) != ErrNoRemote {
		t.Errorf("no key: %+v", s.Value(at("A1")))
	}
}
