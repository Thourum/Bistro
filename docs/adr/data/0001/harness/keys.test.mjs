import test from 'node:test';
import assert from 'node:assert/strict';
import { enumerateCells, fingerprint, selectCells, preflightBlockers, resumeCells, runNativeLifecycle } from './run-keys.mjs';
import { openNative } from './keys/driver-io.mjs';
import { mkdtempSync, writeFileSync, existsSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { compareKeys } from './keys-stats.mjs';
import { parseInventory } from '../gate3/bindings-by-class.mjs';

const fixture = { tables: {
  vim: { 'detail.approve': ['g', 'a'], 'global.quit.win': 'Alt+F4' },
  emacs: { 'detail.nextFile': 'Control+Tab', 'diff.nextComment': 'Alt+N' },
  standard: { 'detail.approve': ['r', 'a'] },
} };
const cells = enumerateCells(fixture);
const observed = (cell, changes = {}) => ({ cell, status: 'complete',
  prepared: { supported: true, intendedBindingActive: true, text: 'seeded composer', targetKind: cell.focus === 'inside' ? 'textarea' : 'outside' },
  nativeTap: [{ event: 'cg_event' }], dom: [{ event: 'keys_keydown' }],
  actionIds: [], text: 'observed typing', nativeMarkedText: { verified: true }, ...changes });

function compare(cell, before = {}, after = {}) {
  return compareKeys({ cells: [cell], baseline: [observed(cell, before)], candidate: [observed(cell, after)] });
}

test('expands every dimension, excludes Windows quit and adds real textarea submit in every style', () => {
  assert.equal(cells.length, (1 + 2 + 1 + 6) * 4 * 2 * 2 * 2);
  assert.equal(new Set(cells.map((cell) => cell.id)).size, cells.length);
  assert.ok(cells.every((cell) => cell.actionId !== 'global.quit.win'));
  assert.equal(cells.filter((cell) => cell.actionId === 'textarea.submit').length, 6 * 32);
  assert.ok(cells.some((cell) => cell.actionId === 'textarea.submit' && cell.steps[0] === 'Control+Enter'));
  assert.deepEqual(enumerateCells(fixture), cells);
  const actual = enumerateCells(parseInventory());
  assert.equal(actual.length, (121 + 123 + 115 + 3) * 32);
  assert.deepEqual([...new Set(actual.map((cell) => cell.layout))], ['US', 'German', 'French-AZERTY', 'Dvorak']);
});

test('live execution cannot accidentally select more than ten cells without explicit full opt-in', () => {
  assert.throws(() => selectCells(cells, {}), /--limit/);
  assert.throws(() => selectCells(cells, { limit: '11' }), /1\.\.10/);
  assert.throws(() => selectCells(cells, { full: true, limit: '1' }), /not both/);
  assert.equal(selectCells(cells, { 'dry-run': true }).length, 10);
  assert.equal(selectCells(cells, { limit: '2' }).length, 2);
  assert.equal(selectCells(cells, { full: true }).length, cells.length);
});

test('preflight stops for missing posting/AX permission or even one disabled required source', () => {
  const names = ['US', 'German', 'French-AZERTY', 'Dvorak', 'Japanese-active', 'Japanese-off'];
  const required = Object.fromEntries(names.map((name) => [name, { enabledIDs: ['source'] }]));
  const ready = { postEventAccess: true, accessibilityTrusted: true, passiveTapCreated: true, inventory: { required } };
  assert.deepEqual(preflightBlockers(ready, { ok: true }), []);
  assert.match(preflightBlockers({ ...ready, postEventAccess: false }, { ok: true }).join(), /postEventAccess/);
  assert.match(preflightBlockers({ ...ready, inventory: { required: { ...required, Dvorak: { enabledIDs: [] } } } }, { ok: true }).join(), /Dvorak/);
});

test('resume rejects changed fingerprints and duplicate successful cells, retries incomplete groups', () => {
  const manifest = { fingerprint: fingerprint(cells), cells };
  assert.throws(() => resumeCells({ ...manifest, fingerprint: 'different' }, manifest, []), /fingerprint/);
  const complete = observed(cells[0]);
  assert.throws(() => resumeCells(manifest, manifest, [complete, complete]), /duplicate/);
  assert.equal(resumeCells(manifest, manifest, [{ cell: cells[0], status: 'incomplete' }]).size, 0);
  assert.ok(resumeCells(manifest, manifest, [complete]).has(cells[0].id));
});

test('exact actions AND resulting typing text define parity, not native or DOM event equality', () => {
  const cell = cells.find((entry) => entry.actionId === 'detail.nextFile' && entry.ime === 'off');
  const baseline = { actionIds: ['detail.nextFile'] };
  assert.equal(compare(cell, baseline, baseline).outcomes[0].status, 'pass');
  const failure = compare(cell, baseline, { actionIds: ['detail.prevFile'] });
  assert.equal(failure.status, 'fail');
  assert.equal(failure.coreBlockers.length, 1);
  assert.equal(compare(cell, {}, { text: 'wrong characters' }).outcomes[0].status, 'fail');
  assert.throws(() => compareKeys({ cells: [cell], baseline: [observed({ ...cell, steps: ['x'] })], candidate: [] }), /identity/);
});

test('missing, unsupported, synthetic textarea and unproved native IME cells remain incomplete', () => {
  const cell = cells.find((entry) => entry.focus === 'inside' && entry.ime === 'off');
  assert.equal(compareKeys({ cells: [cell], baseline: [], candidate: [] }).status, 'incomplete');
  assert.equal(compare(cell, {}, { prepared: { supported: false } }).status, 'incomplete');
  assert.equal(compare(cell, {}, { prepared: { supported: true, targetKind: 'fake-textarea' } }).status, 'incomplete');
  assert.equal(compare(cell, {}, { nativeTap: [] }).status, 'incomplete');
  const active = cells.find((entry) => entry.ime === 'active');
  assert.equal(compare(active, {}, { nativeMarkedText: undefined }).status, 'incomplete');
});

test('identical unsafe IME dispatch fails either-shell rule rather than appearing equivalent', () => {
  const cell = cells.find((entry) => entry.ime === 'active');
  const summary = compare(cell, { actionIds: ['detail.approve'] }, { actionIds: ['detail.approve'] });
  assert.equal(summary.outcomes[0].status, 'fail');
  assert.equal(summary.classes.find((entry) => entry.name === 'IME active').status, 'fail');
  assert.equal(compareKeys({ cells: [cell], baseline: [observed(cell, { actionIds: ['detail.approve'] })], candidate: [] }).status, 'fail');
});

test('non-US Option tolerance needs verified Settings workaround and never exempts a core failure', () => {
  const optionCells = Array.from({ length: 20 }, (_, index) => {
    const cell = cells.find((entry) => entry.actionId === 'diff.nextComment' && entry.layout === 'German' && entry.ime === 'off');
    const identity = { ...cell, actionId: `noncore.${index}` };
    return { ...identity, id: fingerprint(identity) };
  });
  const baseline = optionCells.map((cell) => observed(cell));
  const candidate = optionCells.map((cell, index) => observed(cell, index === 0 ? { text: 'different' } : {}));
  const result = compareKeys({ cells: optionCells, baseline, candidate });
  assert.equal(result.classes.find((entry) => entry.name === 'Option non-US').failingShare, 0.05);
  assert.equal(result.classes.find((entry) => entry.name === 'Option non-US').status, 'fail');
  const rebound = candidate.map((group, index) => index === 0 ? { ...group, workaround: { rebindVerified: true, description: 'Settings → Keyboard alternative verified in same context' } } : group);
  assert.equal(compareKeys({ cells: optionCells, baseline, candidate: rebound }).classes.find((entry) => entry.name === 'Option non-US').status, 'pass');
});

test('failed native worker command requests restoration and nonzero restoration exit never publishes completion', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'bistro-keys-lifecycle-'));
  const binary = join(directory, 'fake-worker');
  const log = join(directory, 'worker.jsonl');
  const marker = join(directory, 'completed.json');
  const fakeWorker = `#!${process.execPath}
import { createInterface } from 'node:readline';
const emit = (row) => process.stdout.write(JSON.stringify(row) + '\\n');
process.on('SIGTERM', () => {
  emit({event:'restore_requested'});
  emit({event:'restored',ok:false});
  process.exitCode = 1;
  process.stdin.destroy();
});
createInterface({input:process.stdin}).on('line', (line) => {
  const command = JSON.parse(line);
  emit({event:'reply',id:command.id,ok:false,error:'fixture worker failed mid-run'});
});
emit({event:'worker_ready'});
`;
  writeFileSync(binary, fakeWorker, { mode: 0o700 });
  const worker = openNative(binary, 'worker', process.pid, log);
  return Promise.resolve().then(async () => {
    await worker.ready;
    await assert.rejects(runNativeLifecycle({ worker,
      run: () => worker.command({ command: 'inject', source: 'fixture', key: 'x' }),
      complete: () => writeFileSync(marker, 'unexpected completion'),
    }), /native restoration worker failed/);
    assert.equal(existsSync(marker), false);
    assert.equal(worker.child.exitCode, 1);
    const events = readFileSync(log, 'utf8').trim().split('\n').map(JSON.parse);
    assert.ok(events.some((row) => row.event === 'restore_requested'));
    assert.ok(events.some((row) => row.event === 'restored' && row.ok === false));
  }).finally(() => {
    if (worker.child.exitCode === null) worker.child.kill('SIGTERM');
    rmSync(directory, { recursive: true, force: true });
  });
});
