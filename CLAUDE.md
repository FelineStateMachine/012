# Working on 012

Read [docs/UX.md](docs/UX.md), [docs/extending.md](docs/extending.md) and
[docs/testing.md](docs/testing.md) before changing code.

## No broken windows

Noise that stays becomes invisible, and then real problems hide in it.

- `make check` passes before every push: gofmt, vet, `make lint`
  (staticcheck in every module, doclint, doccheck, cognitive complexity
  at most 25, Go files at most 500 lines), unit tests, the excelize oracle and the
  libghostty e2e tests. A red check is fixed first, not worked around.
- A warning is fixed where it points, or turned off in its tool's config
  with the reason written next to it (`staticcheck.conf`). Nothing is left
  standing because "it's always been there".
- Docs and comments describe the code as it is and why. How it got there
  (what changed, what it did before, before and after numbers, which commit <!-- doclint:allow: names the phrases the rule forbids -->
  or merge, who did it) belongs in commit messages; doclint enforces this.
  Saying why the code stays compatible with a planned change is fine: that
  explains the code today. Plans go in ROADMAP.md.
- Each doc has one job and one home per fact: change the section a
  reader would look in, link to it from elsewhere, and restructure when
  appending would make a wall of text. Shipped roadmap items move to its
  Shipped list as one line.
- No scratch files, debug tests, temporary samples or build outputs (`*.test`, binaries) in commits; `make lint` refuses compiled files and anything over 2 MB.
- Visual changes are reviewed in the golden gallery (`make screens`).
