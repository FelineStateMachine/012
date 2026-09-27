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
[ssh.md](../terminal/ssh.md#unsaved-work).
