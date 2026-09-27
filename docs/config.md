# Configuration

012 reads one settings file, in the style of Ghostty's:

| OS | File |
|---|---|
| Linux, BSD | `$XDG_CONFIG_HOME/012/config`, usually `~/.config/012/config` |
| macOS | `~/Library/Application Support/012/config`, or `$XDG_CONFIG_HOME/012/config` when that's set |
| Windows | `%AppData%\012\config`, or `%XDG_CONFIG_HOME%\012\config` when that's set |

```
# ~/.config/012/config
theme = light:Builtin Solarized Light,dark:Dracula
chart-images = true
jev-api-key-command = op read op://Private/TypeSafe/credential
config-file = ?local.conf
```

- One `key = value` per line. `#` starts a comment line; blank lines are
  ignored. Quote a value (`"  like this  "`) to keep spaces at its ends.
- An empty value (`theme =`) puts an option back to its default.
- `config-file = path` reads another file after this one, relative to this
  file's directory; its settings win over the ones around it. A leading
  `?` (`config-file = ?local.conf`) makes it optional. Includes can nest;
  a file that includes itself is skipped with a warning.
- Unknown keys, bad values and missing includes never stop 012: it starts
  with the defaults for those options and says what's wrong on the context
  line (and in `012 config`).

Settings are process-wide. Settings that belong to a workbook, such as
decimal arithmetic, are saved in the workbook's `.012` file instead.

**Precedence:** command-line flags win over environment variables, which
win over the config file, which wins over the defaults. `012 config` shows
where each value came from.

**Secrets never go in this file.** The JEV API key lives in the OS
credential store or comes from a command; see [JEV setup](jev.md#setup).

## Commands

| Command | Does |
|---|---|
| `012 config` | print the settings in effect, each with where it came from (file and line, variable, flag or default), secrets redacted, then any warnings |
| `012 config path` | print the config file's path |
| `012 config edit` | open the file in `$VISUAL` or `$EDITOR`, creating it with every option commented out if it doesn't exist |
| `012 config default` | print a config file with every option at its default, commented out and described |
| `012 config themes` | list the themes: built in and in the themes directory |
| `012 config set-key` | store the JEV API key in the credential store, read from the terminal without echo or from stdin |
| `012 config delete-key` | remove the JEV API key from the credential store |

In the app, File > Settings has **Theme** (a picker with live preview),
**Open config file** (in your editor, reloading when it closes),
**Reload config** and **JEV API key**. Reloading applies the options marked
"File > Settings > Reload config" below; the rest take effect on the next
start.

## Options

<!-- Generated from internal/config/registry.go by `go test ./internal/config -update-docs`. Don't edit below. -->

### Appearance

#### `theme`

Colors. `terminal` uses the terminal's own 16-color palette. Any other name is a terminal color scheme, built in (`012 config themes` lists them) or a file in the themes directory, drawn in its own colors with solid menu and status bars. `light:NAME,dark:NAME` picks one by the terminal's background and follows it when it changes.

| | |
|---|---|
| Type | text |
| Default | `terminal` |
| Environment | `O12_THEME` |
| Flag | `--theme` |
| Applies | File > Settings > Reload config |

#### `chart-images`

Draw charts as images on terminals with kitty graphics (kitty, Ghostty, WezTerm). When false, charts are always text.

| | |
|---|---|
| Type | true or false |
| Default | `true` |
| Environment | `O12_CHART_IMAGES` |
| Applies | File > Settings > Reload config |

#### `notifications`

Send a desktop notification (OSC 9) when JEV answers or an import finishes while the terminal window is in the background.

| | |
|---|---|
| Type | true or false |
| Default | `true` |
| Environment | `O12_NOTIFICATIONS` |
| Applies | File > Settings > Reload config |

#### `keymap`

Keys in the grid. `default` works like Google Sheets; `vim` adds hjkl, counts, operators, visual selection and a : command line (File > Settings > Vim keys).

| | |
|---|---|
| Type | one of `default`, `vim` |
| Default | `default` |
| Environment | `O12_KEYMAP` |
| Applies | File > Settings > Reload config |

### JEV functions

#### `jev-api-key-command`

A command that prints the TypeSafe API key, used when TYPESAFE_API_KEY isn't set and the credential store has no key, e.g. `op read op://Private/TypeSafe/credential` or `pass show typesafe`. It runs the first time a sheet asks JEV something, without a shell, for up to 10 seconds, and its first line of output is the key; for pipes, write `sh -c '...'` yourself. The key itself never goes in this file.

| | |
|---|---|
| Type | command |
| Default | (empty) |
| Applies | restart 012 |

#### `jev-credential-store`

Look for the API key in the OS credential store (macOS Keychain, Windows Credential Manager, the Secret Service on Linux), where `012 config set-key` and Settings put it.

| | |
|---|---|
| Type | true or false |
| Default | `true` |
| Environment | `O12_JEV_CREDENTIAL_STORE` |
| Applies | restart 012 |

#### `jev-base-url`

The TypeSafe service to ask; the SDK's default when empty. Must be https, except on localhost.

| | |
|---|---|
| Type | URL |
| Default | (empty) |
| Environment | `TYPESAFE_BASE_URL` |
| Applies | restart 012 |

#### `jev-model`

The JEV model to ask; the service's default when empty.

| | |
|---|---|
| Type | text |
| Default | (empty) |
| Environment | `TYPESAFE_DEFAULT_MODEL` |
| Applies | restart 012 |

### Telemetry

#### `log-file`

Append telemetry events to this JSON log file. See docs/observability.md.

| | |
|---|---|
| Type | path |
| Default | (empty) |
| Environment | `O12_LOG` |
| Flag | `--log` |
| Applies | restart 012 |

#### `log-level`

The least severe telemetry event recorded; debug adds every frame and command.

| | |
|---|---|
| Type | one of `debug`, `info`, `warn`, `error` |
| Default | `info` |
| Environment | `O12_LOG_LEVEL` |
| Applies | restart 012 |

#### `otlp-endpoint`

Send telemetry to this OTLP/HTTP collector, e.g. http://localhost:4318. The other OTEL_* variables still apply.

| | |
|---|---|
| Type | URL |
| Default | (empty) |
| Environment | `OTEL_EXPORTER_OTLP_ENDPOINT` |
| Flag | `--otlp` |
| Applies | restart 012 |

### 012 serve

#### `serve-listen`

The address 012 serve listens on. Anything but the loopback address lets other machines reach it (with an authorized key). See docs/ssh.md.

| | |
|---|---|
| Type | host:port |
| Default | `127.0.0.1:2312` |
| Applies | restart 012 |

#### `serve-authorized-keys`

The public keys allowed to log in to 012 serve, in OpenSSH's authorized_keys format.

| | |
|---|---|
| Type | path |
| Default | `~/.ssh/authorized_keys` |
| Applies | restart 012 |

#### `serve-host-key`

012 serve's private host key, generated when missing; ssh_host_ed25519_key in the config directory when empty.

| | |
|---|---|
| Type | path |
| Default | (empty) |
| Applies | restart 012 |

#### `serve-idle-timeout`

End a 012 serve session that has had no input for this long; 0 never does.

| | |
|---|---|
| Type | duration |
| Default | `30m` |
| Applies | restart 012 |

#### `serve-max-sessions`

How many 012 serve sessions may run at once; more are turned away.

| | |
|---|---|
| Type | number |
| Default | `8` |
| Applies | restart 012 |

### Config files

#### `config-file`

Read another config file after this one, relative to this file's directory. A leading `?` makes it optional: no warning when it doesn't exist.

| | |
|---|---|
| Type | path |
| Default | (empty) |
| Repeatable | yes |
| Applies | restart 012 |
