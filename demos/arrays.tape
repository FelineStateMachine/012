# Arrays and spills: UNIQUE and SORT list the regions, FILTER lists the
# big quarters, each spilling into the cells beside and below it, and a
# change to the data resizes the spill. The context line says where a
# spilled value comes from.
Output out/arrays.gif
Source lib/setup.tape

Hide
Type "../bin/012 data/sales.csv" Enter
Sleep 2s
Show

Sleep 800ms
Ctrl+G
Type "F1" Enter
Type "Regions" Enter
Type@60ms "=SORT(UNIQUE(A2:A17))" Enter
Sleep 1.5s
Ctrl+G
Type "F8" Enter
Type "Over 300 units" Enter
Type@60ms "=FILTER(A2:C17, C2:C17>300)" Enter
Sleep 2s
Screenshot out/stills/arrays-spill.png
Ctrl+G
Type "C2" Enter
Type "310" Enter
Sleep 2s
Ctrl+G
Type "G11" Enter
Sleep 1.5s
Screenshot out/stills/arrays-spilled-from.png
Sleep 1s
