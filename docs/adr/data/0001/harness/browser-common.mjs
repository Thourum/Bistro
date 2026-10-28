import { access, mkdir, readFile, readdir, realpath, rm, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { createCollector } from './collector.mjs';
import { launch, resetUserData, stopEngine, withTimeout, positiveInteger, MINUTE_MS } from './common.mjs';
import { serveRenderer } from './serve-renderer.mjs';
import { percentile, summarizeFrames } from './stats.mjs';

export const VIEWPORT = Object.freeze({ width: 1600, height: 900 });
export const FRAME_TIMEOUT_MS = 3 * MINUTE_MS;
export const IDLE_SETTLE_MS = 5000;
export const LOCAL_SETTINGS_KEY = 'bistro-settings-v1';
const WS_PORT = 8080;

/** Create an isolated output and serve the exact extracted production renderer. */
export async function prepareBrowserRun(values) {
  for (const name of ['seed', 'out', 'renderer']) if (!values[name]) return Promise.reject(new Error(`--${name} is required`));
  const seed = await realpath(values.seed);
  const renderer = await realpath(values.renderer);
  const runs = values.runs === undefined ? null : positiveInteger(values.runs, '--runs');
  const counts = { startup: runs ?? 30, memory: runs ?? 5, frames: runs ?? 10 };
  const out = resolve(values.out);
  if (out === seed || out.startsWith(seed + '/') || seed.startsWith(out + '/')) return Promise.reject(new Error('seed and output must be separate'));
  await mkdir(out, { recursive: true });
  if ((await readdir(out)).length) return Promise.reject(new Error('--out must be empty'));
  await writeFile(join(out, '.harness-owner.json'), JSON.stringify({ root: await realpath(out) }), { flag: 'wx' });
  const seedManifest = JSON.parse(await readFile(join(seed, 'seed-manifest.json'), 'utf8'));
  if (!Array.isArray(seedManifest.fixtures) || !seedManifest.fixtures.length || seedManifest.fixtures.some((f) => !/^[a-z0-9-]+$/.test(f.name) || typeof f.prId !== 'string')) return Promise.reject(new Error('invalid fixture manifest'));
  const engine = await realpath(values.engine ?? join(seed, 'bistro-sync'));
  await access(engine);
  if (!seedManifest.pinnedComplete) console.warn('SMOKE ONLY: pinned real fixtures absent');
  const server = await serveRenderer({ root: renderer });
  return { seed, out, renderer, counts, seedManifest, engine, server };
}

/** Refuse an occupied 8080 rather than connect to another engine or private user data. */
export async function startSeedEngine({ seed, out, engine, directory }) {
  const userData = join(directory, 'user-data');
  await resetUserData({ seed, userData, ownerRoot: out });
  const ownedEngine = launch(engine, ['-ws-port', String(WS_PORT), '-db-path', join(userData, 'sync.db'),
    '-auto-sync=false', '-replay-queue=false', '-exit-on-stdin-close'],
  { log: join(directory, 'engine.log'), env: { ...process.env, BISTRO_REPO_DIR: join(seed, 'repos') } });
  let isWaiting = true;
  const waitReady = async () => {
    while (isWaiting) {
      const text = await readFile(join(directory, 'engine.log'), 'utf8');
      if (text.split('\n').some((line) => line.startsWith('{') && line.includes('"event":"ready"') && line.includes('"wsPort":8080'))) return;
      await sleep(50);
    }
  };
  return withTimeout(Promise.race([waitReady(), ownedEngine.done.then(() => Promise.reject(new Error('engine exited before ready')))]), MINUTE_MS, 'engine ready')
    .then(() => ({ ownedEngine, userData }))
    .catch(async (error) => { await stopEngine(ownedEngine); return Promise.reject(error); })
    .finally(() => { isWaiting = false; });
}

/** Run one phase against a fresh DB and collector; persist errors and clean up on failure. */
export async function withBrowserPhase({ context, directory, prId, runPage }) {
  await mkdir(directory, { recursive: true });
  const engine = await startSeedEngine({ ...context, directory });
  const collector = await createCollector({ file: join(directory, 'events.jsonl') });
  const query = new URLSearchParams({ bench_collector: collector.url });
  if (prId) query.set('bench_diff', prId);
  const url = `${context.server.url}/?${query}`;
  return runPage({ url, collector, directory, ...engine }).finally(async () => {
    await collector.close().finally(() => stopEngine(engine.ownedEngine));
    await rm(engine.userData, { recursive: true, force: true });
  });
}

/** Frame-only report reuses Gate 1 statistics; never fabricates hyperfine or memory records. */
export async function summarizeBrowserOutput(context) {
  const fixtures = {};
  for (const fixture of context.seedManifest.fixtures) {
    const frames = [];
    for (let run = 1; run <= context.counts.frames; run++) {
      const file = join(context.out, 'frames', fixture.name, `run-${run}`, 'events.jsonl');
      const events = (await readFile(file, 'utf8')).trim().split('\n').filter(Boolean).map(JSON.parse);
      const frame = events.filter((event) => event.event === 'frames');
      if (frame.length !== 1 || frame[0].prId !== fixture.prId || !events.some((event) => event.event === 'diff_done') || events.some((event) => event.event === 'error')) return Promise.reject(new Error(`incomplete frames: ${file}`));
      frames.push(frame[0]);
    }
    fixtures[fixture.prId] = summarizeFrames(frames);
  }
  const summary = { schemaVersion: 1, smokeOnly: context.counts.frames < 10 || !context.seedManifest.pinnedComplete,
    fixtures, method: 'Gate 1 summarizeFrames; headed browser/native host, production bundle; no shell verdict' };
  await writeFile(join(context.out, 'frame-summary.json'), JSON.stringify(summary, null, 2) + '\n');
  return summary;
}

export function summarizeReceiptStartup(timesMs) {
  return { n: timesMs.length, p50: percentile(timesMs, 0.5), p95: percentile(timesMs, 0.95), p99: percentile(timesMs, 0.99),
    endpoint: 'driver spawn to collector pr_list_rendered receipt; engine already ready; warm/no purge',
    notComparableToHyperfineExit: true };
}
