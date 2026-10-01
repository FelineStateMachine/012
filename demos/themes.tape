# Themes: File > Settings > Theme (here from the palette) lists the
# built-in color schemes; the highlighted one is drawn live as you move,
# typing narrows the list, and Esc goes back to the theme you had (Enter
# would keep it in the config file).
Output out/themes.gif
Source lib/setup.tape
Set Theme "nord"

Hide
Type "../bin/012 data/spending.csv" Enter
Sleep 2s
Show

Sleep 800ms
Ctrl+K
Type@100ms "theme"
Sleep 600ms
Enter
Sleep 1.2s
Type@150ms "dracula"
Sleep 1.5s
Screenshot out/stills/theme-dracula.png
Backspace@60ms 7
Type@150ms "1-2-3"
Sleep 2s
Screenshot out/stills/theme-classic.png
Backspace@60ms 5
Type@150ms "latte"
Sleep 2s
Screenshot out/stills/theme-light.png
Escape
Sleep 1.5s
