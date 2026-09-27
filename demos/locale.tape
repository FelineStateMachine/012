# Locale: a small budget typed in English (United States), then File >
# Settings > Locale, here from the palette, shows it as Germany writes
# it: decimal commas, day-first dates, euros after the number and ; in
# formulas. The values don't change; the file stores the same thing.
Output out/locale.gif
Source lib/setup.tape

Hide
Type "../bin/012" Enter
Sleep 1.5s
Show

Sleep 600ms
Type "Item" Tab Type "Amount" Tab Type "Due" Enter
Type "Rent" Tab Type "$950.00" Tab Type "9/26/2026" Enter
Type "Power" Tab Type "$84.20" Tab Type "10/3/2026" Enter
Type "Total" Tab Type "=SUM(B2:B3)" Enter
Type "VAT" Tab Type "=ROUND(B4*0.19, 2)" Enter
Sleep 1.5s
Ctrl+K
Type@120ms "locale"
Sleep 1s
Enter
Sleep 1s
Type@120ms "germany"
Sleep 1.5s
Enter
Sleep 1s
Up Right
Sleep 2.5s
Screenshot out/stills/locale-de.png
Sleep 500ms
