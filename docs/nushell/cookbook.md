---
title: "Cookbook"
sidebar_position: 5
---

# Cookbook

Short worked examples. Pipelines that end in `^012` run in a nushell
session; lines of the form `name = pipeline` are typed at a
[notebook's](notebooks.md) `nu❯` prompt (`012 nu`, or `!` in any workbook). Nushell's own
[cookbook](https://www.nushell.sh/cookbook/) has more pipelines to start
from.

## A CSV cleaned, then summarized

Money written as text (`"$2,252.50"`) becomes numbers in nushell, and a
second region totals it by region, with a chart that follows it.

![Nushell turns a CSV's money text into numbers, a second region totals it by region, and a column chart of the totals follows the table](../media/nu-recipe.gif)

```nu
sales = open sales.csv | update Revenue { str replace -ar '[$,]' '' | into float }
totals = $sales | group-by Region --to-table | update items { get Revenue | math sum } | rename Region Revenue
```

To chart it, select the totals table, its header row and rows, and
**Insert > Chart**. Change `sales`'s command (F2 on its label) to read
another file, and `totals` runs again after it and the chart redraws.

To pivot rather than total, select the `sales` table and
**Data > Pivot table** ([Pivot tables](../sheets/pivots.md)): Region in
rows, Quarter in columns, Revenue in values.

Nushell:
[`update`](https://www.nushell.sh/commands/docs/update.html),
[`str replace`](https://www.nushell.sh/commands/docs/str_replace.html),
[`into float`](https://www.nushell.sh/commands/docs/into_float.html),
[`group-by`](https://www.nushell.sh/commands/docs/group-by.html),
[`math sum`](https://www.nushell.sh/commands/docs/math_sum.html).

## A log file followed live

A JSON lines log (`app.jsonl`) linked with **Data > Linked file > Link a
table** takes in rows as they're written ([Following
files](../files/following.md)). A region reads it as `$app`:

![A JSON lines log grows in a linked region; a shell region keeps its errors, and refreshing it later picks up the errors written since](../media/follow-log.gif)

```nu
errors = $app | where status >= 500 | select time path status ms
by_path = $errors | group-by path --to-table | update items { length } | rename path errors
```

The linked region grows by itself; Enter on `errors`'s label reads the
rows the log has now, and `by_path` follows. A log in CSV or TSV works
the same; a plain text log needs a line of nushell to become a table,
such as [`parse`](https://www.nushell.sh/commands/docs/parse.html) in a
region's command:

```nu
lines = open --raw app.log | lines | parse "{time} {level} {message}"
```

## Disk usage with a chart

```nu
usage = du * | select path physical | update path { path basename } | sort-by physical --reverse
```

Sizes arrive in the Size format ([Types](types.md)), so a chart's axis
and `=SUM(nu.usage)` read them as bytes. Select the table and
**Insert > Chart** for a column chart; Enter on the label runs `du`
again.

Nushell: [`du`](https://www.nushell.sh/commands/docs/du.html).

## An API response as a table

```nu
http get https://api.github.com/repos/nushell/nushell/issues | select number title user.login comments created_at | update created_at { into datetime } | to nuon | ^012 -
```

Sort by comments, filter by author, or make a pivot table of issues by
author, in the sheet. In a notebook, the same pipeline without
`| to nuon | ^012 -` is a region that fetches the list again when
refreshed; `nu-timeout` stops a request that hangs.

Nushell: [`http get`](https://www.nushell.sh/commands/docs/http_get.html),
[`into datetime`](https://www.nushell.sh/commands/docs/into_datetime.html),
and the cookbook's [HTTP](https://www.nushell.sh/cookbook/http.html)
page.

## Processes, sorted, filtered and sent back

```nu
ps | where cpu > 1 | sort-by cpu --reverse | select pid name cpu mem | to nuon | ^012 --pipe | from nuon | get pid | each {|pid| kill $pid }
```

In 012, sort the list and filter it down to the processes to stop
(**Data > Create a filter**); Ctrl+Q, Enter sends the rows the filter
shows, and nushell stops those and no others. D sends nothing, and the
pipeline stops there ([The exit status](pipelines.md#the-exit-status)).

Nushell: [`ps`](https://www.nushell.sh/commands/docs/ps.html),
[`kill`](https://www.nushell.sh/commands/docs/kill.html).

## Git history as a sheet

```nu
commits = git log --pretty=%h»¦«%an»¦«%s»¦«%aI -n 500 | lines | split column "»¦«" commit author subject date | update date { into datetime }
authors = $commits | group-by author --to-table | update items { length } | rename author commits | sort-by commits --reverse
```

`date` is a datetime, so a filter by date or a pivot table by month
works on it in the sheet. The pattern is the one nushell's cookbook
explains in [Parsing git
log](https://www.nushell.sh/cookbook/parsing_git_log.html).
