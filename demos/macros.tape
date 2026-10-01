# Macros: Data > Macros > Record macro with relative references (here
# from the palette) records a column total as a Starlark script; saved
# with a name and a shortcut, it replays from any cell, from Run macro
# or the palette (or Ctrl+Alt+Shift+1, which VHS can't type), as one
# undo step.
Output out/macros.gif
Source lib/setup.tape
Set Theme "Github"

Hide
Type "../bin/012 data/spending.csv" Enter
Sleep 2s
Show

Sleep 800ms
Ctrl+Down Down
Type "Total" Enter
Up Right
Sleep 500ms
Ctrl+K
Type@100ms "relative references"
Sleep 600ms
Enter
Sleep 1s
Type "=SUM(B2:B7)" Enter
Up
Ctrl+B
Sleep 800ms
Screenshot out/stills/macro-recording.png
Ctrl+K
Type@100ms "stop and save"
Sleep 600ms
Enter
Sleep 800ms
Type@120ms "Column total"
Enter
Sleep 800ms
Enter
Sleep 1.2s
Right
Ctrl+K
Type@100ms "run macro"
Sleep 600ms
Enter
Sleep 1s
Enter
Sleep 1.5s
Right
Ctrl+K
Type@100ms "column total"
Sleep 1s
Enter
Sleep 2s
Screenshot out/stills/macro-replayed.png
Sleep 1.5s
