#!/usr/bin/env node
import { access, mkdir, readFile, readdir, realpath, rm, stat, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';
import { setTimeout as sleep } from 'node:timers/promises';
import { createCollector } from './collector.mjs';
import { execute, launch, withTimeout, quoteShell, positiveInteger, MINUTE_MS, resetUserData, stopProcessGroup } from './common.mjs';

const SCRIPT = fileURLToPath(import.meta.url);
const STARTUP_RUNS = 30;
const MEMORY_RUNS = 5;
const FRAME_RUNS = 10;
const IDLE_SETTLE_MS = 2000;
const FRAME_TIMEOUT_MS = 2 * MINUTE_MS;

function registerInterruption() {
  let rejectInterrupted;
  const interrupted = new Promise((_, reject) => { rejectInterrupted = reject; });
  interrupted.catch(() => {});
  const signals = ['SIGINT', 'SIGTERM'].map((signal) => {
    const handler = () => rejectInterrupted(new Error(`measurement interrupted by ${signal}`));
    process.on(signal, handler);
    return { signal, handler };
  });
  return { interrupted, dispose() { for (const { signal, handler } of signals) process.removeListener(signal, handler); } };
}

async function cleanupRun({ appProcess, collector, interruption, userData }) {
  let isStopped = false;
  try { if (appProcess) await stopProcessGroup(appProcess); isStopped = true; }
  finally {
    try { await collector.close(); }
    finally {
      interruption.dispose();
      // Keep the profile if subprocess shutdown failed; never delete a DB still in use.
      if (isStopped) await rm(userData, { recursive: true, force: true });
    }
  }
}

function appArguments({ userData, collector, prId, quit }) {
  const query = new URLSearchParams({ bench_collector: collector });
  if (prId) query.set('bench_diff', prId);
  return [`--user-data-dir=${userData}`, `--bench-query=${encodeURIComponent(query.toString())}`, ...(quit ? ['--quit-after-ready'] : [])];
}

function benchEnvironment(seed) {
  // Shells spawned by Electron-based tools inherit this; unfused builds would then start as plain Node.
  const { ELECTRON_RUN_AS_NODE: _runAsNode, ...inherited } = process.env;
  return { ...inherited, BISTRO_REPO_DIR: join(seed, 'repos'), BISTRO_DISABLE_AUTO_UPDATES: '1',
    BISTRO_ENABLE_AUTO_UPDATES: '0', BISTRO_SKIP_GO_SPAWN: '0', WS_PORT: '0' };
}

async function recordConditions(out) {
  const commands = [
    ['macOS', 'sw_vers', []], ['hardware', 'sysctl', ['-n', 'hw.model']],
    ['power source', 'pmset', ['-g', 'batt']], ['power settings (includes lowpowermode)', 'pmset', ['-g']],
    ['thermal', 'pmset', ['-g', 'therm']], ['displays (refresh rate / UI Looks like)', 'system_profiler', ['SPDisplaysDataType']],
  ];
  const reports = await Promise.all(commands.map(async ([label, command, args]) => {
    try { return `## ${label}\n${await execute(command, args)}`; }
    catch (error) { return `## ${label}\nUNAVAILABLE: ${error.message}\n`; }
  }));
  let purge;
  try { await execute('sudo', ['-n', 'true']); purge = true; }
  catch { purge = false; }
  await writeFile(join(out, 'conditions.txt'), `Recorded: ${new Date().toISOString()}\nCache condition: ${purge ? 'purge before startup runs (sudo -n purge)' : 'warm, no purge (sudo -n true unavailable)'}\n${reports.join('\n')}`);
  return purge;
}

async function recordSizes({ app, dmg, out }) {
  const appAllocated = await execute('du', ['-sk', app]);
  const binaryStat = await stat(join(app, 'Contents/MacOS/Bistro'));
  const sizes = { appAllocatedKiB: Number(appAllocated.trim().split(/\s+/)[0]), appBinaryBytes: binaryStat.size };
  let raw = `Installed .app allocated size (du -sk):\n${appAllocated}\nApp binary stat bytes: ${binaryStat.size}\n`;
  if (dmg) {
    sizes.dmgBytes = (await stat(dmg)).size;
    raw += `DMG stat bytes: ${sizes.dmgBytes}\nDMG allocated size (du -sk):\n${await execute('du', ['-sk', dmg])}`;
  } else raw += 'DMG: not supplied\n';
  await writeFile(join(out, 'sizes.txt'), raw);
  await writeFile(join(out, 'sizes.json'), `${JSON.stringify(sizes, null, 2)}\n`);
}

async function waitForEvent({ collector, name, appProcess }) {
  return Promise.race([
    collector.waitFor(name, { timeoutMs: FRAME_TIMEOUT_MS }),
    appProcess.done.then(() => { throw new Error(`app exited before ${name}`); }),
  ]);
}

async function processTree({ rootPid, cancelled }) {
  const listing = await execute('ps', ['-axo', 'pid=,ppid=,comm='], { timeoutMs: MINUTE_MS, cancelled });
  const rows = listing.trim().split('\n').map((line) => {
    const match = line.trim().match(/^(\d+)\s+(\d+)\s+(.+)$/);
    if (!match) throw new Error(`unparseable ps row: ${line}`);
    return { pid: Number(match[1]), parentPid: Number(match[2]), command: match[3] };
  });
  const pids = new Set([rootPid]);
  let previousSize;
  do {
    previousSize = pids.size;
    for (const row of rows) if (pids.has(row.parentPid)) pids.add(row.pid);
  } while (pids.size !== previousSize);
  const tree = rows.filter((row) => pids.has(row.pid));
  if (!tree.some((row) => /(?:bistro-sync|go-bin)$/.test(row.command))) throw new Error('process tree has no bistro-sync/go-bin; refuse partial memory measurement');
  return tree;
}

async function sampleMemory({ appProcess, directory, cancelled }) {
  await mkdir(directory, { recursive: true });
  const tree = await processTree({ rootPid: appProcess.child.pid, cancelled });
  await writeFile(join(directory, 'process-tree.json'), `${JSON.stringify(tree, null, 2)}\n`);
  const rawFile = join(directory, 'footprint.json');
  await execute('footprint', [...tree.flatMap(({ pid }) => ['-p', String(pid)]), '-j', rawFile],
    { log: join(directory, 'footprint.txt'), timeoutMs: MINUTE_MS, cancelled });
  const raw = JSON.parse(await readFile(rawFile, 'utf8'));
  if (!Array.isArray(raw.processes) || (raw.errors?.length ?? Object.keys(raw.errors ?? {}).length)) throw new Error(`footprint failed; see ${rawFile}`);
  const multiplier = raw['bytes per unit'];
  if (!Number.isFinite(multiplier) || multiplier <= 0) throw new Error('footprint missing bytes per unit');
  const processes = tree.map(({ pid, command }) => {
    const entry = raw.processes.find((row) => row.pid === pid);
    if (!entry || !Number.isFinite(entry.footprint) || entry.footprint < 0) throw new Error(`footprint missing PID ${pid}`);
    return { pid, command, bytes: entry.footprint * multiplier };
  });
  const totalBytes = processes.reduce((sum, row) => sum + row.bytes, 0);
  await writeFile(join(directory, 'sample.json'), `${JSON.stringify({ sampledAt: new Date().toISOString(), processes, totalBytes,
    method: 'sum of per-process footprint (shared allocations may be counted in more than one process)' }, null, 2)}\n`);
}

async function measureStartup({ binary, seed, out, runs, purge }) {
  const directory = join(out, 'startup');
  await mkdir(directory, { recursive: true });
  const userData = join(directory, 'user-data');
  const collector = await createCollector({ file: join(directory, 'events.jsonl') });
  const interruption = registerInterruption();
  let benchmark;
  try {
    const command = [binary, ...appArguments({ userData, collector: collector.url, quit: true })].map(quoteShell).join(' ');
    const prepare = [process.execPath, SCRIPT, '--prepare-user-data', userData, '--owner-root', out, '--seed', seed].map(quoteShell).join(' ') + (purge ? ' && sudo -n purge' : '');
    await writeFile(join(directory, 'command.txt'), `${command}\nPrepare: ${prepare}\n`);
    benchmark = launch('hyperfine', ['--shell', 'sh', '--warmup', '1', '--runs', String(runs),
      '--export-json', join(out, 'startup.json'), '--prepare', prepare,
      `${command} >> ${quoteShell(join(directory, 'app.log'))} 2>&1`],
    { detached: true, env: benchEnvironment(seed), log: join(directory, 'hyperfine.log') });
    benchmark.child.stdin.end();
    await Promise.race([interruption.interrupted, collector.failed, withTimeout(benchmark.done, (runs + 1) * FRAME_TIMEOUT_MS, 'hyperfine startup')]);
    collector.assertHealthy();
    const receipts = (await readFile(join(directory, 'events.jsonl'), 'utf8')).trim().split('\n').map((line) => JSON.parse(line));
    if (receipts.filter((event) => event.event === 'pr_list_rendered').length !== runs + 1) throw new Error('startup missing PR-list ready events (including warmup)');
  } finally { await cleanupRun({ appProcess: benchmark, collector, interruption, userData }); }
}

async function measureInteractive({ binary, seed, out, fixture, run, memory }) {
  const phase = fixture ? `frames/${fixture.name}` : 'idle';
  const directory = join(out, phase, `run-${run}`);
  await mkdir(directory, { recursive: true });
  const userData = join(directory, 'user-data');
  await resetUserData({ seed, userData, ownerRoot: out });
  const collector = await createCollector({ file: join(directory, 'events.jsonl') });
  const interruption = registerInterruption();
  let appProcess;
  try {
    appProcess = launch(binary, appArguments({ userData, collector: collector.url, prId: fixture?.prId }),
      { detached: true, env: benchEnvironment(seed), log: join(directory, 'app.log') });
    const unexpectedExit = appProcess.done.then(() => { throw new Error('app exited before intentional shutdown'); });
    const cancelled = Promise.race([interruption.interrupted, collector.failed, unexpectedExit]);
    cancelled.catch(() => {});
    await Promise.race([cancelled, waitForEvent({ collector, name: fixture ? 'diff_done' : 'pr_list_rendered', appProcess })]);
    if (fixture) {
      const frames = await Promise.race([cancelled, collector.waitFor('frames')]);
      if (frames.prId !== fixture.prId || !Array.isArray(frames.deltas) || !frames.deltas.length) throw new Error('missing or wrong fixture frames');
    }
    await Promise.race([cancelled, sleep(IDLE_SETTLE_MS)]);
    if (memory) await sampleMemory({ appProcess, directory: join(directory, 'memory'), cancelled });
    await Promise.race([cancelled, Promise.resolve()]);
    if (appProcess.child.exitCode !== null || appProcess.child.signalCode !== null) throw new Error('app exited during measurement');
    collector.assertHealthy();
  } finally { await cleanupRun({ appProcess, collector, interruption, userData }); }
}

async function main() {
  const { values } = parseArgs({ options: Object.fromEntries(['app', 'label', 'seed', 'out', 'runs', 'dmg', 'prepare-user-data', 'owner-root'].map((name) => [name, { type: 'string' }]).concat([['help', { type: 'boolean' }]])) });
  if (values.help) {
    console.log('Usage: node run-electron.mjs --app <path.app> --label <name> --seed <seedDir> --out <dir> [--runs N] [--dmg <file>]\nDefaults: 30 startup, 5 idle-memory, 10 frame runs per fixture; post-diff memory on first 5. --runs overrides all counts. Requires macOS, Node 24, hyperfine and footprint. Builds nothing.');
    return;
  }
  if (values['prepare-user-data']) {
    if (!values.seed || !values['owner-root']) throw new Error('--seed and --owner-root are required for internal preparation');
    await resetUserData({ seed: resolve(values.seed), userData: resolve(values['prepare-user-data']), ownerRoot: resolve(values['owner-root']) });
    return;
  }
  for (const key of ['app', 'label', 'seed', 'out']) if (!values[key]) throw new Error(`--${key} is required`);
  if (process.platform !== 'darwin') throw new Error('run-electron requires macOS footprint/purge');
  const app = await realpath(resolve(values.app)), seed = await realpath(resolve(values.seed));
  await mkdir(resolve(values.out), { recursive: true });
  const out = await realpath(resolve(values.out));
  if (out === seed || out.startsWith(`${seed}/`) || seed.startsWith(`${out}/`)) throw new Error('--out and --seed must be separate directories');
  if ((await readdir(out)).length) throw new Error('--out must be empty (do not mix runs)');
  await writeFile(join(out, '.harness-owner.json'), JSON.stringify({ root: out }), { flag: 'wx' });
  const binary = join(app, 'Contents/MacOS/Bistro');
  await access(binary);
  const manifest = JSON.parse(await readFile(join(seed, 'seed-manifest.json'), 'utf8'));
  if (!Array.isArray(manifest.fixtures) || manifest.fixtures.length < 2) throw new Error('seed manifest must contain at least two fixtures');
  if (!manifest.pinnedComplete) console.warn('WARNING: pinned GitHub PR fixtures absent; smoke run only, not Gate 1 evidence');
  if (!manifest.syntheticComplete) console.warn('WARNING: synthetic fixtures absent; this is an incomplete Gate 1 measurement');
  for (const fixture of manifest.fixtures) if (!/^[a-z0-9-]+$/.test(fixture.name) || typeof fixture.prId !== 'string') throw new Error('invalid seed fixture');
  const override = values.runs ? positiveInteger(values.runs, '--runs') : null;
  const counts = { startup: override ?? STARTUP_RUNS, memory: override ?? MEMORY_RUNS, frames: override ?? FRAME_RUNS };
  await writeFile(join(out, 'run-manifest.json'), `${JSON.stringify({ label: values.label, app, seed, node: process.version, counts, seedManifest: manifest }, null, 2)}\n`);
  await recordSizes({ app, dmg: values.dmg ? resolve(values.dmg) : null, out });
  const purge = await recordConditions(out);
  console.log('Startup phase');
  await measureStartup({ binary, seed, out, runs: counts.startup, purge });
  console.log('Idle memory phase');
  for (let run = 1; run <= counts.memory; run++) await measureInteractive({ binary, seed, out, run, memory: true });
  for (const fixture of manifest.fixtures) {
    console.log(`Large-diff phase: ${fixture.prId}`);
    for (let run = 1; run <= counts.frames; run++) await measureInteractive({ binary, seed, out, fixture, run, memory: run <= counts.memory });
  }
  await writeFile(join(out, 'completed.json'), JSON.stringify({ completedAt: new Date().toISOString() }));
  console.log(`Raw measurements: ${out}`);
}

await main();
