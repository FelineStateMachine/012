# Live mode: a household budget open with 012 --listen, and an agent,
# claude (liveagent, scripted), attached with 012 mcp --attach. Claude
# points at the bills and suggests March's figures; its cells carry a
# ◇ and the context line says what it would put there. The person
# reviews and accepts them, answers claude's question on the context
# line, and accepts the line it then suggests.
Output out/live.gif
Source lib/setup.tape

Hide
Type "export XDG_RUNTIME_DIR=$(mktemp -d) && go build -o ../bin/liveagent ./liveagent && ../bin/liveagent -write out/budget.012 && (../bin/liveagent >/dev/null 2>&1 &) && clear && ../bin/012 --listen out/budget.012" Enter
Wait+Screen@60s /READY/
Show

Wait+Screen@20s /claude/
Sleep 1.5s
Wait+Screen@20s /1 suggestion/
Sleep 800ms
Down 3
Right
Sleep 2s
Screenshot out/stills/live-suggestion.png
Sleep 1s
Ctrl+K
Type@80ms "review suggestions"
Sleep 400ms
Enter
Sleep 1.5s
Screenshot out/stills/live-review.png
Sleep 1s
Enter
Sleep 1.5s
Wait+Screen@20s /asks/
Sleep 1.5s
Screenshot out/stills/live-ask.png
Sleep 1s
Enter
Wait+Screen@20s /1 suggestion/
Sleep 1s
Ctrl+K
Type@80ms "review suggestions"
Sleep 400ms
Enter
Sleep 1.5s
Enter
Sleep 2.5s
