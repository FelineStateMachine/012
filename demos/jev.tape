# JEV functions: ask a model about each review, Sheets-style. Answers
# arrive in the background (cells show Loading…), and the context line
# shows the confidence. Recorded against demos/fakejev, a stand-in
# service that `make demos` starts; no real key is involved.
Output out/jev.gif
Source lib/setup.tape

Hide
Type "export TYPESAFE_API_KEY=demo TYPESAFE_BASE_URL=http://127.0.0.1:8799" Enter
Type "../bin/012" Enter
Sleep 1.5s
Show

Sleep 600ms
Type "Review" Tab Type "Complaint?" Tab Type "Sentiment" Tab Type "Urgency" Enter
Type "Box arrived crushed" Enter
Type "Fast and friendly" Enter
Type "Wrong size, no reply" Enter
Ctrl+K
Type "column width"
Sleep 500ms
Enter
Type "22" Enter
Ctrl+G
Type "B2" Enter
Type@25ms '=JEV.TEST(A2, "Is this a complaint?")' Tab
Type@25ms '=JEV.CLASSIFY(A2, "Sentiment", "negative, positive")' Tab
Type@25ms '=JEV.SCORE(A2, "Urgency", "low, mid, high")' Enter
Sleep 1.5s
Ctrl+G
Type "B2:D4" Enter
Sleep 600ms
Ctrl+D
Sleep 2.5s
Escape
Right Down
Sleep 2.5s
Screenshot out/stills/jev-results.png
Sleep 500ms
