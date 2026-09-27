# Layout: a trip plan gets a title merged across it, borders (a thick
# outline and a double line under the headers), and notes that wrap,
# their rows growing to fit, all from the command palette. Go to
# (Ctrl+G) selects the ranges.
Output out/layout.gif
Source lib/setup.tape

Hide
Type "../bin/012 data/trip.012" Enter
Sleep 1.5s
Show

Sleep 800ms
Ctrl+G
Type "A1:C1" Enter
Ctrl+K
Type "merge all"
Sleep 300ms
Enter
Sleep 800ms
Ctrl+G
Type "A2:C6" Enter
Ctrl+K
Type "all borders"
Sleep 300ms
Enter
Sleep 800ms
Ctrl+K
Type "thick lines"
Sleep 300ms
Enter
Ctrl+K
Type "outer borders"
Sleep 300ms
Enter
Sleep 800ms
Ctrl+G
Type "A2:C2" Enter
Ctrl+K
Type "double lines"
Sleep 300ms
Enter
Ctrl+K
Type "bottom border"
Sleep 300ms
Enter
Sleep 800ms
Ctrl+G
Type "C3:C5" Enter
Ctrl+K
Type "wrap"
Sleep 300ms
Enter
Sleep 800ms
Ctrl+G
Type "C4" Enter
Sleep 2s
Screenshot out/stills/layout-trip.png
Sleep 1s
