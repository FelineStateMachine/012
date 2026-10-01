import path from 'node:path';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';
import {themes as prismThemes} from 'prism-react-renderer';
import repoLinks from './src/remark/repo-links';
// The addresses of pages that moved or went, each to where its reader
// goes now; doccheck makes sure every docs page the site has published
// is still a page or is here.
import redirects from './redirects.json';

const repo = 'https://github.com/FelineStateMachine/012';
const repoDir = path.resolve(__dirname, '..');
const docsDir = path.join(repoDir, 'docs');

// The navbar's sections: a label and the docs folder it opens.
const sections: [string, string][] = [
  ['Getting started', 'getting-started'],
  ['Sheets', 'sheets'],
  ['Formulas', 'formulas'],
  ['Files', 'files'],
  ['Nushell', 'nushell'],
  ['Agents', 'agents'],
  ['Terminal', 'terminal'],
];

// The site is served at the root of its own host; SITE_URL names that
// host for canonical links and the sitemap.
const url = process.env.SITE_URL || 'https://012.dev.site';

const config: Config = {
  title: '012',
  tagline: 'A spreadsheet for the terminal: the look of Lotus 1-2-3, the behavior of Google Sheets.',
  favicon: 'img/favicon.svg',
  url,
  baseUrl: '/',
  // Every page is a directory with an index.html, so plain static hosting
  // resolves each address.
  trailingSlash: true,

  // The build is a docs check: a link or anchor that goes nowhere fails it.
  onBrokenLinks: 'throw',
  onBrokenAnchors: 'throw',
  onDuplicateRoutes: 'throw',

  markdown: {
    // .md files are CommonMark, as GitHub reads them: <, { and HTML
    // comments are text and markup, not JSX. .mdx files would be MDX.
    format: 'detect',
    // ```mermaid blocks are drawn as diagrams, as GitHub draws them; the
    // swizzled src/theme/Mermaid gives them the site's palette.
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: 'throw',
      onBrokenMarkdownImages: 'throw',
    },
  },

  i18n: {defaultLocale: 'en', locales: ['en']},

  presets: [
    [
      'classic',
      {
        docs: {
          path: docsDir,
          routeBasePath: 'docs',
          sidebarPath: './sidebars.ts',
          editUrl: `${repo}/edit/main/`,
          exclude: ['**/_*.{md,mdx}', '**/_*/**'],
          beforeDefaultRemarkPlugins: [[repoLinks, {docsDir, repoDir, blobUrl: `${repo}/blob/main`}]],
        },
        blog: false,
        theme: {customCss: './src/css/custom.css'},
      } satisfies Preset.Options,
    ],
  ],

  // A page at each old address that sends its reader on (a static host
  // can't redirect by itself).
  plugins: [['@docusaurus/plugin-client-redirects', {redirects}]],

  themes: [
    '@docusaurus/theme-mermaid',
    [
      '@easyops-cn/docusaurus-search-local',
      {
        hashed: true,
        indexBlog: false,
        indexPages: false,
        docsRouteBasePath: 'docs',
        docsDir: docsDir,
        highlightSearchTermsOnTargetPage: true,
        explicitSearchResultPath: true,
      },
    ],
  ],

  themeConfig: {
    colorMode: {defaultMode: 'dark', respectPrefersColorScheme: true},
    navbar: {
      // The logomark alone: it already reads 012, so no title beside it.
      logo: {alt: '012', src: 'img/logo.svg'},
      // The docs' sections, as their folders are, and the reference as a
      // menu; the landing page's control panel lists the same sections.
      items: [
        ...sections.map(([label, dir]) => ({
          to: `/docs/${dir}/`,
          activeBasePath: `/docs/${dir}/`,
          label,
          position: 'left' as const,
        })),
        {
          type: 'dropdown',
          label: 'Reference',
          position: 'left',
          to: '/docs/reference/',
          activeBasePath: '/docs/reference/',
          items: [
            {to: '/docs/reference/keys/', label: 'Keys'},
            {to: '/docs/reference/functions/', label: 'Functions'},
            {to: '/docs/reference/config/', label: 'Configuration'},
            {to: '/docs/reference/command-line/', label: 'Command line'},
            {to: '/docs/reference/json/', label: 'JSON'},
            {to: '/docs/reference/macro-api/', label: 'Macro API'},
          ],
        },
        {href: repo, label: 'GitHub', position: 'right'},
        // The mode indicator, as at the right end of 012's first line.
        {type: 'html', position: 'right', value: '<span class="mode-chip" aria-hidden="true">READY</span>'},
      ],
    },
    footer: {
      style: 'light',
      links: [
        {
          title: 'Use',
          items: [
            {label: 'Install and run', to: '/docs/getting-started/install/'},
            {label: 'The screen', to: '/docs/getting-started/the-screen/'},
            {label: 'Keys and mouse', to: '/docs/reference/keys/'},
          ],
        },
        {
          title: 'Build',
          items: [
            {label: 'Contributing', to: '/docs/contributing/'},
            {label: 'Roadmap', href: `${repo}/blob/main/ROADMAP.md`},
            {label: 'GitHub', href: repo},
          ],
        },
      ],
      copyright: `<span class="mode-chip">READY</span> 012 is free software under the <a href="${repo}/blob/main/LICENSE">MIT license</a>.`,
    },
    // The base theme is the one Mermaid lets every color of be set;
    // src/theme/Mermaid sets them from custom.css for each color mode.
    mermaid: {theme: {light: 'base', dark: 'base'}},
    prism: {
      theme: prismThemes.oneLight,
      darkTheme: prismThemes.oneDark,
      additionalLanguages: ['bash', 'sql'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
