#!/usr/bin/env node
import { mkdir, readFile, realpath, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { parseArgs } from 'node:util';
import { pathToFileURL } from 'node:url';
import { setTimeout as sleep } from 'node:timers/promises';
import { execute, launch, stopProcessGroup, withTimeout } from './common.mjs';
import { FRAME_TIMEOUT_MS, IDLE_SETTLE_MS, prepareBrowserRun, withBrowserPhase,
  summarizeBrowserOutput, summarizeReceiptStartup } from './browser-common.mjs';

async function sampleMemory({ host, hostEvents, engine, directory }) {
  const required = ['webcontent_pid', 'gpu_pid', 'networking_pid'];
  const identified = required.map((event) => hostEvents.findLast((row) => row.event === event));
  if (identified.some((row) => !Number.isSafeInteger(row?.pid) || row.pid <= 0)) return Promise.reject(new Error('missing WebKit PID SPI; refuse partial memory'));
  const pids = [...new Set([host.child.pid, engine.child.pid, ...identified.map((row) => row.pid)])];
  const listing = await execute('ps', ['-axo', 'pid=,ppid=,comm=']);
  const all = listing.trim().split('\n').map((line) => {
    const match = line.trim().match(/^(\d+)\s+(\d+)\s+(.+)$/);
    return match ? { pid: Number(match[1]), parentPid: Number(match[2]), command: match[3] } : null;
  }).filter(Boolean);
  const processes = pids.map((pid) => all.find((row) => row.pid === pid));
  if (processes.some((row) => !row) || !/bistro-sync$/.test(processes[1].command) || identified.some((row) => !/com\.apple\.WebKit\./.test(all.find((p) => p.pid === row.pid)?.command ?? ''))) return Promise.reject(new Error('attributed process disappeared or wrong binary'));
  await mkdir(directory, { recursive: true });
  await writeFile(join(directory, 'process-tree.json'), JSON.stringify(processes, null, 2) + '\n');
  const file = join(directory, 'footprint.json');
  await execute('footprint', [...pids.flatMap((pid) => ['-p', String(pid)]), '-j', file], { log: join(directory, 'footprint.txt'), timeoutMs: FRAME_TIMEOUT_MS });
  const raw = JSON.parse(await readFile(file, 'utf8'));
  const units = raw['bytes per unit'];
  if (!Array.isArray(raw.processes) || !Number.isFinite(units) || units <= 0 || (raw.errors?.length ?? Object.keys(raw.errors ?? {}).length)) return Promise.reject(new Error(`footprint failed: ${file}`));
  const samples = processes.map((row) => ({ ...row, bytes: raw.processes.find((sample) => sample.pid === row.pid)?.footprint * units }));
  if (samples.some((row) => !Number.isFinite(row.bytes) || row.bytes < 0)) return Promise.reject(new Error('incomplete footprint'));
  await writeFile(join(directory, 'sample.json'), JSON.stringify({ processes: samples,
    totalBytes: samples.reduce((sum, row) => sum + row.bytes, 0), sampledAt: new Date().toISOString(),
    method: 'host + standalone engine + explicit WebContent/Networking/GPU PID SPIs; shared allocations may be counted repeatedly' }, null, 2) + '\n');
}

/** Require a foreground host before measurement and invalidate the whole phase on focus/visibility loss. */
export function createWindowGuard() {
  let hasForeground = false;
  const foreground = Promise.withResolvers();
  const failed = Promise.withResolvers();
  return {
    ready: foreground.promise,
    failed: failed.promise,
    observe(row) {
      if (row.event === 'host_error') failed.reject(new Error(row.message));
      if (row.event !== 'window_state') return;
      if (row.isKey === true && row.isVisible === true) {
        hasForeground = true;
        foreground.resolve();
      } else if (hasForeground) failed.reject(new Error('WKWebView window lost key or visible status during measurement'));
    },
  };
}

async function runHostPhase({ context, hostBinary, directory, prId, memory }) {
  return withBrowserPhase({ context, directory, prId, runPage: async ({ url, collector, ownedEngine }) => {
    const launchedPerfMs = performance.now();
    const host = launch(hostBinary, [url, join(context.seed, 'settings.json')], { detached: true, log: join(directory, 'host.log') });
    const hostEvents = [];
    const windowGuard = createWindowGuard();
    let pending = '';
    host.child.stdout.setEncoding('utf8');
    host.child.stdout.on('data', (chunk) => {
      pending += chunk;
      const lines = pending.split('\n'); pending = lines.pop();
      // Host stdout is a JSONL protocol; keep anything else as evidence rather than crash the driver mid-run.
      for (const line of lines.filter(Boolean)) {
        const row = (() => { try { return JSON.parse(line); } catch { return { event: 'host_stdout_unparsed', line }; } })();
        hostEvents.push(row);
        windowGuard.observe(row);
      }
    });
    const exited = host.done.then(() => Promise.reject(new Error('host exited before phase completed')));
    const ready = Promise.all([
      withTimeout(windowGuard.ready, FRAME_TIMEOUT_MS, 'key and visible WKWebView window'),
      collector.waitFor('pr_list_rendered', { timeoutMs: FRAME_TIMEOUT_MS }),
    ]);
    const measurement = ready.then(async ([, receipt]) => {
      const timing = { launchedPerfMs, receivedPerfMs: receipt.receivedPerfMs,
        elapsedMs: receipt.receivedPerfMs - launchedPerfMs, engineReadyBeforeLaunch: true,
        windowKeyVisibleRequired: true };
      await writeFile(join(directory, 'launch-receipt.json'), JSON.stringify(timing, null, 2) + '\n');
      if (prId) await collector.waitFor('diff_done', { timeoutMs: FRAME_TIMEOUT_MS });
      if (memory) {
        await sleep(IDLE_SETTLE_MS);
        await sampleMemory({ host, hostEvents, engine: ownedEngine, directory: join(directory, 'memory') });
      }
      collector.assertHealthy();
      return timing.elapsedMs;
    });
    return Promise.race([measurement, exited, windowGuard.failed]).catch(async (error) => {
      await writeFile(join(directory, 'failed.json'), JSON.stringify({ failedAt: new Date().toISOString(), message: error.message }, null, 2) + '\n');
      return Promise.reject(error);
    }).finally(async () => {
      await stopProcessGroup(host);
      await writeFile(join(directory, 'host-events.json'), JSON.stringify(hostEvents, null, 2) + '\n');
    });
  } });
}

async function main() {
const { values } = parseArgs({ options: Object.fromEntries(['host', 'seed', 'out', 'renderer', 'engine', 'runs'].map((name) => [name, { type: 'string' }]).concat([['help', { type: 'boolean' }]])) });
if (values.help) console.log('node run-webview.mjs --host <compiled bin> --renderer <built renderer> --seed <dir> --out <empty dir> [--runs N] [--engine <bin>]\nDefaults: 30 startup launch-to-receipt, 5 idle memory, 10 frames per fixture. Keep window visible/frontmost. n=1 is smoke only.');
else {
  if (!values.host) await Promise.reject(new Error('--host is required'));
  const hostBinary = await realpath(values.host);
  const context = await prepareBrowserRun(values);
  const manifest = { schemaVersion: 1, hostBinary, renderer: context.renderer, node: process.version,
    counts: context.counts, seedManifest: context.seedManifest, viewport: { width: 1600, height: 900 },
    startupEndpoint: 'host launch to collector receipt; engine prestarted; warm/no purge; not Electron hyperfine launch-to-exit',
    memoryAttribution: 'private PID SPIs, not launchd ancestry' };
  await writeFile(join(context.out, 'run-manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
  await writeFile(join(context.out, 'conditions.txt'), `Recorded ${new Date().toISOString()}\n${await execute('sw_vers', [])}\n${await execute('pmset', ['-g', 'batt'])}\n${await execute('pmset', ['-g', 'therm'])}\n${await execute('system_profiler', ['SPDisplaysDataType'])}`);
  const run = (options) => runHostPhase({ context, hostBinary, ...options });
  await (async () => {
    const timesMs = [];
    for (let index = 1; index <= context.counts.startup; index++) timesMs.push(await run({ directory: join(context.out, 'startup', `run-${index}`) }));
    await writeFile(join(context.out, 'startup-receipt.json'), JSON.stringify({ timesMs, summary: summarizeReceiptStartup(timesMs) }, null, 2) + '\n');
    for (let index = 1; index <= context.counts.memory; index++) await run({ directory: join(context.out, 'idle', `run-${index}`), memory: true });
    for (const fixture of context.seedManifest.fixtures) for (let index = 1; index <= context.counts.frames; index++) {
      await run({ directory: join(context.out, 'frames', fixture.name, `run-${index}`), prId: fixture.prId, memory: index <= context.counts.memory });
    }
    await summarizeBrowserOutput(context);
    await writeFile(join(context.out, 'completed.json'), JSON.stringify({ completedAt: new Date().toISOString() }) + '\n');
    console.log(`WKWebView output: ${context.out}`);
  })().finally(() => context.server.close());
}
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) await main();
