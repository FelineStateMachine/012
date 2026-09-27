# Decimal arithmetic: binary floating point makes $4.35 come to 434
# whole cents and $0.10 + $0.20 miss 0.3. File > Settings > Decimal
# arithmetic, here from the palette, makes this file's money math exact.
Output out/decimal.gif
Source lib/setup.tape

Hide
Type "../bin/012" Enter
Sleep 1.5s
Show

Sleep 600ms
Type "Coffee" Tab Type "$4.35" Enter
Type "Tip" Tab Type "$0.10" Enter
Type "Fee" Tab Type "$0.20" Enter
Type "Cents" Tab Type "=INT(B1*100)" Enter
Type "Is 0.30?" Tab Type "=B2+B3=0.3" Enter
Type "Residue" Tab Type "=0.1+0.2-0.3" Enter
Sleep 1.5s
Ctrl+K
Type@120ms "decimal"
Sleep 1s
Enter
Sleep 2s
Screenshot out/stills/decimal-on.png
Alt+f
Sleep 500ms
Up@400ms 2
Sleep 500ms
Right
Sleep 2s
Screenshot out/stills/settings-menu.png
Escape@300ms 2
Sleep 1s
