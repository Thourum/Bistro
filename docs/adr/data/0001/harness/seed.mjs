#!/usr/bin/env node
import { mkdir, writeFile, access, rm } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { createInterface } from 'node:readline';
import { parseArgs } from 'node:util';
import { randomUUID } from 'node:crypto';
import { execute, launch, stopEngine, withTimeout, repoRoot, positiveInteger, MINUTE_MS } from './common.mjs';

export const FIXTURES = Object.freeze([
  { name: 'vendoring', prId: 'github:kubernetes/kubernetes:143358', headPrefix: '09048f5ccf2c' },
  { name: 'generated', prId: 'github:googleapis/google-cloud-go:20526', headPrefix: 'de1e8de4f65e' },
]);
const SYNTHETIC_IDS = ['github:bench/large-diff:1', 'github:bench/large-diff:2'];
const DEFAULT_SYNC_TIMEOUT_MS = 30 * MINUTE_MS;

async function startEngine({ binary, outDir }) {
  const engine = launch(binary, [
    '-ws-port', '0', '-db-path', join(outDir, 'sync.db'), '-auto-sync=false',
    '-replay-queue=false', '-log-level', 'warn', '-exit-on-stdin-close',
  ], { cwd: join(repoRoot, 'go'), log: join(outDir, 'seed-engine.log'),
    env: { ...process.env, BISTRO_REPO_DIR: join(outDir, 'repos') } });
  const errors = createInterface({ input: engine.child.stderr });
  const dispatchFailure = new Promise((_, reject) => {
    errors.on('line', (line) => {
      if (line.includes('msg="dispatch error"')) reject(new Error(`engine dispatch failed; see ${join(outDir, 'seed-engine.log')}`));
    });
  });
  dispatchFailure.catch(() => {});
  engine.done.finally(() => errors.close()).catch(() => {});
  const lines = createInterface({ input: engine.child.stdout });
  const ready = new Promise((resolveReady, reject) => {
    lines.on('line', (line) => {
      if (!line.startsWith('{')) return;
      try {
        const message = JSON.parse(line);
        if (message.event === 'ready' && Number.isInteger(message.wsPort) && message.wsPort > 0) resolveReady(message.wsPort);
      } catch (error) { reject(new Error(`invalid engine stdout JSON: ${error.message}`)); }
    });
  });
  try {
    const port = await withTimeout(Promise.race([ready, engine.done.then(() => { throw new Error('engine exited before ready'); })]), MINUTE_MS, 'engine ready');
    return { ...engine, port, dispatchFailure };
  } catch (error) {
    await stopEngine(engine);
    throw error;
  } finally { lines.close(); }
}

async function connectEngine(port) {
  const socket = new WebSocket(`ws://127.0.0.1:${port}/ws`);
  await withTimeout(new Promise((resolveOpen, reject) => {
    socket.addEventListener('open', resolveOpen, { once: true });
    socket.addEventListener('error', () => reject(new Error('engine WebSocket connection failed')), { once: true });
  }), MINUTE_MS, 'WebSocket open');
  return socket;
}

async function request(socket, { type, data, responseType, timeoutMs }) {
  const id = randomUUID();
  let onMessage, onClose;
  const response = new Promise((resolveResponse, reject) => {
    onMessage = ({ data: raw }) => {
      try {
        const message = JSON.parse(raw);
        if (message.id !== id) return;
        if (message.type !== responseType) throw new Error(`${type}: expected ${responseType}, got ${message.type}`);
        if (message.data?.error) throw new Error(`${type}: ${message.data.error}`);
        resolveResponse(message.data);
      } catch (error) { reject(error); }
    };
    onClose = () => reject(new Error(`${type}: engine WebSocket closed`));
    socket.addEventListener('message', onMessage);
    socket.addEventListener('close', onClose);
    socket.send(JSON.stringify({ id, type, data, timestamp: new Date().toISOString() }));
  });
  try { return await withTimeout(response, timeoutMs, `${type} ${data.pr_id ?? ''} (see seed-engine.log)`); }
  finally {
    socket.removeEventListener('message', onMessage);
    socket.removeEventListener('close', onClose);
  }
}

async function readDiff(socket, prId) {
  const diff = await request(socket, { type: 'REQUEST_DIFF', data: { pr_id: prId }, responseType: 'DIFF_READY', timeoutMs: MINUTE_MS });
  if (diff.pr_id !== prId || !Array.isArray(diff.files) || !diff.files.length || typeof diff.diff !== 'string' || !diff.diff.length) {
    throw new Error(`${prId}: diff must have nonempty files and unified text`);
  }
  for (const field of ['additions', 'deletions']) {
    if (!Number.isSafeInteger(diff[field]) || diff[field] < 0) throw new Error(`${prId}: invalid ${field}`);
    const sum = diff.files.reduce((total, file) => {
      if (!Number.isSafeInteger(file[field]) || file[field] < 0) throw new Error(`${prId}: invalid file ${field}`);
      return total + file[field];
    }, 0);
    if (sum !== diff[field]) throw new Error(`${prId}: ${field} differs from file row totals`);
  }
  return { files: diff.files.length, additions: diff.additions, deletions: diff.deletions, unifiedBytes: Buffer.byteLength(diff.diff) };
}

async function syncFixtures({ engine, timeoutMs }) {
  const socket = await connectEngine(engine.port);
  try {
    const auth = await request(socket, { type: 'AUTH_CHECK', data: {}, responseType: 'AUTH_CHECK_RESULT', timeoutMs: MINUTE_MS });
    if (!auth.authenticated) throw new Error('Log in to Bistro first: no authenticated engine provider');
    const fixtures = [];
    for (const fixture of FIXTURES) {
      const [, owner, repo, number] = fixture.prId.match(/^github:([^/]+)\/([^:]+):(\d+)$/);
      const headSha = (await execute('gh', ['api', `repos/${owner}/${repo}/pulls/${number}`, '--jq', '.head.sha'])).trim();
      if (!headSha.startsWith(fixture.headPrefix)) throw new Error(`${fixture.prId}: pinned head SHA changed (${headSha})`);
      console.log(`Syncing ${fixture.prId} (up to ${timeoutMs / MINUTE_MS} minutes)`);
      await request(socket, { type: 'REQUEST_PR_SYNC', data: { pr_id: fixture.prId }, responseType: 'PR_SYNCED', timeoutMs });
      const diff = await readDiff(socket, fixture.prId);
      console.log(`${fixture.prId}: ${diff.files} files, +${diff.additions} -${diff.deletions}`);
      fixtures.push({ ...fixture, headSha, diff });
    }
    return fixtures;
  } finally { socket.close(); }
}

async function seedSynthetic({ outDir, engineBinary }) {
  const source = join(repoRoot, 'go/cmd/bistro-benchseed/main.go');
  try { await access(source); }
  catch (error) {
    if (error.code !== 'ENOENT') throw error;
    console.warn('WARNING: bistro-benchseed is not available yet; synthetic fixtures omitted. Rerun seed after Task 4.');
    return [];
  }
  const binary = join(outDir, 'bistro-benchseed');
  await execute('go', ['build', '-o', binary, './cmd/bistro-benchseed'], { cwd: join(repoRoot, 'go'), log: join(outDir, 'benchseed-build.log') });
  await execute(binary, ['-db-path', join(outDir, 'sync.db')], { log: join(outDir, 'benchseed.log') });
  const engine = await startEngine({ binary: engineBinary, outDir });
  try {
    const socket = await connectEngine(engine.port);
    try {
      const fixtures = [];
      for (const prId of SYNTHETIC_IDS) fixtures.push({ name: prId.endsWith(':1') ? 'synthetic-many-files' : 'synthetic-one-file', prId, headSha: null, diff: await readDiff(socket, prId) });
      if (fixtures[0].diff.files !== 1000 || fixtures[1].diff.files !== 1) throw new Error('synthetic diff file counts do not match 1000 / 1');
      return fixtures;
    } finally { socket.close(); }
  } finally { await stopEngine(engine); }
}

async function writeSettings(outDir) {
  // Mirror DEFAULT_SETTINGS; disable periodic sync, first-run UI and telemetry for repeatable measurements.
  const settings = {
    $schema: 1,
    appearance: { theme: 'light', uiFont: '"Geist Variable", "Geist", "Inter", system-ui, sans-serif',
      codeFont: '"JetBrains Mono", "Geist Mono Variable", "Fira Code", "IBM Plex Mono", monospace',
      uiFontSize: 14, codeFontSize: 13, highDensity: false, onboardingComplete: true, tourComplete: true },
    sync: { autoSync: false, syncInterval: 60, notifications: false }, prList: { view: 'flat' }, diff: { style: 'unified' },
    privacy: { telemetryEnabled: false }, repositories: { devFolderPath: null },
    classification: { mode: 'rules', dismissalScope: 'local', provider: 'anthropic', model: '', endpoint: '' },
    navigation: { style: 'vim' }, keybindings: {},
  };
  await writeFile(join(outDir, 'settings.json'), `${JSON.stringify(settings, null, 2)}\n`);
}

async function seedPinned({ binary, outDir, timeoutMs }) {
  const engine = await startEngine({ binary, outDir });
  const interrupt = () => engine.child.stdin.end();
  process.once('SIGINT', interrupt);
  process.once('SIGTERM', interrupt);
  try { return await Promise.race([syncFixtures({ engine, timeoutMs }), engine.dispatchFailure]); }
  finally {
    process.removeListener('SIGINT', interrupt);
    process.removeListener('SIGTERM', interrupt);
    await stopEngine(engine);
  }
}

async function main() {
  const { values } = parseArgs({ options: { out: { type: 'string' }, engine: { type: 'string' }, 'timeout-minutes': { type: 'string' }, 'synthetic-only': { type: 'boolean' }, help: { type: 'boolean' } } });
  if (values.help) {
    console.log('Usage: node seed.mjs --out <seedDir> [--engine <bistro-sync>] [--timeout-minutes 30] [--synthetic-only]\nBuilds the engine if --engine is absent. Read-only GitHub sync; log in to Bistro and gh first.\n--synthetic-only skips the pinned GitHub PRs (smoke runs only; the manifest is marked incomplete).');
    return;
  }
  if (!values.out) throw new Error('--out is required');
  const timeoutMs = values['timeout-minutes'] ? positiveInteger(values['timeout-minutes'], '--timeout-minutes') * MINUTE_MS : DEFAULT_SYNC_TIMEOUT_MS;
  if (timeoutMs < 10 * MINUTE_MS) throw new Error('--timeout-minutes must be at least 10 (large clones)');
  const outDir = resolve(values.out);
  await mkdir(join(outDir, 'repos'), { recursive: true });
  await rm(join(outDir, 'seed-manifest.json'), { force: true });
  const binary = values.engine ? resolve(values.engine) : join(outDir, 'bistro-sync');
  if (!values.engine) await execute('go', ['build', '-o', binary, './cmd/bistro-sync'], { cwd: join(repoRoot, 'go'), log: join(outDir, 'engine-build.log') });
  const engineGitRev = (await execute('git', ['rev-parse', 'HEAD'], { cwd: repoRoot })).trim();
  const fixtures = values['synthetic-only'] ? [] : await seedPinned({ binary, outDir, timeoutMs });
  const synthetic = await seedSynthetic({ outDir, engineBinary: binary });
  await writeSettings(outDir);
  const manifest = { schemaVersion: 1, createdAt: new Date().toISOString(), engineGitRev,
    engineBinary: binary, engineRevisionSource: values.engine ? 'checkout revision; supplied binary provenance unverified' : 'built from checkout',
    fixtures: [...fixtures, ...synthetic], pinnedComplete: fixtures.length === FIXTURES.length,
    syntheticComplete: synthetic.length === SYNTHETIC_IDS.length };
  await writeFile(join(outDir, 'seed-manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`);
  console.log(`Seed ready: ${outDir}`);
}

await main();
