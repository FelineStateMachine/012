# JEV functions

With a TypeSafe API key, four functions ask the hosted JEV model about a
value (a cell, a range or text). They follow Sheets' argument style: the
value, the question, then what the answers mean.

| Function | Returns | Like |
|---|---|---|
| `=JEV.TEST(A2, "Is this a complaint?", [yes means], [no means])` | TRUE or FALSE | an `IF` condition, a boolean mask |
| `=JEV.PROB(A2, "Is this a complaint?")` | probability of yes, as a percent | `predict_proba` |
| `=JEV.CLASSIFY(A2, "Sentiment", "negative, positive", [descriptions])` | the best label | `SWITCH`, `pd.cut` |
| `=JEV.SCORE(A2, "Urgency", "low, mid, high")` | a score from 0 to levels-1 | a rating scale |

Labels, descriptions and levels can also be ranges. Answers arrive in the
background (cells show `Loading…`), are cached by question, and the context
line shows the confidence for the selected cell. Data > Ask JEV again
re-asks the selection.

Without a key the functions show `#N/A` and say how to add one.
`JEV_LIVE_TEST=1 go test ./internal/jev -run TestLive` checks the real
service.

## Setup

The API key never goes in a file 012 reads or on a command line. Store it
once in your operating system's credential store:

```sh
012 config set-key        # asks for the key without echoing it
op read op://Private/TypeSafe/credential | 012 config set-key   # or pipe it in
012 config delete-key     # remove it
```

or in the app, File > Settings > JEV API key, which asks on the context
line with the key masked and turns JEV on at once. It then checks the key
with one test call, a fixed yes/no question about `2 + 2 = 4` that sends
none of your data, and says "Key saved and checked" or "Key saved, but the
check failed" with the reason (such as the service refusing the key). A
key whose check failed is kept, since the service may only be unreachable
for now; store the right one the same way. `012 config` says where the
key would come from, without showing it.

012 looks for the key in this order:

1. **`TYPESAFE_API_KEY`** in the environment, for CI, containers and
   one-off runs.
2. **The credential store**: the macOS Keychain, the Windows Credential
   Manager, or the Secret Service on Linux and the BSDs (GNOME Keyring,
   KWallet, KeePassXC; this needs `secret-tool`, in the `libsecret-tools`
   package on Debian and Ubuntu). The item is service `012`, account
   `jev-api-key`. `jev-credential-store = false` in the config skips it.
3. **`jev-api-key-command`** in the config: a command that prints the key,
   for password managers:

   ```
   jev-api-key-command = op read op://Private/TypeSafe/credential
   jev-api-key-command = pass show typesafe
   jev-api-key-command = sh -c 'gpg -dq ~/.secrets/typesafe.gpg | head -1'
   ```

   It runs the first time a sheet asks JEV something, not at every
   start, for at most 10 seconds, and the first line it prints is the key.
   It runs without a shell: the value is split into words, with quotes
   grouping words and a backslash escaping the next character, and
   nothing is expanded (`$HOME`, `~`, `*`, pipes and `;` are passed on
   literally). For a pipeline, write `sh -c '...'` yourself.

The service's address and model come from the config file or the
environment only: `jev-base-url` (or `TYPESAFE_BASE_URL`), which must be
https unless it's this machine, and `jev-model` (or
`TYPESAFE_DEFAULT_MODEL`). See [config.md](config.md).

**`.env` files are no longer read.** A `.env` next to a downloaded sheet
could set `TYPESAFE_BASE_URL` and send your key to someone else's server.
If the directory 012 starts in has a `.env` with `TYPESAFE_API_KEY`, the
context line suggests `012 config set-key`; 012 never reads the key from
it.

## How requests behave

- Requests run in the background, eight at a time; the status line counts
  them and terminals that support it (OSC 9;4) show progress in the tab.
- Answers are cached by the exact question (value, question and criteria),
  so recalculating, undo and redo never ask twice, and `JEV.TEST` and
  `JEV.PROB` on the same question share one request.
- Errors in the inputs (`#DIV/0!` in the value, an empty question, fewer
  than two labels, more than the service's 255 labels or 10 score levels)
  show as errors at once and are never sent.
- A failed request shows `#ERROR!`, and the context line says why.
- 012 never logs, shows or saves the API key anywhere but the credential
  store, and never passes it as a command-line argument: the macOS
  Keychain and the Secret Service get it on stdin.

The client is [typesafe-go](https://github.com/FelineStateMachine/typesafe-go).
