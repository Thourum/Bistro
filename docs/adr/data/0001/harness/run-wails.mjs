#!/usr/bin/env node
import { readFile, realpath, writeFile, stat } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';
import { execute } from './common.mjs';

const { values } = parseArgs({ options: Object.fromEntries(
  ['app', 'dmg', 'seed', 'out', 'renderer', 'engine', 'runs'].map((name) => [name, { type: 'string' }])
    .concat([['help', { type: 'boolean' }]])
) });
if (values.help) console.log('node run-wails.mjs --app <BistroWails.app> --dmg <ULMO dmg> --renderer <built renderer> --seed <seed> --out <empty dir> [--runs N] [--engine <bin>]\nReuses run-webview: 30 startup, 5 idle/post-diff memory, 10 frames/fixture; n=1 smoke only.');
else {
  for (const key of ['app', 'dmg', 'renderer', 'seed', 'out']) if (!values[key]) throw new Error(`--${key} is required`);
  const app = await realpath(values.app);
  const dmg = await realpath(values.dmg);
  const renderer = await realpath(values.renderer);
  const host = join(app, 'Contents', 'MacOS', 'wails-spike');
  const args = [join(dirname(fileURLToPath(import.meta.url)), 'run-webview.mjs'), '--host', host];
  for (const key of ['renderer', 'seed', 'out', 'engine', 'runs']) if (values[key]) args.push(`--${key}`, values[key]);
  // run-webview also requires key+visible window_state and invalidates the phase on later loss.
  // Reuse Gate 2's phase lifecycle, collector, statistics and explicit WebKit PID attribution unchanged.
  const FULL_RUN_TIMEOUT_MS = 24 * 60 * 60 * 1000;
  await execute(process.execPath, args, { env: { ...process.env, BISTRO_ADR_RENDERER: renderer },
    log: join(dirname(app), 'driver.log'), timeoutMs: FULL_RUN_TIMEOUT_MS });
  const manifestFile = join(values.out, 'run-manifest.json');
  const manifest = JSON.parse(await readFile(manifestFile, 'utf8'));
  const xctrace = await execute('xcrun', ['--find', 'xctrace']).then((path) => ({ available: true, path: path.trim() }),
    (error) => ({ available: false, error: error.message }));
  const sizes = { appAllocatedKiB: Number((await execute('du', ['-sk', app])).trim().split(/\s+/)[0]),
    dmgBytes: (await stat(dmg)).size, dmgFormat: 'ULMO', app, dmg };
  await writeFile(manifestFile, JSON.stringify({ ...manifest, shell: 'Wails v3.0.0',
    wailsCommit: '7b11001b92b712210b27d8cb53430e83f67ad8e4',
    bundleIdentifier: 'local.bistro.adr0001.wails-spike', xctrace,
    inspector: 'WKWebView.isInspectable=true; DevToolsEnabled=true; profiling not run', sizes }, null, 2) + '\n');
  console.log(`Wails output: ${values.out}`);
}
