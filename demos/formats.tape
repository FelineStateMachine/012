# Formats: select the amounts, make them Currency rounded from
# Format > Number, then bold the header row. Formats follow Sheets'
# presets, and a SUM of currency is currency too.
Output out/formats.gif
Source lib/setup.tape

Hide
Type "../bin/012 data/spending.csv" Enter
Sleep 2s
Show

Sleep 800ms
Down Right
Ctrl+Shift+Right
Ctrl+Shift+Down
Sleep 1s
Alt+o
Sleep 700ms
Right
Sleep 1s
Down@400ms 8
Sleep 800ms
Enter
Sleep 1.5s
Ctrl+Up Ctrl+Left
Ctrl+Shift+Right
Ctrl+B
Sleep 1s
Ctrl+Down Down
Type "Total" Tab Type "=SUM(B2:B7)" Tab Type "=SUM(C2:C7)" Tab Type "=SUM(D2:D7)" Enter
Sleep 3s
