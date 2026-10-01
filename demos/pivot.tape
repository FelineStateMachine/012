# Pivot tables: Data > Pivot table (here from the palette) summarizes
# the table around the active cell on a new sheet, and its editor docks
# at the right: Space adds a field to Rows, Columns or Values, and the
# results update behind it. They stay live: a change to the data
# recomputes them.
Output out/pivot.gif
Source lib/setup.tape
Set Theme "Github"

Hide
Type "../bin/012 data/sales.csv" Enter
Sleep 2s
Show

Sleep 800ms
Ctrl+K
Type@100ms "pivot table"
Sleep 600ms
Enter
Sleep 1.5s
Space
Type@150ms "reg"
Sleep 600ms
Enter
Sleep 1.2s
Down
Space
Type@150ms "quar"
Sleep 600ms
Enter
Sleep 1.2s
Down
Space
Type@150ms "rev"
Sleep 600ms
Enter
Sleep 2s
Screenshot out/stills/pivot-editor.png
Enter
Sleep 1s
Ctrl+G
Type "sales!D2" Enter
Sleep 800ms
Type "9000" Enter
Sleep 800ms
Ctrl+G
Type "'Pivot Table 1'!A1" Enter
Sleep 2.5s
Screenshot out/stills/pivot-results.png
Sleep 500ms
