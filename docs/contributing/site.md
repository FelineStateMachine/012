---
title: "The docs site"
sidebar_position: 9
---

# The docs site

`website/` builds these docs into a static site with
[Docusaurus](https://docusaurus.io). It reads `docs/` where it is, so a
page is written once and reads the same on GitHub and on the site. It is
the only part of the repository that needs Node: `make build`,
`make check` and the Go toolchain never touch it.

```sh
make site        # install from website/package-lock.json, build into website/build
make site-serve  # a dev server on http://localhost:3000 that reloads as docs change
```

Both need Node 20.11 or later and npm. `npm ci` installs exactly what
the lockfile pins, with install scripts off (no dependency needs one).
To look at the built site as a static host serves it, run
`npm run serve` in `website/` after `make site`. `SITE_URL` sets the
address canonical links and the sitemap use (`SITE_URL=https://... make
site`), [012.dev.site](#where-it-lives) unless set; the site sits at the
root of its host.

## Where it lives

The site is published at [012.dev.site](https://012.dev.site/), which
also serves the install scripts and the latest release's archives. It
is a Cloudflare Worker with static assets and no code of its own,
configured in `website/wrangler.jsonc`: the worker `012-site` serves
`website/build`, `012.dev.site` is its custom domain (Cloudflare keeps
the DNS record and the certificate), an address that names a page's
directory without its trailing slash redirects to it, and an address
with no file gets the site's 404 page. `website/static/_headers` gives
the files whose extension names no type, `install.ps1` and
`SHA256SUMS`, a text one. To publish a new build:

```sh
make site-publish VERSION=v1.2.3
```

`make site-publish` is `make site-release`, which is `make site` with
the release added ([Publishing](releasing.md#publishing)), followed by
`wrangler deploy` in `website/`, the version `website/package.json` pins.
It needs Node 22 or later, which wrangler does, and wrangler logged in
to the Cloudflare account that holds the `dev.site` zone
(`npx wrangler login` in `website/`). Every deploy replaces the whole
site, so a deploy of a plain `make site` build would take the install
script and the archives off it. A file may be at most 25 MiB, which the
release archives stay well under.

`f58b.n.zip`, on the owner's nzip server, keeps links to the site
there working: each page's address serves a page that sends the browser
on to the same page here, and its `install.sh` and `install.ps1` run
this site's.

## The build is a docs check

`make site` fails on a link to a page that doesn't exist, an anchor no
heading makes, a missing image, two pages at one address or a
[diagram](#diagrams) Mermaid can't parse. It checks
the same links `scripts/doccheck` does, as the site resolves them, so a
page that passes `make lint` but breaks on the site shows up here.

## Old addresses keep working

A page's address is its path under `docs/`, so moving or deleting a
page would break links to it from elsewhere. A page that moves or goes
leaves a redirect in `website/redirects.json`, from its old address to
the page its reader wants now:

```json
{"from": "/docs/terminal/nushell/", "to": "/docs/nushell/notebooks/"}
```

`@docusaurus/plugin-client-redirects` writes a page at each old address
that sends the browser on, since a static host can't redirect by
itself, and the build fails on a redirect to a page that doesn't exist.
`scripts/doccheck` (in `make lint`) fails when a page the site has
published is gone without a redirect: it reads git's history back to
the site's first commit for every page there has been.

## How the docs become pages

- Pages are Markdown as GitHub reads it (`markdown.format: 'detect'`):
  `.md` files are CommonMark, so `<`, `{` and HTML comments are text and
  markup rather than MDX. A page that needs components would be `.mdx`.
- Each folder's `README.md` is its category's page (`/docs/sheets/`),
  and the sidebar is generated from the tree: `_category_.json` for
  folders, `title` and `sidebar_position` for pages
  ([Docs](testing.md#docs) has the rules the tree follows).
- Images in `docs/media/` are bundled from the pages that show them;
  an image marked `#gh-dark-mode-only` or `#gh-light-mode-only` shows in
  that color mode only ([Pictures of 012](#pictures-of-012)).
- Prism has no nushell grammar, so `nu` code blocks use a small one in
  `website/src/prism/nushell.ts`, loaded by the swizzled
  `website/src/theme/prism-include-languages.ts`.
- A relative link that leaves `docs/` (the roadmap, `CLAUDE.md`)
  becomes a link to the file on GitHub
  (`website/src/remark/repo-links.ts`).
- The generated pages, [Functions](../reference/functions.md) and
  [Configuration](../reference/config.md), are pages like any other.
- `mermaid` code blocks are drawn as diagrams
  (`@docusaurus/theme-mermaid`), as GitHub draws them; see
  [Diagrams](#diagrams).
- Search is local: `@easyops-cn/docusaurus-search-local` builds an index
  at build time, and nothing is sent to a service.

## The look

The landing page (`website/src/pages/index.tsx`) is drawn as a 012
screen: a control panel with the mode indicator, the recording in an
overlay frame, the features as cells of a sheet (the one under the
pointer shows its entry in the control panel), and a status line. The
colors in `website/src/css/custom.css` are the reference palettes of the
golden screens, given the roles the app gives them: cyan where you are,
bright black header bands, blue links and selection. Code, headings and
chrome are JetBrains Mono, bundled with the site, the font of every
[picture of 012](#font-and-colors); body text is the system's own sans.

## Diagrams

Diagrams are Mermaid, in a `mermaid` code block, never ASCII art: the
same source is drawn on the site and on GitHub. A code block of text
stays for text a reader types or a shell prints; 012's own screen is a
picture ([Pictures of 012](#pictures-of-012)).

- Draw one where a flow, a state machine or a sequence would otherwise
  take paragraphs: a flowchart for data moving through the code, a state
  diagram for modes, a sequence diagram for two programs talking. Not
  for its own sake: a list or a table that reads well stays one.
- A short sentence before it says what it shows, and the prose around
  it keeps the facts the diagram can't (why, limits, names to search
  for) without walking through it again.
- Labels are the words the docs use: the key, the command's title, the
  function or type name; menu paths in them are checked as in prose.
- Colors and fonts come from the site: `website/src/theme/Mermaid`
  (Docusaurus's component, swizzled) draws with Mermaid's `base` theme
  and variables read from the palette in `custom.css` for the color
  mode shown, so diagrams don't set their own styles.
- `make site` fails on a diagram Mermaid can't parse, which the site
  would draw as an error: `website/scripts/check-mermaid.mjs` (`npm run
  diagrams` in `website/`) parses every `mermaid` block in `docs/` with
  the Mermaid the site draws with, and names the page, the line and
  Mermaid's error. A `;` in a sequence diagram's message ends the
  statement, so write a comma, or `#59;` where the semicolon matters.

## Pictures of 012

A page shows 012 as a picture: a still of the screen in the site's
color mode, or a recording. A screen typed out in a code block reads as
a test fixture, loses the colors that carry meaning, and wraps on a
narrow page, so `scripts/doclint` fails a code block that draws one
(box-drawing lines with the menu bar or the notebook's toolbar, or the
grid's column headers).

A still is a [golden screen](testing.md#golden-screens) drawn as a PNG.
Each is an entry of `docScreens` in `e2e/stills_test.go`: its name,
the screen whose setup it records, a terminal size that reads on a
page (80 columns, unless the state needs more) and the
[family](#font-and-colors) of the pages that show it. `make screens`
records it on a dark and a light terminal and draws both into
`docs/media`, in the family's dark and light schemes:

```go
{name: "evaluate", from: "evaluate", cols: 80, rows: 12, family: "Catppuccin"},
// docs/media/evaluate-dark.png and docs/media/evaluate-light.png
```

The drawing is Go (`e2e/stilldraw_test.go`): the golden's cells in its
page's [color schemes](#font-and-colors), text in JetBrains Mono, and
box drawing, blocks, braille and the chrome's symbols drawn as a
terminal draws them, so frames join and every machine draws the same
picture. Each PNG records which golden it was drawn from, and
`make e2e` fails when a golden has changed since.

A page shows both pictures together, marked as GitHub marks an image for
one color mode; `custom.css` shows the site's the same way:

```md
![Evaluate formula on B8, two steps in: the next part underlined](../media/evaluate-dark.png#gh-dark-mode-only)
![Evaluate formula on B8, two steps in: the next part underlined](../media/evaluate-light.png#gh-light-mode-only)
```

- The alt text says what the picture shows, the state and what to look
  at, the same words on both. Write a range in it as `B3 to B5`: the site
  reads `:B5` in text as a directive and drops it.
- One still where the state is the point: what a feature looks like
  before a page explains it, or a state words describe badly (a box,
  marks in the grid). The prose keeps what the picture can't: keys,
  limits, why.
- A flow of several steps is a recording, a VHS tape in `demos/`
  ([Demo recordings](testing.md#demo-recordings)).
- `scripts/doccheck` fails on a still no page shows, or one shown
  without its other color mode.

## Font and colors

Every picture of 012 is set in one font, JetBrains Mono: the
recordings, the stills, and the site's code and terminal frames. It is
VHS's and Ghostty's default, so the pictures look like the terminals
people use; it reads well small, its italic isn't cursive, and the SIL
Open Font License lets it be bundled. The site's comes from
`@fontsource/jetbrains-mono`, and the stills and recordings use the
TTFs in `e2e/testdata/fonts` (licence in `NOTICE`). VHS's browser draws
with the fonts the system or the page gives it, so `make demos` serves
ttyd's own page with the font added (`demos/lib/font-page`) and the
recordings need nothing installed. Box drawing and blocks are drawn as
shapes in both, by xterm.js in the recordings and by the stills'
drawing, so frames join whatever the font.

The pictures' colors are a few terminal color schemes people know, in
families of a dark and a light scheme, so the docs don't all look alike
while each picture stays plainly 012. Each page is in one family:

| Family | Dark | Light | Pages |
|---|---|---|---|
| Catppuccin | `Catppuccin Mocha` | `Catppuccin Latte` | `getting-started/`, `contributing/`, `reference/`, `formulas/tracing.md` |
| Tokyo Night | `tokyonight` | `tokyonight-day` | `nushell/` |
| Rosé Pine | `rose-pine` | `rose-pine-dawn` | `sheets/rules.md`, `sheets/formatting.md`, `sheets/tables.md` |
| GitHub | `GitHub Dark` | `Github` | `sheets/` |
| Gruvbox | `GruvboxDark` | `Gruvbox Light` | `files/`, `formulas/` |
| Nord | `nord` | `nord-light` | `agents/`, `terminal/` |

- A page is in the family that names it, or else the one naming its
  deepest folder. Its stills are drawn in the family's dark and light
  schemes, and its recordings use either one (`Set Theme` in the tape,
  or `demos/lib/setup.tape`'s). The README and the landing page, a tour
  of every topic, show each recording as its own page does.
- `scripts/doccheck` fails on a page showing a picture in no family,
  and on a still or a tape in a family other than its pages'.
- A family is a restrained, widely used scheme with both a dark and a
  light variant: no neon or high-saturation schemes. Its schemes are
  named as in 012's built-in list (`internal/ui/theme/themes.json`,
  VHS's own list, so `Set Theme` takes the same names), and
  `TestPictureSchemes` (`internal/ui/theme`) holds each to readable
  contrast: its text on its background at least 4.5:1, and 012's roles
  drawn in it (header bands, the selection, the pointer, muted text)
  at their minimums.
- The recordings run 012 with its theme set to the tape's scheme
  (`make demos` sets `O12_THEME` from `Set Theme`), so 012's roles get
  the scheme's colors with their contrast held, as for anyone who picks
  that scheme. A still draws its golden's palette colors in the scheme
  and holds each cell to the same minimums (4.5:1 for text, 3:1 for
  lines and muted text, 2:1 for faint text), moving the text toward the
  scheme's text color as 012 does, so it stays drawn from the golden.
