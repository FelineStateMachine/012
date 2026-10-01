# Notebooks: 012 nu opens a notebook, its first cell ready to type in.
# A code cell reads a CSV as sales, a second reads it as $sales, and its
# output, entered with Enter, is a grid to select in, and goes to a
# sheet (G) where a formula sums it; with the notebook
# reactive, editing sales runs both again and the sheet follows. The terminal (ttyd) sends Shift+Enter as Enter, so
# cells run with Esc then r. Needs nu on the PATH.
Output out/notebook.gif
Source lib/setup.tape
Set Theme "tokyonight"

Hide
Type "../bin/012 nu" Enter
Sleep 2s
Show

Sleep 800ms
Type@15ms "sales = open data/sales.csv | where Region in [North South] | select Region Quarter Units"
Sleep 500ms
Escape
Type "r"
Sleep 1s
Type "b"
Enter
Type@15ms "top = $sales | where Units > 250 | sort-by Units --reverse"
Sleep 500ms
Escape
Type "r"
Sleep 2s
Screenshot out/stills/notebook-cells.png
Down
Enter
Sleep 400ms
Ctrl+Shift+Down
Sleep 1s
Screenshot out/stills/notebook-grid.png
Escape Escape
Sleep 500ms
Type "G"
Sleep 800ms
Enter
Sleep 1.2s
Ctrl+K
Type@20ms "next sh"
Sleep 500ms
Enter
Sleep 400ms
Right Right Right Right
Type "=SUM(nu.top)" Enter
Sleep 1s
Screenshot out/stills/notebook-sent.png
Ctrl+K
Type@20ms "previous sh"
Sleep 500ms
Enter
Sleep 400ms
Ctrl+K
Type@20ms "reacti"
Sleep 400ms
Enter
Sleep 400ms
Up Up Up
Sleep 300ms
Enter
Type@20ms " | where Quarter != Q1"
Escape
Type "r"
Sleep 2s
Screenshot out/stills/notebook-reactive.png
Ctrl+K
Type@20ms "next sh"
Sleep 500ms
Enter
Sleep 2s
Screenshot out/stills/notebook-sum.png
