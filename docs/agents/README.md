---
title: "Agents"
sidebar_position: 1
---

# Agents

Coding agents and assistants work on `.012` workbooks through the same
operations people use: every change is typed into cells as the screen
types it, checked by the same rules, made as one change and
shown by [`012 diff`](../files/git.md#012-diff) like anyone's. Nothing
agents do runs a program or reaches the network unless they were
started with the flags that allow it, as for scripts.

| Page | For |
|---|---|
| This page | Agents with a shell: the commands, their JSON, and the Claude Code skill |
| [MCP server](mcp.md) | `012 mcp`, for any MCP host: tools, resources, prompts, views in the chat, and adding it to Claude Code, Codex and Claude Desktop with `012 agent --install-mcp` |
| [JSON output](../reference/json.md) | The schemas of what commands write with `--format json`, and what the MCP tools return |

## With a shell

Agents that can run commands use the ones [scripts](../files/scripts.md)
use, in this order:

```sh
012 describe book.012 --format json                  # what's where
012 get book.012 Sales --format json                 # read what describe pointed at
012 set book.012 D2 '=B2*C2' D3 '=B3*C3' --dry-run   # preview a change as a diff
012 set book.012 D2 '=B2*C2' D3 '=B3*C3'             # make it: one change, saved atomically
012 recalc book.012 --format json                    # check no formula shows an error
012 export book.012 report.html                      # share it as a web page
```

`012 describe` lists each sheet with its used range, the row that looks
like its header and the column names in it, frozen panes, filters,
tables, notebook outputs and linked files sent to it, and charts
(numbered, as `012 export --chart` names them); pivot tables with their
source; notebook tabs with their cells and whether each ran; and the
workbook's named ranges:

```
Sales (shown)  A1:F9, 9 rows x 6 columns, 54 cells, 9 formulas
  header row 1: Date, Region, Units, Price, Total, Share
  frozen 1 row, 0 columns
  table Orders  A1:F9  Date, Region, Units, Price, Total, Share
  chart 1  Units by date  column of A1:C9 at H2
Notebook (notebook)  2 cells
  1 code  files = ls  (ran)
  2 code  $files | where size > 1kb  (not run)
names
  Rate  Sales!B12
```

A header row is guessed: the first row with contents, when every cell
in it is text and the rows below hold numbers, dates, booleans or
formulas, or when it's bold. `--format json` gives the same as the
[schema](../reference/json.md#describe).

`012 set --dry-run` checks the entries as `set` does (a formula that
doesn't parse, a validation rule, a protected range stop it the same
way) and prints what would change, as `012 diff` would, without saving;
`--format json` gives the changes as records instead. Every command's
flags are in [Command line](../reference/command-line.md).

## The Claude Code skill

012 carries a [Claude Code skill](https://docs.claude.com/en/docs/claude-code/skills):
a `SKILL.md` that tells Claude when to reach for `012` and how to use
it well (describe first, preview with `--dry-run`, `recalc` after
changing formulas, never edit the file's JSON by hand).

```sh
012 agent --install-skill                          # into ~/.claude/skills/012, for every project
012 agent --install-skill .claude/skills/012       # into this project, to commit with it
012 agent --skill                                  # print it
```

Installing over a file that isn't this 012's skill asks for `--force`;
installing the same skill again says it's up to date. A newer 012 may
carry a newer skill: install it again after upgrading.

## Trust

Agents get no more than scripts do. A workbook's
[notebook](../nushell/notebooks.md) cells run only with `--notebooks`,
and then only when the workbook was saved on this computer or
`--trust` says so; [JEV functions](../formulas/jev.md) ask the network
only with `--jev`. Protected ranges refuse changes unless `--force` is
given. Leave these flags to the person asking.
