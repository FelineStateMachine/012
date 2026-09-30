// Parses every mermaid code block in docs/ with the Mermaid the site
// draws with, and fails on one it can't read: in the browser such a
// block is drawn as an error, not a diagram. `npm run build` (and so
// `make site`) runs it first.
//
// Mermaid's parser runs in Node, but it sanitizes labels with DOMPurify,
// which needs a DOM: jsdom gives it one, set up before Mermaid loads.

import {readFileSync, readdirSync} from 'node:fs';
import {join, relative} from 'node:path';
import {fileURLToPath} from 'node:url';
import {JSDOM} from 'jsdom';

const docs = fileURLToPath(new URL('../../docs', import.meta.url));

const {window} = new JSDOM('<!doctype html><html><body></body></html>');
globalThis.window = window;
globalThis.document = window.document;
globalThis.DOMParser = window.DOMParser;
globalThis.Element = window.Element;
const {default: mermaid} = await import('mermaid');
mermaid.initialize({startOnLoad: false});

function markdownFiles(dir) {
  return readdirSync(dir, {withFileTypes: true}).flatMap((e) => {
    const path = join(dir, e.name);
    if (e.isDirectory()) {
      return markdownFiles(path);
    }
    return /\.mdx?$/.test(e.name) ? [path] : [];
  });
}

// The mermaid blocks of a page, as CommonMark reads its fences: a block
// inside a longer fence (an example of Markdown) is text, not a diagram.
function mermaidBlocks(text) {
  const blocks = [];
  let fence = null;
  text.split('\n').forEach((line, i) => {
    const m = /^(\s*)(`{3,}|~{3,})\s*([^`\s]*)/.exec(line);
    if (fence === null) {
      if (m) {
        fence = {marker: m[2], indent: m[1].length, mermaid: m[3] === 'mermaid', line: i + 1, body: []};
      }
      return;
    }
    const close = /^\s*(`{3,}|~{3,})\s*$/.exec(line);
    if (close && close[1][0] === fence.marker[0] && close[1].length >= fence.marker.length) {
      if (fence.mermaid) {
        blocks.push({line: fence.line, text: fence.body.join('\n')});
      }
      fence = null;
      return;
    }
    fence.body.push(line.slice(Math.min(fence.indent, line.search(/\S|$/))));
  });
  return blocks;
}

let count = 0;
const failures = [];
for (const file of markdownFiles(docs).sort()) {
  for (const block of mermaidBlocks(readFileSync(file, 'utf8'))) {
    count++;
    try {
      await mermaid.parse(block.text);
    } catch (err) {
      const first = block.text.trim().split('\n')[0];
      failures.push(`docs/${relative(docs, file)}:${block.line}: ${first}\n${String(err.message ?? err)}`);
    }
  }
}

if (failures.length > 0) {
  console.error(`${failures.length} of ${count} mermaid diagrams don't parse:\n`);
  console.error(failures.join('\n\n'));
  process.exit(1);
}
console.log(`${count} mermaid diagrams parse`);
