# Following a log: a JSON lines log grows while 012 follows it in a
# linked region (Data > Linked file > Link a table, here from the
# palette). A shell region reads it as $app and keeps the errors;
# refreshing it later reads the rows the log has now. The log is
# data/requests.jsonl written a line at a time. Needs nu on the PATH.
Output out/follow-log.gif
Source lib/setup.tape

Hide
Type "rm -f out/app.jsonl; head -n 6 data/requests.jsonl > out/app.jsonl" Enter
Type `(tail -n +7 data/requests.jsonl | while IFS= read -r l; do sleep 0.8; printf '%s\n' "$l" >> out/app.jsonl; done) & clear; ../bin/012` Enter
Sleep 2s
Show

Sleep 600ms
Ctrl+K
Type@100ms "link a table"
Sleep 600ms
Enter
Sleep 800ms
Type "out/app.jsonl"
Sleep 500ms
Enter
Sleep 1s
Enter
Sleep 3.5s
Screenshot out/stills/follow-log-linked.png
Type "!"
Sleep 600ms
Type "errors = $app | where status >= 500 | select time path status ms"
Sleep 500ms
Enter
Sleep 2.5s
Screenshot out/stills/follow-log-errors.png
Escape
Sleep 6s
Enter
Sleep 2.5s
Screenshot out/stills/follow-log-refreshed.png
Sleep 1s
