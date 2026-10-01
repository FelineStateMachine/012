import {useState, type ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import Layout from '@theme/Layout';
import firstSteps from '@site/../docs/media/first-steps.gif';
import styles from './index.module.css';

// A cell of the landing page's sheet: a feature, what the control panel
// shows when the pointer is on it, and where it leads.
type Cell = {
  addr: string;
  entry: string;
  hint: string;
  title: string;
  body: string;
  to: string;
};

// The control panel at rest: the pointer on A1, holding a label, written
// with the apostrophe 012 shows before text.
const home: Cell = {
  addr: 'A1',
  entry: "'Start here",
  hint: 'Hover a cell, or Tab through them. F1 opens the guides.',
  title: '',
  body: '',
  to: '/docs/',
};

const cells: Cell[] = [
  {
    addr: 'A1',
    entry: '=SUM(B2:B9)*(1+Tax)',
    hint: 'Formulas: Enter commits, arrows point at cells, suggestions as you type',
    title: 'Formulas as in Sheets',
    body: "Sheets' functions with suggestions and argument hints, named ranges, arrays that spill, LAMBDA, decimal arithmetic for money, and undo for everything.",
    to: '/docs/formulas/',
  },
  {
    addr: 'B1',
    entry: '=SORT(FILTER(A2:C40, C2:C40>10), 3, FALSE)',
    hint: 'Working with sheets: sort, filter, rules, pivots, charts, macros',
    title: 'Data tools',
    body: 'Freeze, sort, filter, find and replace, conditional formatting, dropdowns and checkboxes, live pivot tables, and charts that float over the grid.',
    to: '/docs/sheets/',
  },
  {
    addr: 'A2',
    entry: "'.012 CSV TSV JSON NUON XLSX SQLite Parquet WK1",
    hint: 'Files: import, download, Excel, the diff-friendly .012 format',
    title: 'Files in, files out',
    body: 'A diff-friendly JSON format of its own; imports from CSV to Lotus 1-2-3, exports to CSV, JSON, NUON, XLSX and SQLite.',
    to: '/docs/files/',
  },
  {
    addr: 'B2',
    entry: "'theme = light:Catppuccin Latte,dark:Catppuccin Mocha",
    hint: 'The terminal: its features, themes, 012 serve over SSH',
    title: 'Native to the terminal',
    body: 'Your terminal\'s own colors or any of hundreds of schemes, the mouse, hyperlinks, charts as images, and your sheets over SSH with 012 serve.',
    to: '/docs/terminal/',
  },
  {
    addr: 'A3',
    entry: "'ls | sheet | where size > 1kb",
    hint: 'Nushell: pipelines, notebooks, types, a cookbook',
    title: 'Nushell, both ways',
    body: "A stage in a nushell pipeline with file sizes, durations and dates kept, or a notebook of nushell cells whose outputs, sent to a sheet, are live tables that formulas and charts read.",
    to: '/docs/nushell/',
  },
  {
    addr: 'B3',
    entry: "'$app | where status >= 500",
    hint: 'Following files: a linked region follows a log as it grows',
    title: 'Files that keep growing',
    body: 'Link a CSV, JSON lines or NUON log and its rows come in as they are written, like tail -f; formulas, charts and pipelines over it keep up.',
    to: '/docs/files/following/',
  },
];

// The menu bar lists the navbar's sections, in its order: on this page
// it stands in for them (custom.css hides the navbar's at widths where
// both would show).
const menu: {label: string; to: string}[] = [
  {label: 'Getting started', to: '/docs/getting-started/'},
  {label: 'Sheets', to: '/docs/sheets/'},
  {label: 'Formulas', to: '/docs/formulas/'},
  {label: 'Files', to: '/docs/files/'},
  {label: 'Nushell', to: '/docs/nushell/'},
  {label: 'Agents', to: '/docs/agents/'},
  {label: 'Terminal', to: '/docs/terminal/'},
  {label: 'Reference', to: '/docs/reference/'},
];

function Chip({children}: {children: ReactNode}) {
  return <kbd className={styles.chip}>{children}</kbd>;
}

// The control panel: the entry of the cell under the pointer, the menu
// bar and the context line, with the mode indicator at the right.
function ControlPanel({cell, pointing}: {cell: Cell; pointing: boolean}) {
  return (
    <section className={styles.panel} aria-label="Control panel" data-control-panel>
      <div className={clsx(styles.line, styles.line1)}>
        <span className={styles.addr}>{cell.addr}:</span>
        <span className={styles.entry}>{cell.entry}</span>
        <span className={styles.mode}>{pointing ? 'POINT' : 'READY'}</span>
      </div>
      <nav className={clsx(styles.line, styles.line2)} aria-label="Sections">
        {menu.map((m) => (
          <Link key={m.label} to={m.to} className={styles.menuItem}>
            <u>{m.label[0]}</u>
            {m.label.slice(1)}
          </Link>
        ))}
      </nav>
      <div className={clsx(styles.line, styles.line3)} aria-live="polite">
        {cell.hint}
        <span className={styles.cursor} aria-hidden="true" />
      </div>
    </section>
  );
}

function Hero() {
  return (
    <section className={styles.hero}>
      <figure className={styles.frame}>
        <figcaption className={styles.frameTitle}>first-steps.tape</figcaption>
        <img
          src={firstSteps}
          alt="Typing a small budget, pointing at cells in a formula, and watching totals recalculate"
          width={1000}
          height={600}
        />
        <span className={styles.frameCount}>Catppuccin Mocha</span>
      </figure>
      <div className={styles.heroText}>
        <h1 className={styles.srOnly}>012</h1>
        <p className={styles.sub}>
          The control panel, the mode indicator and the character grid of Lotus 1-2-3. Inside the grid, Google
          Sheets: typing replaces a cell, <code>=</code> starts a formula, Enter and Tab move you on, and Sheets'
          shortcuts do what you expect. One pure-Go binary.
        </p>
        <pre className={styles.install} aria-label="Install">
          <span className={styles.prompt}>$ </span>curl -fsSL https://012.dev.site/install.sh | sh
        </pre>
        <div className={styles.actions}>
          <Link className={clsx(styles.action, styles.actionPrimary)} to="/docs/getting-started/">
            <Chip>F1</Chip> Get started
          </Link>
          <Link className={styles.action} to="/docs/reference/keys/">
            <Chip>F10</Chip> Every key
          </Link>
          <span className={styles.searchHint}>
            <Chip>Ctrl+K</Chip> searches the docs
          </span>
        </div>
      </div>
    </section>
  );
}

// The features as a sheet: column and row headers around six cells,
// the one under the pointer drawn as 012 draws the active cell.
function Sheet({active, onPoint}: {active: string | null; onPoint: (c: Cell | null) => void}) {
  const cols = ['A', 'B'];
  const rows = [1, 2, 3];
  return (
    <section className={styles.sheet} aria-label="Features" onMouseLeave={() => onPoint(null)}>
      <div className={styles.corner} />
      {cols.map((c) => (
        <div key={c} className={clsx(styles.colHeader, active?.startsWith(c) && styles.headerActive)}>
          {c}
        </div>
      ))}
      {rows.map((r) => (
        <Row key={r} r={r} active={active} onPoint={onPoint} />
      ))}
    </section>
  );
}

function Row({r, active, onPoint}: {r: number; active: string | null; onPoint: (c: Cell | null) => void}) {
  const row = cells.filter((c) => c.addr.endsWith(String(r)));
  return (
    <>
      <div className={clsx(styles.rowHeader, active?.endsWith(String(r)) && styles.headerActive)}>{r}</div>
      {row.map((c) => (
        <Link
          key={c.addr}
          to={c.to}
          className={clsx(styles.cell, active === c.addr && styles.cellActive)}
          onMouseEnter={() => onPoint(c)}
          onFocus={() => onPoint(c)}
          onBlur={() => onPoint(null)}>
          <span className={styles.cellEntry}>{c.entry}</span>
          <h2 className={styles.cellTitle}>{c.title}</h2>
          <p className={styles.cellBody}>{c.body}</p>
          <span className={styles.cellGo}>Open {c.to.replaceAll('/', ' ').trim().split(' ').pop()} &rarr;</span>
        </Link>
      ))}
    </>
  );
}

function StatusLine() {
  const refs = [
    {label: 'Keys and mouse', to: '/docs/reference/keys/'},
    {label: 'Functions', to: '/docs/reference/functions/'},
    {label: 'Configuration', to: '/docs/reference/config/'},
    {label: 'Macro API', to: '/docs/reference/macro-api/'},
    {label: 'Roadmap', to: 'https://github.com/FelineStateMachine/012/blob/main/ROADMAP.md'},
  ];
  return (
    <nav className={styles.status} aria-label="Reference">
      <span className={styles.statusLabel}>Reference</span>
      {refs.map((r) => (
        <Link key={r.label} to={r.to} className={styles.statusItem}>
          {r.label}
        </Link>
      ))}
    </nav>
  );
}

export default function Home(): ReactNode {
  const [cell, setCell] = useState<Cell | null>(null);
  return (
    <Layout
      description="012 is a terminal spreadsheet with the look of Lotus 1-2-3 and the behavior of Google Sheets.">
      <main className={styles.page}>
        <ControlPanel cell={cell ?? home} pointing={cell !== null} />
        <Hero />
        <Sheet active={cell?.addr ?? null} onPoint={setCell} />
        <StatusLine />
      </main>
    </Layout>
  );
}
