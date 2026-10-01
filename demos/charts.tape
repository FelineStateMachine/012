# Charts: Insert > Chart (here from the palette) charts the table
# around the active cell; the editor bar switches the type with
# Left/Right, and the chart redraws as its data changes. Terminals with kitty graphics or sixel get a real
# image; VHS records only the text layer, so this shows the text chart under it.
Output out/charts.gif
Source lib/setup.tape
Set Theme "Github"

Hide
Type "../bin/012 data/spending.csv" Enter
Sleep 2s
Show

Sleep 800ms
Ctrl+K
Type@120ms "chart"
Sleep 900ms
Enter
Sleep 1.8s
Screenshot out/stills/chart-editor.png
Right
Sleep 1.5s
Right
Sleep 1.5s
Right
Sleep 1.5s
Left@600ms 3
Sleep 800ms
Enter
Sleep 600ms
Escape
Sleep 600ms
Down@200ms 3 Right@200ms 2
Type "1400" Enter
Sleep 2.5s
Screenshot out/stills/chart-column.png
Sleep 500ms
