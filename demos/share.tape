# Sharing a workbook over 012 serve: ann (top pane) and bob (bottom)
# open the same budget through ssh, in tmux panes (Ctrl+B and an arrow
# moves between them). Bob changes the food line, ann the rent: each
# sees the other's change arrive, their pointer in its color and their
# initial on the row header. Ann lists who's here; bob's undo takes
# back only his own change. Needs tmux, ssh and ssh-keygen.
Output out/share.gif
Source lib/setup.tape
Set Theme "nord"
Set Width 1300
Set Height 900

Hide
Type "clear; lib/share-demo" Enter
Sleep 3s
Show

Sleep 800ms
Down
Down
Down
Right
Type "480"
Enter
Sleep 600ms
Right
Right
Sleep 800ms
Ctrl+B
Up
Sleep 500ms
Down
Down
Right
Type "1300"
Enter
Sleep 1.5s
Screenshot out/stills/share-presence.png
Ctrl+K
Type@100ms "who's here"
Sleep 500ms
Enter
Sleep 2s
Escape
Sleep 500ms
Ctrl+B
Down
Sleep 500ms
Ctrl+Z
Sleep 2s
Screenshot out/stills/share-undo.png
Sleep 1s

Hide
Ctrl+B
Type ":kill-server"
Enter
Sleep 500ms
Type "lib/share-demo stop" Enter
Sleep 500ms
