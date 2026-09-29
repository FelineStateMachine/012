# Tracing: with precedents and dependents shown, the pointer on North's
# revenue marks the regions and revenues it reads and the price per unit
# that reads it; then Evaluate formula steps through the price, into
# the revenue it reads and back out.
Output out/tracing.gif
Source lib/setup.tape

Hide
Type "../bin/012 data/sales.csv" Enter
Sleep 2s
Ctrl+G
Type "F1" Enter
Type "North" Enter
Type@40ms '=SUMIF(A2:A17,"North",D2:D17)' Enter
Type "Units" Enter
Type@40ms '=SUMIF(A2:A17,"North",C2:C17)' Enter
Type "Per unit" Enter
Type@40ms "=F2/F4" Enter
Show

Sleep 800ms
Ctrl+K
Type@60ms "show precedents"
Sleep 800ms
Enter
Sleep 3s
Ctrl+G
Type "F2" Enter
Sleep 3s
Screenshot out/stills/tracing-marks.png
Down@600ms 2
Sleep 1.5s
Down@600ms 2
Sleep 2s
Ctrl+K
Type@60ms "evaluate formula"
Sleep 800ms
Enter
Sleep 1.5s
Right
Sleep 1.5s
Screenshot out/stills/tracing-into.png
Enter
Sleep 1.2s
Left
Sleep 1.5s
Screenshot out/stills/tracing-evaluate.png
Enter
Sleep 1.2s
Enter
Sleep 2s
Screenshot out/stills/tracing-value.png
Escape
Sleep 1s
