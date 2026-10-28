#!/usr/bin/env node
import { readFile, readdir, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { execute } from '../../harness/common.mjs';

const directory = dirname(fileURLToPath(import.meta.url));
const renderer = resolve(process.argv[2] ?? join(directory, '../extracted-smoke/.vite/renderer/main_window'));
const assets = join(renderer, 'assets');
const stylesheets = (await readdir(assets)).filter((name) => name.endsWith('.css'));
const css = (await Promise.all(stylesheets.map((name) => readFile(join(assets, name), 'utf8')))).join('\n');
const probes = [
  { feature: 'color-mix', token: 'color-mix(', property: 'color', value: 'color-mix(in oklab, red, blue)' },
  { feature: 'oklch', token: 'oklch(', property: 'color', value: 'oklch(50% 0.2 30)' },
  { feature: 'backdrop-filter', token: 'backdrop-filter', property: 'backdrop-filter', value: 'blur(4px)' },
  { feature: 'individual translate', token: 'translate:', property: 'translate', value: '1px 1px' },
  { feature: 'registered custom properties', token: '@property', api: 'registerProperty' },
];
const globalRoot = (await execute('npm', ['root', '-g'])).trim();
const playwright = await import(pathToFileURL(join(globalRoot, 'playwright/index.mjs')).href);
const engines = {};
for (const name of ['chromium', 'webkit']) {
  const browser = await playwright[name].launch({ headless: true }); // Functional API probes only, never rAF timing.
  engines[name] = await (async () => {
    const page = await browser.newPage();
    const supported = await page.evaluate((probes) => probes.map((probe) => probe.api ? typeof CSS[probe.api] === 'function' : CSS.supports(probe.property, probe.value)), probes);
    return { version: browser.version(), executable: playwright[name].executablePath(), features: probes.map((probe, index) => ({
      feature: probe.feature, occurrences: css.split(probe.token).length - 1, supported: supported[index],
    })) };
  })().finally(() => browser.close());
}
await writeFile(join(directory, 'css-results.json'), JSON.stringify({ renderer, stylesheets, engines,
  limitation: 'Current cached Playwright engines only, NOT Safari 16.1 or system WKWebView. Token counts are inventory, not a CSS parser. Cascade layers are present but not covered by CSS.supports.' }, null, 2) + '\n');
console.log(JSON.stringify(engines, null, 2));
