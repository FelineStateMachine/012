# Freeze, sort and filter: keep the header row on screen, sort by the
# active column (frozen rows stay on top), then filter a column by its
# values; the status line counts the hidden rows.
Output out/freeze-sort-filter.gif
Source lib/setup.tape

Hide
Type "../bin/012 data/inventory.csv" Enter
Sleep 2s
Show

Sleep 800ms
Alt+v
Sleep 600ms
Enter
Sleep 600ms
Down
Sleep 500ms
Enter
Sleep 1s
PageDown
Sleep 1.5s
Ctrl+Up Ctrl+Left Right@200ms 4
Alt+d
Sleep 600ms
Enter
Sleep 600ms
Down
Sleep 600ms
Enter
Sleep 1.5s
Left@200ms 3
Alt+d
Sleep 500ms
Type "c"
Sleep 1s
Alt+d
Type "f"
Sleep 1.2s
Space
Sleep 500ms
Type@150ms "tech"
Sleep 800ms
Down
Space
Sleep 800ms
Screenshot out/stills/filter-picker.png
Sleep 800ms
Enter
Sleep 3s
Screenshot out/stills/filtered.png
Sleep 500ms
