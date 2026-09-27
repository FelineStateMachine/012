---
title: "Releasing"
sidebar_position: 8
---

# Releasing

A release is an annotated version tag whose message is the release
notes, and archives of the binaries built from it on a maintainer's
machine. The GitHub repository only hosts the code: nothing builds or
publishes there, and `make dist` uploads nothing.

## Checklist

1. **`make check`** passes on the commit to be tagged (see
   [Testing](testing.md)), with the golden screens reviewed in the
   gallery.
2. **Stress** (optional for small releases): `make stress` on a quiet
   machine, then `make stress-report`, which compares the run with the
   last release's and exits non-zero on a regression past the threshold
   (see [Observability](observability.md#stress-runs)).
   Record the run on the release commit so the next release compares
   against it.
3. **XLSX in spreadsheet apps.** Export a workbook that uses what the
   release touches (formats, notes, frozen panes, filters, validation,
   conditional formats, charts' data) with File > Export, and open it in
   Excel, LibreOffice Calc and Google Sheets: it opens without a repair
   prompt, and values, formulas and formats read as they did in 012. The
   automated checks read XLSX with excelize and 012's own reader only.
4. **Tag** with the release notes as the message, what changed since the
   previous tag for a user, with the install line last:

   ```sh
   git tag -a v1.2.3        # the editor opens for the notes
   git push origin v1.2.3
   ```

   The Go module proxy picks the tag up, so `go install
   github.com/FelineStateMachine/012/cmd/012@v1.2.3` works from then on.
5. **Binaries**: `make dist VERSION=v1.2.3` on the tagged commit.

## make dist

`make dist VERSION=v1.2.3` (`scripts/dist.sh`) cross-compiles 012 with
`CGO_ENABLED=0` and `-trimpath` for macOS, Linux and Windows on amd64
and arm64, stamping the version with `-ldflags -X main.version=...`, so
`012 version` prints it. Each platform gets an archive in `dist/`
(gitignored) holding the binary, `LICENSE`, `NOTICE` and `README.md`:

```
dist/012_1.2.3_darwin_arm64.tar.gz
dist/012_1.2.3_linux_amd64.tar.gz
dist/012_1.2.3_windows_amd64.zip
...
dist/SHA256SUMS
```

`PLATFORMS="darwin/arm64 linux/amd64"` narrows the list. The script
refuses a version that isn't `vMAJOR.MINOR.PATCH` (with an optional
`-suffix`) or that is tagged at another commit than the one checked
out, and warns when the tree has uncommitted changes. The archives and
`SHA256SUMS` are what a release attaches, wherever it is published.
