---
title: "Serving over SSH"
sidebar_position: 3
---

# Serving over SSH

`012 serve` runs an SSH server that gives every session its own 012, in
the client's own terminal, on the files of one directory. It's for
reaching your sheets from another machine or a tablet with an SSH app,
without installing anything there. It is not shared editing: two
sessions are two separate spreadsheets. Shared viewing and editing over
`012 serve` are planned: see [toward multiplayer](../../ROADMAP.md#3-toward-multiplayer).

```sh
012 serve ~/sheets                 # serve ~/sheets on 127.0.0.1:2312
ssh -p 2312 localhost              # from this machine, on a new sheet
ssh -t -p 2312 localhost budget.012   # opening budget.012 from ~/sheets
```

A file name after the host opens that file, as `012 budget.012` does
locally: a `.012` sheet, a new sheet to be saved under that name when
there's no such file, or a file to import (`data.csv`). It must be one
word (quote a name with spaces twice: `ssh -t host "'my book.012'"`)
and needs `-t`, since ssh only asks for a terminal on its own when
there's no command.

```
012 serve [--listen addr] [--authorized-keys file] [--host-key file]
          [--idle-timeout 30m] [--max-sessions 8] [--log file] [--otlp url] [dir]
```

| Flag | Default | What |
|---|---|---|
| `dir` | `.` | The served directory. Sessions open, save, import and download only inside it |
| `--listen` | `127.0.0.1:2312` | Address to listen on. Loopback only unless you change it |
| `--authorized-keys` | `~/.ssh/authorized_keys` | Public keys allowed in, in OpenSSH's format |
| `--host-key` | `<config dir>/012/ssh_host_ed25519_key` | The server's key, generated on first run |
| `--idle-timeout` | `30m` | End a session after this long without input, keeping its unsaved changes; `0` never does |
| `--max-sessions` | `8` | Sessions at once; more are turned away with a message |
| `--log`, `--otlp` | off | Telemetry, as for the app ([Observability](../contributing/observability.md)) |

The config directory is `os.UserConfigDir()`: `~/Library/Application
Support` on macOS, `~/.config` on Linux. On start, 012 prints the served
directory, the address, the host key's fingerprint and the command to
connect.

## Setup

1. Put the public keys that may connect in an authorized_keys file. Your
   own `~/.ssh/authorized_keys` works, or keep a separate one for 012:

   ```sh
   cat ~/.ssh/id_ed25519.pub > ~/.config/012/authorized_keys
   chmod 600 ~/.config/012/authorized_keys
   012 serve --authorized-keys ~/.config/012/authorized_keys ~/sheets
   ```

2. Connect with `ssh -p 2312 host`. The first time, check the host key
   fingerprint ssh shows against the one 012 printed, or add the key to
   `known_hosts` yourself from `ssh_host_ed25519_key.pub` next to it.

3. To reach it from another machine, prefer keeping 012 on loopback and
   tunnelling through the machine's own sshd
   (`ssh -L 2312:127.0.0.1:2312 host`, then `ssh -p 2312 localhost`), or
   a private network such as Tailscale with `--listen 100.x.y.z:2312`.
   Listening beyond loopback prints a warning.

Stop the server with Ctrl+C. Running sessions end with it, keeping
their unsaved changes (see [Unsaved work](#unsaved-work)).

## Security model

The server exposes the files of one directory to whoever holds one of
the authorized keys. Everything else is closed.

- **Authentication.** Public keys only, checked against the
  authorized_keys file, which is read again on every login so edits
  apply at once. No passwords, no keyboard-interactive, no certificates.
  The file must not be writable by group or others, as with sshd's
  `StrictModes`. Keys carrying options 012 can't honor (`command=`,
  `from=`, `no-pty`, `restrict` without `pty`, `expiry-time=`,
  `cert-authority` and so on) are skipped, so a key restricted to, say,
  a backup command never gets a spreadsheet. The user name is logged but
  not checked.
- **Host key.** An ed25519 key generated on first run, written `0600`
  in a `0700` directory. 012 refuses to start if the key is readable by
  others.
- **What a session can do.** Run 012, nothing else; no command is
  executed, unless the server's config turns on notebooks' nushell
  commands (see [Notebooks](#notebooks)). A shell request starts 012 on a new sheet. An exec request
  is accepted only with a terminal (`ssh -t`) and only when it is one
  word, which is taken as a file name and never run: it resolves inside
  the served directory like a name typed in File > Open, and anything
  that doesn't (see Files below), or holds control characters, ends the
  session with the reason before 012 starts. Other exec requests
  (`ssh host cat /etc/passwd`, `ssh -t host sh -c id`, any exec without
  a terminal) are refused, as are subsystems (`sftp`, `scp`), local and
  remote port forwarding and X11; agent forwarding requests are
  accepted by the SSH library but never used. A shell without a
  terminal is told to use `ssh -t` and closed.
- **Files.** Names typed in File > Open, Save as, Import and Download,
  and a file name given on the ssh command line, resolve inside the
  served directory. Refused, with "outside the served directory":
  absolute paths, `..` that climbs out, hidden files and directories
  (`.env`, `.git`, `.ssh`, `.012-recovery`), and symbolic links that
  lead outside or nowhere. Links that stay inside work. Error messages
  name files relative to the served directory, not where it is on the
  server. Files are written with the server's user and permissions;
  recovery files are the one thing 012 writes that no one named, always
  in `.012-recovery/` (see [Unsaved work](#unsaved-work)).
- **JEV.** On only when the server process's own environment has
  `TYPESAFE_API_KEY`; sessions never read `.env` files. Every session
  has its own answer cache, and all of them spend the server's key.
- **Logs.** Logins, rejected keys, failed handshakes, sessions turned
  away and session ends go to stderr and to telemetry as `ssh.*` events
  with the user name, the key's SHA256 fingerprint and the remote
  address, never file contents or cells.

| Event | When | Attributes |
|---|---|---|
| `ssh.session` | a session starts | `user`, `key`, `remote`, `term`, `width`, `height`, `file` (a file was named on the command line; the name isn't logged) |
| `ssh.session_end` | it ends | `user`, `key`, `remote`, `duration`, `idle` (closed for idling), `stopped` (the server stopped), `recovered` (unsaved work was kept) |
| `ssh.rejected` | a key not in authorized_keys | `user`, `key`, `remote` |
| `ssh.full` | a session turned away at `--max-sessions` | `user`, `key`, `remote` |
| `ssh.failed` | a handshake or login fails | `remote`, `error` |
| `ssh.auth_error` | authorized_keys can't be read or parsed | `error` |

The `frames` summaries and the `ssh_sessions` gauge cover every session
together: telemetry is per process.

What it doesn't protect against: someone who can already write to the
served directory on the server (a symbolic link swapped in between the
check and the open), and anything a holder of an authorized key does
with the files they're given.

## Each session

Every session is a separate 012 with nothing shared but the process:
its own workbook, undo history, clipboard, theme, JEV cache and
terminal state. What it learns about the terminal comes from the
client: the window size and its changes, the colors (from the `TERM`
the client sends), the light or dark background, kitty or sixel graphics, and
the clipboard, which Ctrl+C sets on the client's machine through OSC 52.

[Macros](../sheets/macros.md) can be recorded, run, renamed and deleted in a
session, but not edited as scripts: that would start an editor on the
server. A file's macros ask for trust once per session, since the
session isn't the server's own computer, and scripts have no file,
network or clock access in any case.

## Notebooks

A session opens [notebooks](../nushell/notebooks.md) and shows their
regions, but runs none of their commands: a command is a program on the
server, with the server's user and every file it can reach, well
beyond the served directory. `serve-shell = on` in the server's config
lets sessions run them, as the user 012 serve runs as, following the
`shell` option as the local app does; a file's commands still ask once
per session before they run. Only turn it on when everyone holding an
authorized key may run programs on the server.

## Unsaved work

A session that ends without the user quitting, because it was idle for
`--idle-timeout`, because the server is stopping (Ctrl+C, SIGTERM), or
because 012 [stopped on an internal error](../files/saving.md#if-012-crashes)
(whose report goes to the server's log), keeps its unsaved changes: the whole workbook is written to
`.012-recovery/<name>-<time>.012` in the served directory, and the
client's terminal is told where before the connection closes:

```
012: closed after 30m0s without input
012: unsaved changes kept in .012-recovery/budget-20260927-140203.012; open budget.012 again to restore them
```

The next session opening `budget.012` (from File > Open or
`ssh -t host budget.012`) asks on the context line: Enter restores them
as unsaved changes to `budget.012`, D deletes them, Esc leaves them for
next time. Saving a restored workbook removes its recovery file, and
saving over a file that changed on disk still asks first. A sheet never
saved is kept as `(untitled)-<time>.012` and offered when a session
starts on a new sheet.

It stays small: nothing is kept for a workbook without changes or with
no cells, and only the newest three files per name are kept. The
directory is created `0700` and its files `0600`, and it's hidden, so no
name typed in a session reaches it; only 012 itself reads and writes
it. A session whose client just goes away (a dropped connection)
keeps nothing, and neither does quitting, which asks about unsaved
changes as the local app does.

## Two sessions, one file

Opening the same file in two sessions gives two copies. Saving over a
file that changed on disk since it was opened or last saved in this
session (someone else saved it) asks first on the context line:
Enter overwrites it, S saves under another name, Esc cancels. The same
check protects the local app from other programs writing the file.

## Limits

- An idle session is closed without saving over its file; unsaved
  changes go to a recovery file instead.
- A dropped connection loses unsaved changes.
- Each session holds its screen's buffers, about a megabyte, plus the
  sheets it opens; two sessions opening one file hold two copies. Fifty
  sessions typing at once get their frames as fast as ten do: see
  [Bounds of support](../contributing/limits.md#serving-over-ssh) for
  the measurements.
