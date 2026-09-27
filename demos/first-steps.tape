# First steps: type like in Sheets. Tab moves right, Enter goes down and
# back to the column where the Tabs began, = starts a formula, the arrows
# point at cells while typing one, and totals follow their inputs.
Output out/first-steps.gif
Source lib/setup.tape

Hide
Type "../bin/012" Enter
Sleep 1.5s
Show

Sleep 800ms
Type "Item" Tab Type "Price" Tab Type "Qty" Tab Type "Total" Enter
Type "Coffee" Tab Type "$4.35" Tab Type "3" Tab
Type "=" Sleep 300ms Left@400ms 2 Type "*" Sleep 300ms Left
Sleep 700ms
Enter
Type "Bagel" Tab Type "$2.80" Tab Type "2" Tab Type "=B3*C3" Enter
Type "Juice" Tab Type "$3.90" Tab Type "4" Tab Type "=B4*C4" Enter
Type "Total" Tab@300ms 3
Type "=SUM(D2:D4)"
Sleep 700ms
Enter
Sleep 800ms
Up@300ms 3 Left
Type "6" Enter
Sleep 3s
Screenshot out/stills/first-steps-totals.png
Sleep 500ms
