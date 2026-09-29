# Pipelines: in a nushell session, a table goes through 012 and on.
# ls's table goes in as NUON, 012 shows its sizes in the Size format, a
# selection is sent back, and nu carries on with sizes that are still
# file sizes. Needs nu on the PATH.
Output out/pipeline.gif
Source lib/setup.tape
Source lib/nu.tape

Show
Sleep 600ms
Type "ls data | select name size"
Sleep 400ms
Enter
Sleep 2.2s
Type "ls data | select name size | to nuon | ^012 --pipe | from nuon | where size > 400b"
Sleep 600ms
Enter
Sleep 2s
Screenshot out/stills/pipeline-sheet.png
Ctrl+G
Type "A1:B4" Enter
Sleep 1s
Ctrl+Q
Sleep 1.6s
Enter
Sleep 2.5s
Screenshot out/stills/pipeline-result.png
Sleep 1s
