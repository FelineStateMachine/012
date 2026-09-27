# Vim keys (File > Settings > Vim keys, or keymap = vim in the config
# file): the mode indicator says NORMAL, hjkl and counts move, dd cuts a
# row and p puts it back, / finds, and the : line runs any command with
# completions. O12_KEYMAP turns them on here so the recording leaves the
# config file alone.
Output out/vim.gif
Source lib/setup.tape

Hide
Type "O12_KEYMAP=vim ../bin/012 data/inventory.csv" Enter
Sleep 2s
Show

Sleep 1s
Type@400ms "5jll"
Sleep 800ms
Type "dd"
Sleep 1.2s
Type@300ms "3j"
Type "p"
Sleep 1.5s
Type "/"
Sleep 400ms
Type@150ms "acme"
Enter
Sleep 800ms
Type@500ms "nn"
Sleep 800ms
Type "V"
Type@300ms "2j"
Sleep 1s
Screenshot out/stills/vim-visual.png
Escape
Sleep 500ms
Type ":"
Sleep 400ms
Type@150ms "freeze 1 row"
Sleep 1.2s
Screenshot out/stills/vim-command.png
Enter
Sleep 1s
Type "G"
Sleep 1.5s
Type "gg"
Sleep 1.5s
