# Conditional formatting and data validation: a task list whose done
# tasks are struck through in green, whose hours are shaded on a color
# scale, and whose total turns magenta past 100. Space checks a box,
# Alt+Down (here from the palette, as VHS can't type it) opens an
# owner's dropdown, a value the rule rejects is refused
# with Sheets' words, and Format > Conditional formatting lists the rules.
Output out/rules.gif
Source lib/setup.tape

Hide
Type "../bin/012 data/tasks.012" Enter
Sleep 2s
Show

Sleep 1s
Down@300ms 2 Right@300ms 3
Sleep 600ms
Space
Sleep 1.5s
Left@300ms 2 Down@300ms 2
Sleep 600ms
Ctrl+K
Type@100ms "open dropdown"
Sleep 500ms
Enter
Sleep 1s
Type@150ms "bo"
Sleep 800ms
Screenshot out/stills/rules-dropdown.png
Enter
Sleep 1s
Right Up@300ms 2
Type "90" Enter
Sleep 1.5s
Screenshot out/stills/rules-reject.png
Escape
Sleep 500ms
Type "70" Enter
Sleep 2s
Screenshot out/stills/rules-sheet.png
Sleep 500ms
Ctrl+K
Type@100ms "conditional formatting"
Sleep 600ms
Enter
Sleep 1s
Down@500ms 2
Sleep 1.5s
Screenshot out/stills/rules-panel.png
Escape
Sleep 1s
