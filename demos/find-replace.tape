# Find and replace: matches light up as you type and the active cell
# follows the current one. Find and replace keeps the query, takes the
# replacement on the same bar, and Alt+A replaces every match.
Output out/find-replace.gif
Source lib/setup.tape

Hide
Type "../bin/012 data/inventory.csv" Enter
Sleep 2s
Show

Sleep 800ms
Ctrl+F
Sleep 500ms
Type@150ms "acme"
Sleep 1.2s
Screenshot out/stills/find-bar.png
Enter
Sleep 800ms
Enter
Sleep 800ms
Escape
Sleep 600ms
Ctrl+K
Type@100ms "replace"
Sleep 800ms
Enter
Sleep 800ms
Type@120ms "Acme Co."
Sleep 1s
Alt+a
Sleep 3s
Screenshot out/stills/replace-all.png
Sleep 500ms
