# A notebook recipe: nushell cleans a CSV's money text into numbers,
# a second region totals it by region, and a chart of the totals
# follows the table. Needs nu on the PATH.
Output out/nu-recipe.gif
Source lib/setup.tape

Hide
Type "../bin/012 nu" Enter
Sleep 2s
Show

Sleep 800ms
Type@35ms `sales = open data/sales.csv | update Revenue { str replace -ar '[$,]' '' | into float }`
Sleep 500ms
Enter
Sleep 2s
Type@35ms "totals = $sales | group-by Region --to-table | update items { get Revenue | math sum } | rename Region Revenue"
Sleep 500ms
Enter
Sleep 2s
Screenshot out/stills/nu-recipe-regions.png
Escape
Sleep 400ms
Ctrl+G
Type "A21:B25" Enter
Sleep 800ms
Ctrl+K
Type@120ms "chart"
Sleep 800ms
Enter
Sleep 1.5s
Enter
Sleep 800ms
Escape
Sleep 2.5s
Screenshot out/stills/nu-recipe-chart.png
Sleep 1s
