import hljs from 'highlight.js/lib/core';
import bash from 'highlight.js/lib/languages/bash';
import css from 'highlight.js/lib/languages/css';
import diff from 'highlight.js/lib/languages/diff';
import go from 'highlight.js/lib/languages/go';
import javascript from 'highlight.js/lib/languages/javascript';
import json from 'highlight.js/lib/languages/json';
import markdown from 'highlight.js/lib/languages/markdown';
import python from 'highlight.js/lib/languages/python';
import rust from 'highlight.js/lib/languages/rust';
import sql from 'highlight.js/lib/languages/sql';
import typescript from 'highlight.js/lib/languages/typescript';
import xml from 'highlight.js/lib/languages/xml';
import yaml from 'highlight.js/lib/languages/yaml';
import { Result } from 'neverthrow';

const GRAMMARS = {
  typescript,
  javascript,
  go,
  python,
  json,
  css,
  xml,
  bash,
  yaml,
  sql,
  markdown,
  rust,
  diff,
};

for (const [name, grammar] of Object.entries(GRAMMARS)) {
  hljs.registerLanguage(name, grammar);
}

/** The grammars allowed for inline highlighted code. */
export const REGISTERED_LANGUAGES: ReadonlySet<string> = new Set(
  Object.keys(GRAMMARS),
);

const EXTENSION_MAP: Readonly<Record<string, string>> = {
  '.ts': 'typescript',
  '.tsx': 'typescript',
  '.js': 'javascript',
  '.jsx': 'javascript',
  '.mjs': 'javascript',
  '.cjs': 'javascript',
  '.go': 'go',
  '.py': 'python',
  '.json': 'json',
  '.css': 'css',
  '.html': 'xml',
  '.xml': 'xml',
  '.svg': 'xml',
  '.sh': 'bash',
  '.bash': 'bash',
  '.zsh': 'bash',
  '.yml': 'yaml',
  '.yaml': 'yaml',
  '.sql': 'sql',
  '.md': 'markdown',
  '.markdown': 'markdown',
  '.rs': 'rust',
};

export function detectLanguage(filePath: string): string | null {
  const basename = filePath.slice(filePath.lastIndexOf('/') + 1);
  const dotIndex = basename.lastIndexOf('.');
  if (dotIndex === -1) return null;
  return EXTENSION_MAP[basename.slice(dotIndex).toLowerCase()] ?? null;
}

const highlight = Result.fromThrowable((code: string, language: string) =>
  hljs.highlight(code, { language }),
);

export function highlightCode(code: string, language: string | null): string {
  if (!code) return '';
  if (language === null) return escapeHtml(code);
  return highlight(code, language).match(
    (result) => result.value,
    () => escapeHtml(code),
  );
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}
