---
title: "Configuration"
sidebar_position: 4
---

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
decimal arithmetic and its locale, are saved in the workbook's `.012` file
instead; `locale` here is the default for workbooks without one.

**Precedence:** command-line flags win over environment variables, which
win over the config file, which wins over the defaults. `012 config` shows
where each value came from.

**Secrets never go in this file.** The JEV API key lives in the OS
credential store or comes from a command; see [JEV setup](../formulas/jev.md#setup).

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
**Reload config** and **JEV API key** (stored, then checked with one test
call: see [JEV functions](../formulas/jev.md#setup)). Reloading applies the options marked
"File > Settings > Reload config" below; the rest take effect on the next
start.

## Options

<!-- Generated from internal/config/registry.go by `go test ./internal/config -update-docs`. Don't edit below. -->

| Option | Default | Environment, flag |
|---|---|---|
| [`theme`](#theme) | `terminal` | `O12_THEME`, `--theme` |
| [`chart-images`](#chart-images) | `true` | `O12_CHART_IMAGES` |
| [`notifications`](#notifications) | `true` | `O12_NOTIFICATIONS` |
| [`keymap`](#keymap) | `default` | `O12_KEYMAP` |
| [`locale`](#locale) | `en-US` | `O12_LOCALE` |
| [`max-cells`](#max-cells) | `10000000` | `O12_MAX_CELLS` |
| [`shell`](#shell) | `ask` | `O12_SHELL` |
| [`nu-timeout`](#nu-timeout) | `30s` | `O12_NU_TIMEOUT` |
| [`nu-config`](#nu-config) | `false` | `O12_NU_CONFIG` |
| [`nu-save-cell-kb`](#nu-save-cell-kb) | `1024` | `O12_NU_SAVE_CELL_KB` |
| [`nu-save-notebook-kb`](#nu-save-notebook-kb) | `8192` | `O12_NU_SAVE_NOTEBOOK_KB` |
| [`jev-api-key-command`](#jev-api-key-command) |  |  |
| [`jev-credential-store`](#jev-credential-store) | `true` | `O12_JEV_CREDENTIAL_STORE` |
| [`jev-base-url`](#jev-base-url) |  | `TYPESAFE_BASE_URL` |
| [`jev-model`](#jev-model) |  | `TYPESAFE_DEFAULT_MODEL` |
| [`log-file`](#log-file) |  | `O12_LOG`, `--log` |
| [`log-level`](#log-level) | `info` | `O12_LOG_LEVEL` |
| [`otlp-endpoint`](#otlp-endpoint) |  | `OTEL_EXPORTER_OTLP_ENDPOINT`, `--otlp` |
| [`serve-listen`](#serve-listen) | `127.0.0.1:2312` |  |
| [`serve-authorized-keys`](#serve-authorized-keys) | `~/.ssh/authorized_keys` |  |
| [`serve-host-key`](#serve-host-key) |  |  |
| [`serve-idle-timeout`](#serve-idle-timeout) | `30m` |  |
| [`serve-max-sessions`](#serve-max-sessions) | `8` |  |
| [`serve-share`](#serve-share) | `edit` |  |
| [`serve-shell`](#serve-shell) | `false` |  |
| [`config-file`](#config-file) |  |  |

### Appearance

#### `theme`

Colors. `terminal` uses the terminal's own 16-color palette. `high-contrast` draws white on black or black on white by the terminal's background, with WCAG AAA contrast. Any other name is a terminal color scheme, built in (`012 config themes` lists them) or a file in the themes directory, drawn in its own colors with solid menu and status bars. `light:NAME,dark:NAME` picks one by the terminal's background and follows it when it changes.

| | |
|---|---|
| Type | text |
| Default | `terminal` |
| Environment | `O12_THEME` |
| Flag | `--theme` |
| Applies | File > Settings > Reload config |

#### `chart-images`

Draw charts as images on terminals with kitty graphics (kitty, Ghostty, WezTerm) or, outside tmux, sixel graphics (foot, xterm, mlterm, Windows Terminal). When false, charts are always text.

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

### Data

#### `locale`

How new sheets and files without a locale of their own are typed and shown: decimal and thousands separators, date order, the currency symbol and the formula argument separator (`;` where the decimal separator is a comma), as in Sheets' File > Settings > Locale, which sets a file's own. Files store the same thing in every locale. When unset, the POSIX locale (`LC_ALL`, `LC_NUMERIC`, then `LANG`, e.g. `de_DE.UTF-8`) picks it if it's one of these.

| | |
|---|---|
| Type | one of `en-US`, `en-GB`, `en-CA`, `en-AU`, `de-DE`, `de-CH`, `fr-FR`, `fr-CA`, `es-ES`, `es-MX`, `it-IT`, `pt-BR`, `pt-PT`, `nl-NL`, `sv-SE`, `da-DK`, `nb-NO`, `fi-FI`, `pl-PL`, `cs-CZ`, `ru-RU`, `tr-TR`, `ja-JP`, `zh-CN` |
| Default | `en-US` |
| Environment | `O12_LOCALE` |
| When unset | `LC_ALL`, `LC_NUMERIC`, `LANG` |
| Applies | File > Settings > Reload config |

#### `max-cells`

The most cells an import keeps, and a paste or fill writes at once. Numbers and text take 20 to 60 bytes a cell and formulas about 750, so the default of ten million cells of data is 200 to 600 MB. Imports keep whole rows up to the budget and say how many they left out; larger pastes and fills are refused. A function that holds what it reads (MEDIAN, SORT) is given no more of a linked source. The grid itself is 1,048,576 rows by 16,384 columns (A to XFD) whatever this is.

| | |
|---|---|
| Type | number |
| Default | `10000000` |
| Environment | `O12_MAX_CELLS` |
| Applies | File > Settings > Reload config |

### Nushell notebooks

#### `shell`

Whether notebooks run their code cells (docs/nushell/notebooks.md). `ask` runs what you write and asks once before running the cells of a file made on another computer; `on` never asks; `off` runs none. Opening a file never runs its cells.

| | |
|---|---|
| Type | one of `off`, `ask`, `on` |
| Default | `ask` |
| Environment | `O12_SHELL` |
| Applies | File > Settings > Reload config |

#### `nu-timeout`

Stop a notebook cell that runs longer than this; 0 lets it run until Stop (i i) stops it.

| | |
|---|---|
| Type | duration |
| Default | `30s` |
| Environment | `O12_NU_TIMEOUT` |
| Applies | File > Settings > Reload config |

#### `nu-config`

Run notebook cells with your nushell config files (config.nu, env.nu) rather than `nu --no-config-file`, for your own commands and aliases.

| | |
|---|---|
| Type | true or false |
| Default | `false` |
| Environment | `O12_NU_CONFIG` |
| Applies | File > Settings > Reload config |

#### `nu-save-cell-kb`

The largest output of one cell a saved file keeps, in kilobytes of NUON; a larger one is left out and shows `not saved; run to see` when the file is opened.

| | |
|---|---|
| Type | number |
| Default | `1024` |
| Environment | `O12_NU_SAVE_CELL_KB` |
| Applies | File > Settings > Reload config |

#### `nu-save-notebook-kb`

How much of all its cells' outputs a saved file keeps, in kilobytes of NUON: the outputs that fit, in the notebook's order.

| | |
|---|---|
| Type | number |
| Default | `8192` |
| Environment | `O12_NU_SAVE_NOTEBOOK_KB` |
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

Append telemetry events to this JSON log file. See docs/contributing/observability.md.

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

The address 012 serve listens on. Anything but the loopback address lets other machines reach it (with an authorized key). See docs/terminal/ssh.md.

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

#### `serve-share`

Whether 012 serve sessions opening the same file share its workbook: edit, everyone edits; view, one writes and the others follow until writing is handed over; off, each session has its own copy.

| | |
|---|---|
| Type | one of `edit`, `view`, `off` |
| Default | `edit` |
| Applies | restart 012 |

#### `serve-shell`

Let 012 serve sessions run notebooks' code cells, as the user 012 serve runs as, following the shell option. Off, served notebooks show their cells and saved outputs but run nothing.

| | |
|---|---|
| Type | true or false |
| Default | `false` |
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
