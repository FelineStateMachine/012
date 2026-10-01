# Pipelines: in a nushell session, a table goes through 012 and on with
# the sheet command. ls's table shows its sizes in the Size format, a
# selection is sent back when 012 quits (--send selection, so it doesn't
# ask), and nu carries on with sizes that are still file sizes. Needs nu
# on the PATH.
Output out/pipeline.gif
Source lib/setup.tape
Set Theme "tokyonight"
Source lib/nu.tape
Hide
Type "^012 nu --install-module ../bin/012.nu" Enter
Sleep 1s
Type "use ../bin/012.nu *; clear" Enter
Sleep 1s

Show
Sleep 600ms
Type "ls data | select name size"
Sleep 400ms
Enter
Sleep 2.2s
Type "ls data | select name size | sheet --send selection | where size > 400b"
Sleep 600ms
Enter
Sleep 2s
Screenshot out/stills/pipeline-sheet.png
Ctrl+G
Type "A1:B4" Enter
Sleep 1s
Ctrl+Q
Sleep 2.5s
Screenshot out/stills/pipeline-result.png
Sleep 1s
