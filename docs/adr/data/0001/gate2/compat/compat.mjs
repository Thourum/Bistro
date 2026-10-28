#!/usr/bin/env node
import { readFile, writeFile } from 'node:fs/promises';
import { dirname, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ESLint } from 'eslint';
import compat from 'eslint-plugin-compat';
import tseslint from 'typescript-eslint';

const directory = dirname(fileURLToPath(import.meta.url));
const root = resolve(directory, '../../../../../../');
const packageConfig = JSON.parse(await readFile(resolve(directory, 'package.json'), 'utf8'));
const targets = packageConfig.browserslist;
const eslint = new ESLint({ cwd: root, overrideConfigFile: true, overrideConfig: [{
  files: ['electron/src/renderer/**/*.{ts,tsx}'],
  languageOptions: { parser: tseslint.parser, parserOptions: { ecmaVersion: 'latest', sourceType: 'module', ecmaFeatures: { jsx: true } } },
  plugins: { compat }, rules: { 'compat/compat': 'error' },
  settings: { targets, lintAllEsApis: true },
}] });
const [sentinel] = await eslint.lintText('requestIdleCallback(() => {});', { filePath: 'electron/src/renderer/compat-sentinel.ts' });
if (!sentinel.messages.some((message) => message.ruleId === 'compat/compat')) throw new Error('compat checker failed Safari 16.1 sentinel');
const results = await eslint.lintFiles(['electron/src/renderer/**/*.{ts,tsx}']);
const formatter = await eslint.loadFormatter('stylish');
const findings = results.flatMap((result) => result.messages.map((message) => ({
  file: relative(root, result.filePath), line: message.line, column: message.column,
  rule: message.ruleId, message: message.message, fatal: Boolean(message.fatal),
})));
const byFeature = Object.groupBy(findings, (finding) => finding.message);
await writeFile(resolve(directory, 'compat-results.txt'), `Targets: ${targets.join(', ')}\nFiles checked: ${results.length}\n\n${formatter.format(results)}`.trimEnd() + '\n');
await writeFile(resolve(directory, 'compat-results.json'), JSON.stringify({ targets, filesChecked: results.length, findings }, null, 2) + '\n');
await writeFile(resolve(directory, 'compat-summary.md'), `# Gate 2.2 JavaScript API compatibility\n\nTarget: **Safari 16.1**, shipped with macOS Ventura 13.0. Source: [WebKit, 24 October 2022](https://webkit.org/blog/13399/webkit-features-in-safari-16-1/). Exact tool pins and resolved dependency versions are in package.json and pnpm-lock.yaml.\n\n${results.length} renderer TS/TSX files checked (including tests); ${findings.length} findings. ES API checks enabled (plugin labels these experimental). No polyfills claimed.\n\n| Feature / diagnostic | Count |\n| --- | ---: |\n${Object.entries(byFeature).map(([feature, rows]) => `| ${feature.replaceAll('|', '\\|')} | ${rows.length} |`).join('\n') || '| None | 0 |'}\n\nRaw locations: compat-results.txt and compat-results.json. This is syntax-based source analysis, not a verdict: instance methods, aliases, dependency code, runtime feature guards and Electron-only bridge calls need manual review. It does not check CSS. See ../tools.md for CSS probes and the minimum-OS limitation.\n`);
console.log(`${results.length} files; ${findings.length} findings`);
// Findings are evidence, not a failed collection. Parse/config failures remain fatal.
if (findings.some((finding) => finding.fatal)) process.exitCode = 1;
