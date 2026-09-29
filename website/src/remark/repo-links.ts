// A remark plugin for links that leave the docs tree. The docs link to
// files beside them in the repository (../../ROADMAP.md, ../../CLAUDE.md),
// which read on GitHub but aren't pages of the site; this points them at
// the same file on GitHub, so the site's broken-link check still catches
// every link that resolves to nothing.
import path from 'node:path';

type Node = {type: string; url?: string; children?: Node[]};

export default function repoLinks(opts: {docsDir: string; repoDir: string; blobUrl: string}) {
  const docsDir = path.resolve(opts.docsDir);
  const repoDir = path.resolve(opts.repoDir);
  return (tree: Node, file: {path?: string}) => {
    if (!file.path) {
      return;
    }
    const from = path.dirname(file.path);
    const walk = (node: Node) => {
      if ((node.type === 'link' || node.type === 'definition') && node.url) {
        node.url = rewrite(node.url, from, docsDir, repoDir, opts.blobUrl);
      }
      node.children?.forEach(walk);
    };
    walk(tree);
  };
}

function rewrite(url: string, from: string, docsDir: string, repoDir: string, blobUrl: string): string {
  if (/^[a-z][a-z0-9+.-]*:|^#|^\//i.test(url)) {
    return url;
  }
  const [file, hash] = url.split('#', 2);
  const target = path.resolve(from, decodeURI(file));
  if (target === docsDir || target.startsWith(docsDir + path.sep) || !target.startsWith(repoDir + path.sep)) {
    return url;
  }
  const rel = path.relative(repoDir, target).split(path.sep).join('/');
  return `${blobUrl}/${rel}${hash ? `#${hash}` : ''}`;
}
