#!/usr/bin/env node
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';
import { enumerateCells, fingerprint } from './run-keys.mjs';

const CORE_ACTIONS = new Set(['detail.approve', 'detail.requestChanges', 'textarea.submit',
  'diff.deleteDraft', 'detail.nextFile', 'detail.prevFile', 'diff.nextFile', 'diff.prevFile',
  'diff.nextComment', 'diff.prevComment']);
const CLASS_RULES = [
  { name: 'plain outside', prediction: 'PASS', threshold: 0, applies: (cell) => cell.focus === 'outside' && cell.steps.length === 1 && !cell.steps[0].includes('+') },
  { name: 'Shift-only outside', prediction: 'PASS', threshold: 0, applies: (cell) => cell.focus === 'outside' && cell.steps.length === 1 && /^Shift\+[^+]+$/.test(cell.steps[0]) },
  { name: 'Cmd', prediction: 'PASS', threshold: 0, applies: (cell) => cell.steps.some((step) => /(?:Mod|Meta)\+/.test(step)) },
  { name: 'Ctrl', prediction: 'FAIL', threshold: 0, applies: (cell) => cell.steps.some((step) => /Control\+/.test(step)) },
  { name: 'Option US', prediction: 'FAIL', threshold: 0, applies: (cell) => cell.steps.some((step) => /Alt\+/.test(step)) },
  { name: 'sequences', prediction: 'PASS', threshold: 0, applies: (cell) => cell.steps.length > 1 },
  { name: 'inside textarea', prediction: 'PASS (inactive IME)', threshold: 0, applies: (cell) => cell.focus === 'inside' },
];

function indexGroups(groups, cells) {
  const expected = new Map(cells.map((cell) => [cell.id, cell]));
  const indexed = new Map();
  for (const group of groups) {
    assert.ok(group?.cell && expected.has(group.cell.id), 'unknown journal cell');
    assert.equal(fingerprint(group.cell), fingerprint(expected.get(group.cell.id)), 'cell identity mismatch');
    assert.ok(['complete', 'incomplete'].includes(group.status), 'invalid group status');
    assert.ok(indexed.get(group.cell.id)?.status !== 'complete', 'duplicate completed group');
    indexed.set(group.cell.id, group);
  }
  return indexed;
}

function hasEvidence(group) {
  return group?.status === 'complete' && group.prepared?.supported === true &&
    (group.cell.focus === 'inside' ? group.prepared.targetKind === 'textarea' : group.prepared.intendedBindingActive === true) &&
    Array.isArray(group.nativeTap) && group.nativeTap.some((row) => row.event === 'cg_event') &&
    Array.isArray(group.dom) && group.dom.some((row) => row.event === 'keys_keydown') &&
    (group.cell.actionId !== 'textarea.submit' || Boolean(group.prepared.text?.trim())) &&
    Array.isArray(group.actionIds) && group.actionIds.every((id) => typeof id === 'string') &&
    typeof group.text === 'string';
}

/** Compare only exact cells; equal missing observations can never establish parity. */
export function compareKeys({ baseline, candidate, cells = enumerateCells() }) {
  const left = indexGroups(baseline, cells);
  const right = indexGroups(candidate, cells);
  const outcomes = cells.map((cell) => {
    const before = left.get(cell.id);
    const after = right.get(cell.id);
    if (!hasEvidence(before) || !hasEvidence(after)) return { cell, status: 'incomplete', reason: 'missing/unsupported/native-unverified observation' };
    const equal = JSON.stringify(before.actionIds) === JSON.stringify(after.actionIds) && before.text === after.text;
    const status = equal ? 'pass' : 'fail';
    return { cell, status, reason: equal ? undefined : 'actionIds or typing text differ',
      baseline: { actionIds: before.actionIds, text: before.text }, candidate: { actionIds: after.actionIds, text: after.text } };
  });
  const classes = CLASS_RULES.map((rule) => {
    const members = outcomes.filter(({ cell }) => rule.applies(cell));
    const failures = members.filter(({ status }) => status === 'fail');
    const incomplete = members.filter(({ status }) => status === 'incomplete').length;
    const observed = members.length - incomplete;
    const failingShare = observed ? failures.length / observed : null;
    const exceeds = observed > 0 && failingShare > rule.threshold;
    const status = exceeds ? 'fail' : incomplete || !members.length ? 'incomplete' : 'pass';
    return { name: rule.name, prediction: rule.prediction, threshold: rule.threshold, expected: members.length,
      observed, incomplete, failing: failures.length, failingShare, status,
      predictionComparison: status === 'incomplete' ? 'unmeasured' : status.toUpperCase() === rule.prediction.split(' ')[0] ? 'confirmed' : 'falsified' };
  });
  const coreBlockers = outcomes.filter(({ cell, status }) => status === 'fail' && CORE_ACTIONS.has(cell.actionId));
  const failing = outcomes.filter(({ status }) => status === 'fail').length;
  const incomplete = outcomes.filter(({ status }) => status === 'incomplete').length;
  const status = coreBlockers.length || classes.some((entry) => entry.status === 'fail') ? 'fail' : incomplete || classes.some((entry) => entry.status === 'incomplete') ? 'incomplete' : 'pass';
  return { schemaVersion: 1, status, expected: cells.length, failing, incomplete, classes, coreBlockers, outcomes };
}

function readRun(directory) {
  const manifest = JSON.parse(readFileSync(join(directory, 'run-manifest.json'), 'utf8'));
  const identity = { schemaVersion: manifest.schemaVersion, target: manifest.target, cells: manifest.cells };
  assert.equal(fingerprint(identity), manifest.fingerprint, 'manifest fingerprint invalid');
  assert.ok(Array.isArray(manifest.cells), 'manifest cells missing');
  const groups = readFileSync(join(directory, 'cells.jsonl'), 'utf8').split('\n').filter(Boolean).map(JSON.parse);
  return { manifest, groups };
}

function main() {
  const { values } = parseArgs({ options: Object.fromEntries(['baseline', 'candidate', 'out'].map((name) => [name, { type: 'string' }])) });
  assert.ok(values.baseline && values.candidate, '--baseline DIR --candidate DIR required');
  const baseline = readRun(resolve(values.baseline));
  const candidate = readRun(resolve(values.candidate));
  const cells = enumerateCells();
  // Full current-source universe catches truncated smoke, stale inventory, and unmatched cells.
  const summary = compareKeys({ baseline: baseline.groups, candidate: candidate.groups, cells });
  const sourceIds = new Set(cells.map((cell) => cell.id));
  for (const run of [baseline, candidate]) assert.ok(run.manifest.cells.every((cell) => sourceIds.has(cell.id)), 'manifest inventory differs from source');
  const output = { ...summary, smokeOnly: baseline.manifest.smokeOnly || candidate.manifest.smokeOnly,
    baselineFingerprint: baseline.manifest.fingerprint, candidateFingerprint: candidate.manifest.fingerprint,
    gate3Prediction: '../gate3/README.md (source prediction, not measured evidence)' };
  if (values.out) writeFileSync(resolve(values.out), `${JSON.stringify(output, null, 2)}\n`);
  console.log(JSON.stringify({ status: output.status, expected: output.expected, failing: output.failing,
    incomplete: output.incomplete, coreBlockers: output.coreBlockers.length, classes: output.classes }, null, 2));
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  Promise.resolve().then(main).catch((error) => { console.error(error.message); process.exitCode = 1; });
}
