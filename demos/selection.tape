# Selection and live stats: select a column of numbers and the status
# line shows its Sum, Avg and Count; widen the selection, select whole
# columns, then everything. (VHS can't drive the mouse; clicking,
# dragging and Shift+click select the same way.)
Output out/selection.gif
Source lib/setup.tape

Hide
Type "../bin/012 data/inventory.csv" Enter
Sleep 2s
Show

Sleep 800ms
Right@300ms 3 Down
Sleep 600ms
Ctrl+Shift+Down
Sleep 2s
Screenshot out/stills/selection-stats.png
Ctrl+Shift+Right
Sleep 2s
Escape
Left
Ctrl+Space
Sleep 2s
Ctrl+A
Sleep 2s
Escape
Sleep 1s
