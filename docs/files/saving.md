---
title: "Saving"
sidebar_position: 3
---

# Saving

Saves are atomic: 012 writes a temporary file and renames it. Saving over
the open file when something else wrote it since it was opened or last
saved (another program, or another session of [012 serve](../terminal/ssh.md)) asks
first: Enter overwrites, S saves under another name, Esc cancels. Save
as (and `:w name`, `:wq name` with vim keys) onto another file that
exists asks the same way: Enter replaces it, Esc cancels, and cancelling
`:wq`'s question cancels its quit too.

Under `012 serve`, a session that idles out or is ended by the server
stopping keeps its unsaved changes in `.012-recovery/` in the served
directory, and the next session opening that file offers them back: see
[Serving over SSH](../terminal/ssh.md#unsaved-work).

## If 012 crashes

An internal error (a bug, a Go panic) stops 012 rather than leaving
the terminal broken: the screen goes back to what the shell showed,
and 012 says what happened and where it put things:

```
012: stopped on an internal error (a panic in update: ...)
012: unsaved changes kept in ~/.config/012/recovery/%2FUsers%2Fann%2Fbudget-20260929-101112.012; open budget.012 again to restore them
012: a report is in ~/.config/012/crashes/crash-20260929-101112.txt; include it when reporting the problem
```

- **Unsaved changes** are written, the whole workbook, to `recovery/`
  in 012's config directory (the one `012 config path` prints the
  [config file](../reference/config.md) of, `~/.config/012` on Linux),
  named after the file's full path. Opening that
  file again, or starting `012` on nothing for a sheet never saved,
  asks on the context line: Enter restores them as unsaved changes to
  the file, D deletes them, Esc leaves them for next time. Saving a
  restored workbook removes its recovery file. Nothing is kept without
  unsaved changes, and only the newest three files per name.
- **The report** goes to `crashes/` beside it: where the error
  happened, the error, its stack of calls, 012's version, the system,
  the file's name and where its changes went, never what the workbook
  holds. The newest ten are kept.

If the error happened while drawing the screen, the terminal also
shows the stack of calls above those lines. Under `012 serve`, the
session's changes go to `.012-recovery/` as above, the client is told
that 012 stopped on an internal error, and the report goes to the
server's log.
