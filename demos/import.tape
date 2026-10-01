# Import: File > Import lists the spreadsheets in the folder (CSV, TSV,
# XLSX, SQLite, Parquet, Lotus .wk1); type to narrow, Enter imports in
# the background with a progress bar. Entries become numbers and
# currency as if typed.
Output out/import.gif
Source lib/setup.tape
Set Theme "GruvboxDark"

Hide
Type "cd data && ../../bin/012" Enter
Sleep 1.5s
Show

Sleep 800ms
Alt+f
Sleep 700ms
Down@400ms 2
Sleep 700ms
Enter
Sleep 1.5s
Screenshot out/stills/import-picker.png
Type@200ms "sal"
Sleep 1s
Enter
Sleep 2s
Right@300ms 3 Down
Ctrl+Shift+Down
Sleep 3s
Screenshot out/stills/import-sales.png
Sleep 500ms
