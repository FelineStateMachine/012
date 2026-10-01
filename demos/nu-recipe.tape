# A notebook recipe: nushell cleans a CSV's money text into numbers, a
# second cell totals it by region, and Insert > Chart on its output's
# grid sends the output to a sheet and charts the totals there. Cells run with Esc then r, as the
# terminal (ttyd) sends Shift+Enter as Enter. Needs nu on the PATH.
Output out/nu-recipe.gif
Source lib/setup.tape
Set Theme "tokyonight"

Hide
Type "../bin/012 nu" Enter
Sleep 2s
Show

Sleep 800ms
Type@35ms `sales = open data/sales.csv | update Revenue { str replace -ar '[$,]' '' | into float }`
Sleep 500ms
Escape
Type "r"
Sleep 2s
Type "b"
Enter
Type@35ms "totals = $sales | group-by Region --to-table | update items { get Revenue | math sum } | rename Region Revenue"
Sleep 500ms
Escape
Type "r"
Sleep 2s
Screenshot out/stills/nu-recipe-cells.png
Down
Enter
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
