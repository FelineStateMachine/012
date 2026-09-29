# Notebooks: 012 nu opens a notebook at the nu❯ prompt. Each pipeline's
# table becomes a named region; a second reads the first as $sales, and
# giving sales a new command runs it again and then the region reading
# it. Needs nu on the PATH.
Output out/notebook.gif
Source lib/setup.tape

Hide
Type "../bin/012 nu" Enter
Sleep 2s
Show

Sleep 800ms
Type "sales = open data/sales.csv | where Region in [North South] | select Region Quarter Units"
Sleep 500ms
Enter
Sleep 1.5s
Type "$sales | where Units > 250 | sort-by Units --reverse"
Sleep 500ms
Enter
Sleep 2s
Screenshot out/stills/notebook-regions.png
Type "sales = open data/sales.csv | where Quarter == Q4 | select Region Quarter Units"
Sleep 500ms
Enter
Sleep 2.5s
Escape
Sleep 500ms
Down Down Down
Sleep 1.5s
Screenshot out/stills/notebook-refreshed.png
Sleep 1.5s
