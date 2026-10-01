# Linked sources: ten million rows of Parquet (made by gensales) linked
# as a source (Data > Linked file > Link a source, here from the
# palette), sorted by amount Z to A in the background, its rows
# numbered past the grid's last, and read whole by formulas on Sheet1:
# a count grouped, an AVERAGEIFS, the last day as a date, and a MEDIAN of
# every amount, sorted on disk in the background.
Output out/sources.gif
Source lib/setup.tape
Set Theme "GruvboxDark"

Hide
Type "go run ./gensales out/sales.parquet && clear && ../bin/012" Enter
Wait+Screen@90s /Help/
Show

Sleep 500ms
Ctrl+K
Type@80ms "link a source"
Sleep 400ms
Enter
Sleep 500ms
Type@40ms "out/sales.parquet"
Enter
Sleep 800ms
Down
Up
Sleep 1s
Screenshot out/stills/sources-linked.png
Sleep 400ms
Right 2
Ctrl+K
Type@80ms "sort sheet z to a"
Sleep 400ms
Enter
Wait+Screen@30s /sorted by amount Z to A/
Sleep 1.2s
Screenshot out/stills/sources-sorted.png
Sleep 400ms
Ctrl+K
Type@80ms "previous sheet"
Sleep 400ms
Enter
Sleep 400ms
Type@8ms "Rows"
Tab
Type@8ms "=ROWS(sales)"
Enter
Type@8ms "North"
Tab
Type@8ms `=AVERAGEIFS(sales[amount],sales[cat],"north")`
Enter
Type@8ms "Last day"
Tab
Type@8ms "=MAX(sales[day])"
Enter
Type@8ms "Median"
Tab
Type@8ms "=MEDIAN(sales[amount])"
Enter
Up
Right
Wait+Screen@60s /Median +[0-9]/
Sleep 1.5s
Screenshot out/stills/sources-formulas.png
Sleep 1s
