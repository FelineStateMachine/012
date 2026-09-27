# Serving over SSH

`012 serve` runs an SSH server that gives every session its own 012, in
the client's own terminal, on the files of one directory. It's for
reaching your sheets from another machine or a tablet with an SSH app,
without installing anything there. It is not shared editing: two
sessions are two separate spreadsheets (see
[the roadmap](../ROADMAP.md#later-sharing-a-live-sheet-shelved)).

```sh
012 serve ~/sheets                 # serve ~/sheets on 127.0.0.1:2312
ssh -p 2312 localhost              # from this machine
```

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
| `--idle-timeout` | `30m` | End a session after this long without input; `0` never does |
| `--max-sessions` | `8` | Sessions at once; more are turned away with a message |
| `--log`, `--otlp` | off | Telemetry, as for the app ([observability.md](observability.md)) |

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

Stop the server with Ctrl+C. Running sessions end with it, and unsaved
changes in them are lost.

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
- **What a session can do.** Run 012, nothing else. Exec requests
  (`ssh host cat /etc/passwd`), subsystems (`sftp`, `scp`), local and
  remote port forwarding and X11 are refused; agent forwarding requests
  are accepted by the SSH library but never used. A session without a
  terminal is told to use `ssh -t` and closed.
- **Files.** Names typed in File > Open, Save as, Import and Download
  resolve inside the served directory. Refused, with "outside the served
  directory": absolute paths, `..` that climbs out, hidden files and
  directories (`.env`, `.git`, `.ssh`), and symbolic links that lead
  outside or nowhere. Links that stay inside work. Error messages name
  files relative to the served directory, not where it is on the
  server. Files are written with the server's user and permissions.
- **JEV.** On only when the server process's own environment has
  `TYPESAFE_API_KEY`; sessions never read `.env` files. Every session
  has its own answer cache, and all of them spend the server's key.
- **Logs.** Logins, rejected keys, failed handshakes, sessions turned
  away and session ends go to stderr and to telemetry as `ssh.*` events
  with the user name, the key's SHA256 fingerprint and the remote
  address, never file contents or cells.

| Event | When | Attributes |
|---|---|---|
| `ssh.session` | a session starts | `user`, `key`, `remote`, `term`, `width`, `height` |
| `ssh.session_end` | it ends | `user`, `key`, `remote`, `duration`, `idle` (closed for idling) |
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
the client sends), the light or dark background, kitty graphics, and
the clipboard, which Ctrl+C sets on the client's machine through OSC 52.

[Macros](macros.md) can be recorded, run, renamed and deleted in a
session, but not edited as scripts: that would start an editor on the
server. A file's macros ask for trust once per session, since the
session isn't the server's own computer, and scripts have no file,
network or clock access in any case.

## Two sessions, one file

Opening the same file in two sessions gives two copies. Saving over a
file that changed on disk since it was opened or last saved in this
session (someone else saved it) asks first on the context line:
Enter overwrites it, S saves under another name, Esc cancels. The same
check protects the local app from other programs writing the file.

## Limits

- Sessions start on a new sheet; there's no way to open a file from the
  ssh command line, since commands are refused.
- An idle session is closed without saving.
- Each session holds about 1.3 MiB of heap on a new sheet, plus about
  300 B per cell of the sheets it opens; two sessions opening one file
  hold two copies. With 50 sessions typing at once on loopback, frames
  still arrive within one frame interval (p95 16.6 ms), the same as with
  10: see [limits.md](limits.md#serving-over-ssh).
