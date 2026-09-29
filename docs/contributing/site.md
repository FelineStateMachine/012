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
site`); the site sits at the root of its host.

## Where it lives

The site is published at [f58b.n.zip](https://f58b.n.zip/), a public,
permanent address on the owner's nzip server. To publish a new build to
the same address:

```sh
SITE_URL=https://f58b.n.zip make site
nzip site push website/build public:012
```

The target keeps its policies (public, no expiry) between pushes.

## The build is a docs check

`make site` fails on a link to a page that doesn't exist, an anchor no
heading makes, a missing image or two pages at one address. It checks
the same links `scripts/doccheck` does, as the site resolves them, so a
page that passes `make lint` but breaks on the site shows up here.

## How the docs become pages

- Pages are Markdown as GitHub reads it (`markdown.format: 'detect'`):
  `.md` files are CommonMark, so `<`, `{` and HTML comments are text and
  markup rather than MDX. A page that needs components would be `.mdx`.
- Each folder's `README.md` is its category's page (`/docs/sheets/`),
  and the sidebar is generated from the tree: `_category_.json` for
  folders, `title` and `sidebar_position` for pages
  ([Docs](testing.md#docs) has the rules the tree follows).
- Images in `docs/media/` are bundled from the pages that show them.
- Prism has no nushell grammar, so `nu` code blocks use a small one in
  `website/src/prism/nushell.ts`, loaded by the swizzled
  `website/src/theme/prism-include-languages.ts`.
- A relative link that leaves `docs/` (the roadmap, `CLAUDE.md`)
  becomes a link to the file on GitHub
  (`website/src/remark/repo-links.ts`).
- The generated pages, [Functions](../reference/functions.md) and
  [Configuration](../reference/config.md), are pages like any other.
- Search is local: `@easyops-cn/docusaurus-search-local` builds an index
  at build time, and nothing is sent to a service.

## The look

The landing page (`website/src/pages/index.tsx`) is drawn as a 012
screen: a control panel with the mode indicator, the recording in an
overlay frame, the features as cells of a sheet (the one under the
pointer shows its entry in the control panel), and a status line. The
colors in `website/src/css/custom.css` are the reference palettes of the
golden screens, given the roles the app gives them: cyan where you are,
bright black header bands, blue links and selection. Headings and chrome
are IBM Plex Mono, body text IBM Plex Sans, both bundled with the site.
