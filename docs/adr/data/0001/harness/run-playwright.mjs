#!/usr/bin/env node
import { access, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { parseArgs } from 'node:util';
import { execute } from './common.mjs';
import { FRAME_TIMEOUT_MS, LOCAL_SETTINGS_KEY, VIEWPORT, prepareBrowserRun, withBrowserPhase,
  summarizeBrowserOutput, summarizeReceiptStartup } from './browser-common.mjs';

async function runPhase({ context, browserType, directory, prId }) {
  return withBrowserPhase({ context, directory, prId, runPage: async ({ url, collector }) => {
    const diagnostics = [];
    const launchedPerfMs = performance.now();
    const browser = await browserType.launch({ headless: false });
    return (async () => {
      const page = await browser.newPage({ viewport: VIEWPORT });
      page.on('console', (message) => diagnostics.push({ event: 'console', type: message.type(), text: message.text(), location: message.location() }));
      page.on('pageerror', (error) => diagnostics.push({ event: 'pageerror', message: error.message, stack: error.stack }));
      page.on('requestfailed', (request) => diagnostics.push({ event: 'requestfailed', url: request.url(), failure: request.failure() }));
      page.on('response', (response) => { if (response.status() >= 400) diagnostics.push({ event: 'http_error', url: response.url(), status: response.status() }); });
      const settings = JSON.parse(await readFile(join(context.seed, 'settings.json'), 'utf8'));
      await page.addInitScript(({ key, settings }) => localStorage.setItem(key, JSON.stringify(settings)), { key: LOCAL_SETTINGS_KEY, settings });
      await page.goto(url, { waitUntil: 'domcontentloaded', timeout: FRAME_TIMEOUT_MS });
      const receipt = await collector.waitFor('pr_list_rendered', { timeoutMs: FRAME_TIMEOUT_MS });
      const timing = { launchedPerfMs, receivedPerfMs: receipt.receivedPerfMs,
        elapsedMs: receipt.receivedPerfMs - launchedPerfMs, engineReadyBeforeLaunch: true };
      await writeFile(join(directory, 'launch-receipt.json'), JSON.stringify(timing, null, 2) + '\n');
      if (prId) await collector.waitFor('diff_done', { timeoutMs: FRAME_TIMEOUT_MS });
      collector.assertHealthy();
      return timing.elapsedMs;
    })().finally(async () => {
      await browser.close();
      await writeFile(join(directory, 'diagnostics.json'), JSON.stringify(diagnostics, null, 2) + '\n');
    });
  } });
}

const { values } = parseArgs({ options: Object.fromEntries(['browser', 'seed', 'out', 'renderer', 'engine', 'runs'].map((name) => [name, { type: 'string' }]).concat([['help', { type: 'boolean' }]])) });
if (values.help) console.log('node run-playwright.mjs --browser chromium|webkit --renderer <built renderer> --seed <dir> --out <empty dir> [--runs N] [--engine <bin>]\nHeaded; cached global Playwright 1.63.0 only. WebKit is NOT system WKWebView. Default 30 startup and 10 frame runs; --runs 1 for smoke.');
else {
  if (!['chromium', 'webkit'].includes(values.browser)) throw new Error('--browser must be chromium or webkit');
  const globalRoot = (await execute('npm', ['root', '-g'])).trim();
  const packageRoot = join(globalRoot, 'playwright');
  const version = JSON.parse(await readFile(join(packageRoot, 'package.json'), 'utf8')).version;
  if (version !== '1.63.0') throw new Error(`expected global Playwright 1.63.0, got ${version}`);
  const playwright = await import(pathToFileURL(join(packageRoot, 'index.mjs')).href);
  const browserType = playwright[values.browser];
  await access(browserType.executablePath()); // Never download browsers; fail if the pinned cache is absent.
  const context = await prepareBrowserRun(values);
  await (async () => {
    await writeFile(join(context.out, 'README.md'), '# Gate 2.1 headed smoke\n\n**Playwright WebKit is NOT system WKWebView.** This is a compatibility smoke test, not a shell verdict. The deciding renderer run is Gate 2.3’s native host on the supported macOS floor. n=1 and synthetic-only fixtures cannot establish S1. Keep the browser visible; headless rAF timing is meaningless. No memory measurement.\n');
    await writeFile(join(context.out, 'run-manifest.json'), JSON.stringify({ schemaVersion: 1, browser: values.browser, playwrightVersion: version,
      executable: browserType.executablePath(), renderer: context.renderer, counts: { startup: context.counts.startup, frames: context.counts.frames },
      seedManifest: context.seedManifest, viewport: VIEWPORT, headless: false, node: process.version,
      startupEndpoint: 'browser launch to collector pr_list_rendered receipt; engine prestarted; warm/no purge; not hyperfine exit' }, null, 2) + '\n');
    await writeFile(join(context.out, 'conditions.txt'), `Recorded ${new Date().toISOString()}\n${await execute('sw_vers', [])}\n${await execute('pmset', ['-g', 'batt'])}\n${await execute('pmset', ['-g', 'therm'])}\n${await execute('system_profiler', ['SPDisplaysDataType'])}`);
    const timesMs = [];
    for (let index = 1; index <= context.counts.startup; index++) timesMs.push(await runPhase({ context, browserType, directory: join(context.out, 'startup', `run-${index}`) }));
    await writeFile(join(context.out, 'startup-receipt.json'), JSON.stringify({ timesMs, summary: summarizeReceiptStartup(timesMs) }, null, 2) + '\n');
    for (const fixture of context.seedManifest.fixtures) for (let index = 1; index <= context.counts.frames; index++) {
      await runPhase({ context, browserType, directory: join(context.out, 'frames', fixture.name, `run-${index}`), prId: fixture.prId });
    }
    await summarizeBrowserOutput(context);
    await writeFile(join(context.out, 'completed.json'), JSON.stringify({ completedAt: new Date().toISOString() }) + '\n');
    console.log(`${values.browser} output: ${context.out}`);
  })().finally(() => context.server.close());
}
