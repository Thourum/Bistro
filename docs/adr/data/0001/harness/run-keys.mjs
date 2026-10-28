#!/usr/bin/env node
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { createKeysCollector, openNative, timeout } from './keys/driver-io.mjs';
import { readFileSync, writeFileSync, appendFileSync, existsSync, mkdirSync } from 'node:fs';
import { resolve, join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, execFileSync, spawnSync } from 'node:child_process';
import { resetUserData } from './common.mjs';
import { parseArgs } from 'node:util';
import { setTimeout as delay } from 'node:timers/promises';
import { parseInventory } from '../gate3/bindings-by-class.mjs';

const DIRECTORY = dirname(fileURLToPath(import.meta.url));
const SMOKE_LIMIT = 10;
const TIMEOUT_MS = 15_000;
const SETTLE_MS = 250;
// Owner scope (2026-10-28): non-US layouts and Japanese IME are under 1% of potential users.
const LAYOUTS = ['US'];
const INJECTORS = ['osascript', 'CGEventPost'];
const MODIFIERS = { Mod: 'cmd', Meta: 'cmd', Control: 'ctrl', Alt: 'alt', Shift: 'shift' };
const LOCAL_SUBMIT = 'textarea.submit';

/** Stable source-only matrix; each cell includes its actual chord, not just action ID. */
export function enumerateCells(inventory = parseInventory()) {
  return Object.entries(inventory.tables).flatMap(([style, table]) => {
    const bindings = { ...table, [LOCAL_SUBMIT]: 'Mod+Enter', 'textarea.submit.ctrl': 'Control+Enter' };
    return Object.keys(bindings).filter((id) => id !== 'global.quit.win').sort().flatMap((bindingId) => {
      const actionId = bindingId === 'textarea.submit.ctrl' ? LOCAL_SUBMIT : bindingId;
      const steps = Array.isArray(bindings[bindingId]) ? bindings[bindingId] : [bindings[bindingId]];
      return LAYOUTS.flatMap((layout) => INJECTORS.flatMap((injection) =>
        ['outside', 'inside'].map((focus) => {
          const cell = { actionId, style, steps, layout, injection, focus };
          return { id: fingerprint(cell), ...cell };
        })));
    });
  });
}

export function fingerprint(value) {
  return createHash('sha256').update(JSON.stringify(value)).digest('hex');
}

/** Full runs require opt-in; live smoke always requires an explicit bounded limit. */
export function selectCells(cells, values) {
  assert.ok(!(values.full && values.limit), 'use --full or --limit, not both');
  if (values.full) return cells;
  if (values['dry-run'] && !values.limit) return cells.slice(0, SMOKE_LIMIT);
  assert.ok(values.limit && /^\d+$/.test(values.limit), '--limit 1..10 is required (or explicit --full)');
  const limit = Number(values.limit);
  assert.ok(limit > 0 && limit <= SMOKE_LIMIT, '--limit must be 1..10; full matrix requires --full');
  return cells.slice(0, limit);
}

export function preflightBlockers(preflight, selfTest) {
  const blockers = [];
  if (selfTest?.ok !== true) blockers.push('native self-test failed');
  for (const field of ['postEventAccess', 'accessibilityTrusted', 'passiveTapCreated']) {
    if (preflight?.[field] !== true) blockers.push(`${field} unavailable`);
  }
  for (const layout of LAYOUTS) {
    if (!preflight?.inventory?.required?.[layout]?.enabledIDs?.length) blockers.push(`${layout} source disabled or missing`);
  }
  return blockers;
}

/** Resume only successful complete groups; duplicate groups or foreign IDs are rejected. */
export function resumeCells(manifest, expected, groups) {
  assert.equal(manifest.fingerprint, expected.fingerprint, 'resume fingerprint mismatch');
  const ids = new Set(expected.cells.map((cell) => cell.id));
  const completed = new Set();
  for (const group of groups) {
    assert.ok(ids.has(group.cell?.id), 'journal contains foreign cell');
    if (group.status !== 'complete') continue;
    assert.ok(!completed.has(group.cell.id), 'duplicate completed cell');
    completed.add(group.cell.id);
  }
  return completed;
}

function nativeRows(binary, command) {
  return execFileSync(binary, [command], { encoding: 'utf8', timeout: TIMEOUT_MS }).trim().split('\n').filter(Boolean).map(JSON.parse);
}

function parseStep(step) {
  const parts = step.split('+');
  const key = parts.pop();
  assert.ok(key, `invalid chord ${step}`);
  const modifiers = parts.map((part) => { assert.ok(MODIFIERS[part], `unknown modifier ${part}`); return MODIFIERS[part]; });
  return { key, modifiers };
}

function appleScriptKey({ key, modifiers }) {
  const named = { Escape: 53, Enter: 36, Tab: 48, Space: 49, ArrowLeft: 123, ArrowRight: 124, ArrowDown: 125, ArrowUp: 126, Home: 115, End: 119, PageUp: 116, PageDown: 121, F1: 122 };
  const operation = named[key] === undefined ? `keystroke ${JSON.stringify(key.toLowerCase())}` : `key code ${named[key]}`;
  const names = { cmd: 'command down', ctrl: 'control down', alt: 'option down', shift: 'shift down' };
  return `tell application "System Events" to ${operation}${modifiers.length ? ` using {${modifiers.map((modifier) => names[modifier]).join(', ')}}` : ''}`;
}

async function runCell({ cell, collector, worker, tap, preflight }) {
  collector.command(cell, 'prepare');
  const prepared = await collector.waitFor('keys_prepared', cell.id);
  if (prepared.supported !== true || prepared.targetKind === 'unsupported') return { cell, status: 'incomplete', reason: prepared.reason ?? 'unsupported fixture route', prepared };
  if (cell.focus === 'inside' && prepared.targetKind !== 'textarea') return { cell, status: 'incomplete', reason: 'real comment textarea unavailable', prepared };
  if (cell.actionId.startsWith('global.quit') || cell.steps.some((step) => /^(?:Meta|Mod)\+Q$/i.test(step))) {
    return { cell, status: 'incomplete', reason: 'excluded from injection: terminates shell; manual check pending', prepared };
  }
  if (cell.actionId === LOCAL_SUBMIT && !prepared.text?.trim()) return { cell, status: 'incomplete', reason: 'non-empty real composer required for submit observation', prepared };
  if (cell.focus === 'outside' && prepared.intendedBindingActive !== true) return { cell, status: 'incomplete', reason: 'intended handler not reachable in fixture', prepared };
  const source = preflight.inventory.required[cell.layout].enabledIDs[0];
  await worker.command({ command: 'source', source });
  const tapStart = tap.rows.length;
  for (const step of cell.steps) {
    const key = parseStep(step);
    await worker.command({ command: 'guard' });
    if (cell.injection === 'CGEventPost') await worker.command({ command: 'inject', source, ...key });
    else execFileSync('osascript', ['-e', appleScriptKey(key)], { timeout: TIMEOUT_MS });
    await delay(SETTLE_MS);
  }
  collector.command(cell, 'snapshot');
  const snapshot = await collector.waitFor('keys_snapshot', cell.id);
  const events = collector.rows.filter((row) => row.cellId === cell.id);
  assert.ok(!tap.rows.slice(tapStart).some((row) => row.event === 'tap_disabled'), 'native tap disabled; abort entire run');
  const nativeTap = tap.rows.slice(tapStart).filter((row) => row.event === 'cg_event');
  const dom = events.filter((row) => ['keys_keydown', 'keys_input', 'keys_composition'].includes(row.event));
  const actionIds = events.filter((row) => row.event === 'keys_action').map((row) => row.actionId);
  const status = nativeTap.length && dom.some((row) => row.event === 'keys_keydown') ? 'complete' : 'incomplete';
  return { cell, status, reason: status === 'incomplete' ? 'native or DOM key observation missing' : undefined,
    source, prepared, nativeTap, dom, actionIds, text: snapshot.text, snapshot };
}

/** Request restoration even after a failed command; completion requires a clean worker exit. */
export async function restoreNative(worker) {
  if (!worker) return;
  if (worker.child.exitCode === null) worker.child.kill('SIGTERM');
  const exit = await timeout(worker.exited, 'native source restoration');
  assert.equal(exit.code, 0, 'native restoration worker failed');
  assert.ok(!worker.rows.some((row) => row.event === 'restored' && row.ok !== true), 'native input-source restoration failed');
}

/** Shared live-driver lifecycle: never publish completion before restoration succeeds. */
export async function runNativeLifecycle({ worker, run, complete }) {
  return Promise.resolve().then(run).then(async () => {
    await worker.command({ command: 'stop' });
    const exit = await timeout(worker.exited, 'native restoration');
    assert.equal(exit.code, 0, 'native restoration worker failed');
    assert.ok(!worker.rows.some((row) => row.event === 'restored' && row.ok !== true), 'native input-source restoration failed');
    await complete();
  }).finally(() => restoreNative(worker));
}

async function liveRun(values, manifest, preflight) {
  const out = resolve(values.out);
  assert.ok(values.pid || values.app || values['launch-command'], '--pid, --app, or --launch-command required');
  assert.ok(!values.pid, '--pid attach currently unsupported: disposable profile/queue ownership cannot be proven; launch a harness-owned target');
  assert.ok(!(values.app && values['launch-command']), '--app and --launch-command are exclusive');
  const collector = await createKeysCollector(join(out, 'renderer.jsonl'));
  const resources = { collector };
  let cleanupPromise;
  const cleanup = () => {
    cleanupPromise ??= Promise.resolve().then(async () => {
      resources.tap?.child.kill('SIGTERM');
      await restoreNative(resources.worker).finally(async () => {
        resources.app?.kill('SIGTERM');
        await collector.close();
      });
    });
    return cleanupPromise;
  };
  const interrupt = () => { void cleanup(); process.exitCode = 1; };
  process.once('SIGINT', interrupt); process.once('SIGTERM', interrupt);
  return Promise.resolve().then(async () => {
    const query = new URLSearchParams({ bench_collector: collector.url, bench_keys: '1', bench_keys_pr: values['fixture-pr'] ?? '' }).toString();
    assert.ok(values.seed, '--seed is required; owner profile is never used');
    const phase = join(out, 'session');
    mkdirSync(phase, { recursive: true });
    writeFileSync(join(out, '.harness-owner.json'), JSON.stringify({ root: out }));
    const userData = join(phase, 'user-data');
    await resetUserData({ seed: resolve(values.seed), userData, ownerRoot: out });
    const queueCount = execFileSync('sqlite3', ['-readonly', join(userData, 'sync.db'), 'SELECT count(*) FROM action_queue;'], { encoding: 'utf8' }).trim();
    assert.equal(queueCount, '0', 'disposable seed has queued writes; refusing launch/replay');
    writeFileSync(join(out, 'profile-preflight.json'), JSON.stringify({ actionQueueCount: 0, databaseReadOnlyChecked: true, userData }));
    const settingsFile = join(userData, 'settings.json');
    const settings = JSON.parse(readFileSync(settingsFile, 'utf8'));
    const isolatedSettings = { ...settings, sync: { ...settings.sync, autoSync: false, notifications: false },
      privacy: { ...settings.privacy, telemetryEnabled: false },
      classification: { ...settings.classification, mode: 'rules', dismissalScope: 'local' } };
    writeFileSync(settingsFile, JSON.stringify(isolatedSettings, null, 2));
    writeFileSync(join(out, 'profile-settings.json'), JSON.stringify(isolatedSettings, null, 2));
    const args = values['launch-args'] ? JSON.parse(values['launch-args']) : [];
    assert.ok(Array.isArray(args) && args.every((arg) => typeof arg === 'string'), '--launch-args must be JSON string array');
    let pid = Number(values.pid);
    if (!pid) {
      const bundleExecutable = values.app ? execFileSync('/usr/libexec/PlistBuddy', ['-c', 'Print :CFBundleExecutable', join(resolve(values.app), 'Contents/Info.plist')], { encoding: 'utf8' }).trim() : undefined;
      assert.ok(!bundleExecutable || /^[^/]+$/.test(bundleExecutable), 'invalid bundle executable');
      const executable = values.app ? join(resolve(values.app), 'Contents/MacOS', bundleExecutable) : values['launch-command'];
      resources.app = spawn(executable, [...args, `--user-data-dir=${userData}`, `--bench-query=${encodeURIComponent(query)}`], {
        stdio: 'ignore', env: { ...process.env, WS_PORT: '0', BISTRO_SKIP_GO_SPAWN: '0', BISTRO_DISABLE_AUTO_UPDATES: '1', BISTRO_REPO_DIR: join(resolve(values.seed), 'repos') },
      });
      resources.app.on('error', (error) => console.error(error.message));
      pid = resources.app.pid;
    } else console.log(`Attach collector query to PID ${pid}: ${query}`);
    assert.ok(Number.isSafeInteger(pid) && pid > 0, 'invalid target PID');
    const ready = await collector.waitFor('keys_ready');
    assert.ok(ready.externalWritesBlocked === true && ready.dryDispatch === true && ready.classificationMode === 'rules', 'renderer read-only/rules/dry-dispatch proof missing');
    writeFileSync(join(out, 'renderer-ready.json'), JSON.stringify(ready, null, 2));
    assert.ok(/^github:bench\/large-diff:[12]$/.test(ready.prId ?? ''), 'synthetic fixture proof missing');
    writeFileSync(join(out, 'run-manifest.json'), JSON.stringify({ ...manifest,
      readinessProof: { classificationMode: ready.classificationMode, externalWritesBlocked: ready.externalWritesBlocked,
        dryDispatch: ready.dryDispatch, prId: ready.prId, receipt: 'renderer-ready.json',
        profilePreflight: 'profile-preflight.json', profileSettings: 'profile-settings.json',
        engineEnvironment: { WS_PORT: '0', BISTRO_SKIP_GO_SPAWN: '0' } },
    }, null, 2));
    resources.worker = openNative(values.native, 'worker', pid, join(out, 'worker.jsonl'));
    await resources.worker.ready;
    resources.tap = openNative(values.native, 'tap', pid, join(out, 'native-tap.jsonl'));
    await resources.tap.ready;
    const journal = join(out, 'cells.jsonl');
    const groups = existsSync(journal) ? readFileSync(journal, 'utf8').trim().split('\n').filter(Boolean).map(JSON.parse) : [];
    const completed = resumeCells(manifest, manifest, groups);
    await runNativeLifecycle({
      worker: resources.worker,
      run: async () => {
        for (const cell of manifest.cells.filter((entry) => !completed.has(entry.id))) {
          const group = await runCell({ cell, collector, worker: resources.worker, tap: resources.tap, preflight });
          appendFileSync(journal, `${JSON.stringify(group)}\n`);
        }
      },
      complete: () => writeFileSync(join(out, 'completed.json'), JSON.stringify({ fingerprint: manifest.fingerprint, requested: manifest.cells.length })),
    });
  }).finally(async () => {
    process.removeListener('SIGINT', interrupt); process.removeListener('SIGTERM', interrupt);
    await cleanup();
  });
}

async function main() {
  const { values } = parseArgs({ options: {
    ...Object.fromEntries(['app', 'pid', 'launch-command', 'launch-args', 'fixture-pr', 'seed', 'out', 'limit', 'label', 'native', 'style', 'layout', 'actionId'].map((name) => [name, { type: 'string' }])),
    ...Object.fromEntries(['dry-run', 'preflight-only', 'full', 'resume', 'help'].map((name) => [name, { type: 'boolean' }])),
  } });
  if (values.help) {
    console.log('run-keys.mjs --dry-run [--limit 1..10] | --out DIR --label NAME (--app APP|--pid PID|--launch-command EXEC) --limit 1..10 [--launch-args JSON] [--fixture-pr ID] [--resume]\nFull matrix requires --full. --preflight-only never launches/injects. --launch-command is one executable, not a shell expression.');
    return;
  }
  const cells = enumerateCells();
  const filtered = cells.filter((cell) => ['style', 'layout', 'actionId'].every((field) =>
    !values[field] || cell[field] === values[field]));
  assert.ok(filtered.length, 'matrix filters matched no cells');
  const selected = selectCells(filtered, values);
  const targetFiles = [values['launch-command'], values.app && join(resolve(values.app), 'Contents/Info.plist'), values.app && join(resolve(values.app), 'Contents/Resources/app.asar'), values.seed && join(resolve(values.seed), 'seed-manifest.json')].filter((file) => file && existsSync(file));
  const targetDigests = targetFiles.map((file) => ({ file: resolve(file), sha256: createHash('sha256').update(readFileSync(file)).digest('hex') }));
  const identity = { schemaVersion: 1, target: { app: values.app, launch: values['launch-command'], args: values['launch-args'], fixture: values['fixture-pr'], seed: values.seed, label: values.label, targetDigests }, cells: selected };
  const manifest = { ...identity, fingerprint: fingerprint(identity), totalMatrixCells: cells.length, smokeOnly: !values.full };
  if (values['dry-run']) { console.log(JSON.stringify({ ...manifest, estimatedRuntime: { measured: false, note: 'minimum injection pacing only; setup/collector/native latency excluded', stepSettleMs: SETTLE_MS, fullMatrixMinimumSeconds: cells.reduce((sum, cell) => sum + cell.steps.length * SETTLE_MS, 0) / 1000 }, counts: Object.fromEntries(['vim', 'emacs', 'standard'].map((style) => [style, cells.filter((cell) => cell.style === style).length])) }, null, 2)); return; }
  assert.ok(values.out && values.label, '--out and --label required');
  const out = resolve(values.out);
  mkdirSync(out, { recursive: true });
  const manifestFile = join(out, 'run-manifest.json');
  if (existsSync(manifestFile)) {
    assert.ok(values.resume, 'existing output requires --resume');
    resumeCells(JSON.parse(readFileSync(manifestFile, 'utf8')), manifest, []);
  } else writeFileSync(manifestFile, `${JSON.stringify(manifest, null, 2)}\n`);
  values.native = resolve(values.native ?? join(DIRECTORY, 'keys/bin/bistro-keys'));
  const selfTest = nativeRows(values.native, 'self-test').find((row) => row.event === 'self_test');
  const preflight = nativeRows(values.native, 'preflight').find((row) => row.event === 'preflight');
  const sources = nativeRows(values.native, 'sources');
  const blockers = preflightBlockers(preflight, selfTest);
  const automationProbe = spawnSync('osascript', ['-e', 'tell application "System Events" to get UI elements enabled'], { encoding: 'utf8', timeout: TIMEOUT_MS });
  const automation = { status: automationProbe.status, stdout: automationProbe.stdout?.trim(), stderr: automationProbe.stderr?.trim(), error: automationProbe.error?.message };
  if (automation.status !== 0 || automation.stdout !== 'true') blockers.push('System Events UI elements unavailable (passive query is not injection permission)');
  writeFileSync(join(out, 'preflight.json'), JSON.stringify({ selfTest, preflight, sources, automation, blockers }, null, 2));
  if (blockers.length) { console.error(`STOP: ${blockers.join('; ')}`); process.exitCode = 2; return; }
  if (values['preflight-only']) { console.log('preflight complete; no source switch or injection'); return; }
  await liveRun(values, manifest, preflight);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => { console.error(error.message); process.exitCode = 1; });
}
