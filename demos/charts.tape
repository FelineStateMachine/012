# Charts: Insert > Chart charts the table around the active cell; the
# editor bar switches the type with Left/Right, and the chart redraws as
# its data changes. Terminals with the kitty graphics protocol get a real
# image; this recording shows the text rendering every terminal gets.
Output out/charts.gif
Source lib/setup.tape

Hide
Type "../bin/012 data/spending.csv" Enter
Sleep 2s
Show

Sleep 800ms
Alt+i
Sleep 900ms
Up
Sleep 500ms
Enter
Sleep 1.8s
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
