import type * as PrismNamespace from 'prismjs';

// A small Prism grammar for nushell, enough for the docs' pipelines:
// comments, strings, $variables, numbers with their units (1kb, 90sec),
// flags, keywords and operators. Tokens take the names Prism's themes
// already color.
export default function nushell(Prism: typeof PrismNamespace): void {
  Prism.languages.nu = {
    comment: {pattern: /(^|\s)#.*/, lookbehind: true, greedy: true},
    string: {pattern: /"(?:\\.|[^"\\\n])*"|'[^'\n]*'|`[^`\n]*`/, greedy: true},
    variable: /\$[A-Za-z_][\w-]*(?:\.[\w-]+)*/,
    keyword: /\b(?:let|mut|const|def|alias|if|else|for|in|while|loop|match|try|catch|return|break|continue|do|use|module|export)\b/,
    number: /\b\d+(?:\.\d+)?(?:kib|mib|gib|kb|mb|gb|tb|b|ns|us|ms|sec|min|hr|day|wk)?\b/i,
    boolean: /\b(?:true|false|null)\b/,
    // --flag or -f after a space, not a minus in an expression
    'attr-name': {pattern: /(\s)--?[A-Za-z][\w-]*/, lookbehind: true},
    operator: /\||==|!=|>=|<=|=~|!~|[<>=^]|\b(?:and|or|not|like)\b/,
    punctuation: /[{}[\]();,]/,
  };
  Prism.languages.nushell = Prism.languages.nu;
}
