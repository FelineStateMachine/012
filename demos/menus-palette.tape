# Menus and the command palette: Alt and a letter open the Sheets-style
# menus, arrows walk them and the status line says what each item does;
# Ctrl+K searches every command by name and shows its keys.
Output out/menus-palette.gif
Source lib/setup.tape

Hide
Type "../bin/012 data/inventory.csv" Enter
Sleep 2s
Show

Sleep 800ms
Alt+f
Sleep 1.2s
Down@400ms 3
Sleep 800ms
Right@900ms 4
Sleep 500ms
Right
Sleep 1.2s
Escape@300ms 2
Sleep 600ms
Ctrl+K
Sleep 1s
Type@120ms "freeze"
Sleep 1.2s
Down@500ms 2
Sleep 800ms
Enter
Sleep 1.5s
PageDown
Sleep 1.5s
Ctrl+K
Type@120ms "width"
Sleep 1.5s
Escape
Sleep 1s
