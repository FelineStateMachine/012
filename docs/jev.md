# JEV functions

With a TypeSafe API key, four functions ask the hosted JEV model about a
value (a cell, a range or text). They follow Sheets' argument style: the
value, the question, then what the answers mean.

| Function | Returns | Like |
|---|---|---|
| `=JEV.TEST(A2, "Is this a complaint?", [yes means], [no means])` | TRUE or FALSE | an `IF` condition, a boolean mask |
| `=JEV.PROB(A2, "Is this a complaint?")` | probability of yes, as a percent | `predict_proba` |
| `=JEV.CLASSIFY(A2, "Sentiment", "negative, positive", [descriptions])` | the best label | `SWITCH`, `pd.cut` |
| `=JEV.SCORE(A2, "Urgency", "low, mid, high")` | a score from 0 to levels-1 | a rating scale |

Labels, descriptions and levels can also be ranges. Answers arrive in the
background (cells show `Loading…`), are cached by question, and the context
line shows the confidence for the selected cell. Data > Ask JEV again
re-asks the selection.

Set `TYPESAFE_API_KEY` in the environment or a `.env` file in the current
directory or next to the sheet; `TYPESAFE_BASE_URL` and
`TYPESAFE_DEFAULT_MODEL` are optional. Without a key the functions show
`#N/A` and say why. `JEV_LIVE_TEST=1 go test ./internal/jev -run TestLive`
checks the real service.

## How requests behave

- Requests run in the background, eight at a time; the status line counts
  them and terminals that support it (OSC 9;4) show progress in the tab.
- Answers are cached by the exact question (value, question and criteria),
  so recalculating, undo and redo never ask twice, and `JEV.TEST` and
  `JEV.PROB` on the same question share one request.
- Errors in the inputs (`#DIV/0!` in the value, an empty question, fewer
  than two labels, more than the service's 255 labels or 10 score levels)
  show as errors at once and are never sent.
- A failed request shows `#ERROR!`, and the context line says why.
- 012 never logs or saves the API key; `.env` files are read for
  `TYPESAFE_*` settings only.

The client is [typesafe-go](https://github.com/FelineStateMachine/typesafe-go).
