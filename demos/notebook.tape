# Notebooks: 012 nu opens a notebook, its first cell ready to type in.
# A code cell reads a CSV as sales, a second reads it as $sales, and its
# output goes to a sheet (G) where a formula sums it; with the notebook
# reactive, editing sales runs both again and the sheet follows. The terminal (ttyd) sends Shift+Enter as Enter, so
# cells run with Esc then r. Needs nu on the PATH.
Output out/notebook.gif
Source lib/setup.tape

Hide
Type "../bin/012 nu" Enter
Sleep 2s
Show

Sleep 800ms
Type@25ms "sales = open data/sales.csv | where Region in [North South] | select Region Quarter Units"
Sleep 500ms
Escape
Type "r"
Sleep 1.5s
Type "b"
Enter
Type@25ms "top = $sales | where Units > 250 | sort-by Units --reverse"
Sleep 500ms
Escape
Type "r"
Sleep 2s
Screenshot out/stills/notebook-cells.png
Type "G"
Sleep 800ms
Enter
Sleep 1.2s
Ctrl+K
Type@60ms "next sheet"
Sleep 500ms
Enter
Sleep 600ms
Right Right Right Right
Type "=SUM(nu.top)" Enter
Sleep 1.5s
Screenshot out/stills/notebook-sent.png
Ctrl+K
Type@60ms "previous sheet"
Sleep 500ms
Enter
Sleep 600ms
Ctrl+K
Type@80ms "reactive"
Sleep 600ms
Enter
Sleep 400ms
Up Up
Sleep 300ms
Enter
Type@30ms " | where Quarter != Q1"
Escape
Type "r"
Sleep 2.5s
Screenshot out/stills/notebook-reactive.png
Ctrl+K
Type@60ms "next sheet"
Sleep 500ms
Enter
Sleep 2s
Screenshot out/stills/notebook-sum.png
