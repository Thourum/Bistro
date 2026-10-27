#!/usr/bin/env node
import { readFile, readdir, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const BOOTSTRAP_RESAMPLES = 1000;
const BOOTSTRAP_SEED = 0xB15720;
const TARGET_REFRESH_MS = 1000 / 120;
const DROPPED_FRAME_MULTIPLIER = 1.5;

/** Linear-interpolated quantile, also used for the median and bootstrap bounds. */
export function percentile(samples, fraction) {
  if (!samples.length) throw new Error('cannot compute percentile of empty samples');
  const sorted = samples.toSorted((left, right) => left - right);
  const position = (sorted.length - 1) * fraction;
  const lower = Math.floor(position);
  return sorted[lower] + (sorted[Math.ceil(position)] - sorted[lower]) * (position - lower);
}

function validateSamples(samples, label, { allowZero = true } = {}) {
  if (!Array.isArray(samples) || !samples.length || samples.some((value) => !Number.isFinite(value) || (allowZero ? value < 0 : value <= 0))) {
    throw new Error(`${label}: expected nonempty finite ${allowZero ? 'nonnegative' : 'positive'} numbers`);
  }
}

function distribution(samples) {
  return { min: Math.min(...samples), median: percentile(samples, 0.5), max: Math.max(...samples) };
}

function tails(samples) {
  return { p50: percentile(samples, 0.5), p95: percentile(samples, 0.95), p99: percentile(samples, 0.99) };
}

/** Reproducible percentile-bootstrap 95% CI for the sample median; not a confidence claim for tails. */
export function bootstrapMedian(samples) {
  let state = BOOTSTRAP_SEED;
  const random = () => {
    state = (Math.imul(state, 1664525) + 1013904223) >>> 0;
    return state / 2 ** 32;
  };
  const medians = Array.from({ length: BOOTSTRAP_RESAMPLES }, () => {
    const resample = Array.from({ length: samples.length }, () => samples[Math.floor(random() * samples.length)]);
    return percentile(resample, 0.5);
  });
  return { low: percentile(medians, 0.025), high: percentile(medians, 0.975), resamples: BOOTSTRAP_RESAMPLES, seed: BOOTSTRAP_SEED };
}

export function summarizeFrames(runs) {
  for (const run of runs) {
    validateSamples(run.deltas, 'frame deltas');
    validateSamples([run.refreshMs], 'refreshMs', { allowZero: false });
  }
  const pooled = runs.flatMap((run) => run.deltas);
  const measuredDrops = runs.reduce((count, run) => count + run.deltas.filter((delta) => delta > run.refreshMs * DROPPED_FRAME_MULTIPLIER).length, 0);
  const targetDrops = pooled.filter((delta) => delta > TARGET_REFRESH_MS * DROPPED_FRAME_MULTIPLIER).length;
  return { nFrames: pooled.length, nRuns: runs.length, pooledMs: tails(pooled),
    perRunP95Ms: distribution(runs.map((run) => percentile(run.deltas, 0.95))),
    measuredRefreshMs: distribution(runs.map((run) => run.refreshMs)),
    droppedFrameShare: {
      measured: { share: measuredDrops / pooled.length, dropped: measuredDrops, threshold: 'each run refreshMs × 1.5' },
      target120Hz: { share: targetDrops / pooled.length, dropped: targetDrops, budgetMs: TARGET_REFRESH_MS,
        thresholdMs: TARGET_REFRESH_MS * DROPPED_FRAME_MULTIPLIER, threshold: '120 Hz budget (8.33 ms) × 1.5' },
    } };
}

async function findFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(entries.filter((entry) => entry.name !== 'user-data').map(async (entry) => {
    const file = join(directory, entry.name);
    return entry.isDirectory() ? findFiles(file) : [file];
  }));
  return nested.flat().toSorted();
}

function memoryTotal(sample, file) {
  if (!Array.isArray(sample.processes) || !sample.processes.length) throw new Error(`${file}: missing memory processes`);
  validateSamples(sample.processes.map((entry) => entry.bytes), file);
  const sum = sample.processes.reduce((total, entry) => total + entry.bytes, 0);
  if (sum !== sample.totalBytes) throw new Error(`${file}: memory total does not match process sum`);
  return sum;
}

async function readMeasurements(outDir) {
  const fixtures = new Map();
  const memory = new Map();
  for (const file of await findFiles(outDir)) {
    if (file.endsWith('/events.jsonl')) {
      const lines = (await readFile(file, 'utf8')).trim().split('\n').filter(Boolean);
      const events = lines.map((line) => JSON.parse(line));
      if (events.some((event) => event.event === 'error')) throw new Error(`${file}: renderer reported an error`);
      const frames = events.filter((event) => event.event === 'frames');
      if (frames.length > 1) throw new Error(`${file}: expected one frames event per run`);
      if (frames.length && !events.some((event) => event.event === 'diff_done')) throw new Error(`${file}: incomplete diff run`);
      for (const frame of frames) {
        if (typeof frame.prId !== 'string' || !frame.prId) throw new Error(`${file}: missing prId`);
        fixtures.set(frame.prId, [...(fixtures.get(frame.prId) ?? []), frame]);
      }
    }
    if (file.endsWith('/memory/sample.json')) {
      const sample = JSON.parse(await readFile(file, 'utf8'));
      const phase = dirnamePhase(file, outDir);
      memory.set(phase, [...(memory.get(phase) ?? []), memoryTotal(sample, file)]);
    }
  }
  return { fixtures, memory };
}

function dirnamePhase(file, outDir) {
  const relative = file.slice(outDir.length + 1).split('/');
  return relative[0] === 'frames' ? `post-diff/${relative[1]}` : relative[0];
}

async function validateCompleteness(outDir, manifest) {
  await readFile(join(outDir, 'completed.json'), 'utf8');
  const { counts, seedManifest } = manifest;
  for (const count of Object.values(counts ?? {})) if (!Number.isSafeInteger(count) || count <= 0) throw new Error('invalid run manifest counts');
  if (!counts?.startup || !counts.memory || !counts.frames || !Array.isArray(seedManifest?.fixtures) || !seedManifest.fixtures.length) throw new Error('invalid run manifest');
  const expectedEvents = [join(outDir, 'startup/events.jsonl')];
  const expectedMemory = [];
  const phases = [{ name: 'idle', count: counts.memory }, ...seedManifest.fixtures.map((fixture) => ({ name: `frames/${fixture.name}`, count: counts.frames, prId: fixture.prId }))];
  for (const phase of phases) {
    for (let run = 1; run <= phase.count; run++) {
      const directory = join(outDir, phase.name, `run-${run}`);
      const eventsFile = join(directory, 'events.jsonl');
      expectedEvents.push(eventsFile);
      const events = (await readFile(eventsFile, 'utf8')).trim().split('\n').filter(Boolean).map(JSON.parse);
      if (events.filter((event) => event.event === 'pr_list_rendered').length !== 1) throw new Error(`${eventsFile}: missing/duplicate list readiness`);
      const frames = events.filter((event) => event.event === 'frames');
      if (phase.prId && (frames.length !== 1 || frames[0].prId !== phase.prId || events.filter((event) => event.event === 'diff_done').length !== 1)) throw new Error(`${eventsFile}: incomplete diff run or wrong fixture`);
      if (!phase.prId && frames.length) throw new Error(`${eventsFile}: unexpected idle frames`);
      if (!phase.prId || run <= counts.memory) expectedMemory.push(join(directory, 'memory/sample.json'));
    }
  }
  const files = await findFiles(outDir);
  for (const [suffix, expected] of [['/events.jsonl', expectedEvents], ['/memory/sample.json', expectedMemory]]) {
    const actual = files.filter((file) => file.endsWith(suffix));
    if (JSON.stringify(actual.toSorted()) !== JSON.stringify(expected.toSorted())) throw new Error(`missing or extra ${suffix} runs`);
  }
}

/** Wall-clock seconds and exit codes from hyperfine 1.x (`times`) or 2.x (`measurements`) JSON. */
function hyperfineRuns(result) {
  if (Array.isArray(result.times)) return { times: result.times, exitCodes: result.exit_codes ?? [] };
  if (!Array.isArray(result.measurements)) throw new Error('startup.json: unrecognised hyperfine schema');
  for (const run of result.measurements) {
    if (run.time_wall_clock?.unit !== 'second') throw new Error('startup.json: wall-clock time is not in seconds');
  }
  return {
    times: result.measurements.map((run) => run.time_wall_clock.value),
    exitCodes: result.measurements.map((run) => run.exit_code),
  };
}

/** Summarize raw driver output without means or silently discarded failed runs. */
export async function summarizeDirectory(outDir) {
  const manifest = JSON.parse(await readFile(join(outDir, 'run-manifest.json'), 'utf8'));
  await validateCompleteness(outDir, manifest);
  const startup = JSON.parse(await readFile(join(outDir, 'startup.json'), 'utf8'));
  if (!Array.isArray(startup.results) || startup.results.length !== 1) throw new Error('startup.json requires exactly one hyperfine result');
  const benchmark = hyperfineRuns(startup.results[0]);
  validateSamples(benchmark.times, 'startup times', { allowZero: false });
  if (benchmark.times.length !== manifest.counts.startup) throw new Error('startup run count differs from manifest');
  if (benchmark.exitCodes.some((code) => code !== 0)) throw new Error('startup contains failed runs');
  const startupEvents = (await readFile(join(outDir, 'startup/events.jsonl'), 'utf8')).trim().split('\n').filter(Boolean).map(JSON.parse);
  if (startupEvents.filter((event) => event.event === 'pr_list_rendered').length !== manifest.counts.startup + 1) throw new Error('startup readiness count differs from manifest (including warmup)');
  const { fixtures, memory } = await readMeasurements(outDir);
  if (!fixtures.size || !memory.size) throw new Error('missing frame or memory measurements');
  const timesMs = benchmark.times.map((seconds) => seconds * 1000);
  return { schemaVersion: 1, generatedAt: new Date().toISOString(), syntheticComplete: manifest.seedManifest.syntheticComplete,
    percentileMethod: 'linear interpolation at (n−1) × q',
    startup: { nRuns: timesMs.length, milliseconds: tails(timesMs), median95CI: bootstrapMedian(timesMs) },
    fixtures: Object.fromEntries([...fixtures].map(([prId, runs]) => [prId, summarizeFrames(runs)])),
    memory: Object.fromEntries([...memory].map(([phase, sums]) => [phase, { nRuns: sums.length, bytes: distribution(sums) }])),
    notes: ['Memory sums per-process footprint; shared allocations can be counted more than once.',
      'Measured-refresh dropped share uses each run’s idle-rAF median; 120 Hz is a separate fixed budget.',
      'Startup includes shell/app launch through PR-list ready and process exit, not just renderer t.',
      'Bootstrap CI is for the startup median, not p95/p99. Small n gives imprecise tails. See conditions.txt.'] };
}

function markdown(summary) {
  const fixed = (value) => value.toFixed(2);
  const percent = (value) => `${(value * 100).toFixed(2)}%`;
  const startup = summary.startup;
  const lines = ['# Gate 1 measurements', '', ...(summary.syntheticComplete ? [] : ['**Incomplete Gate 1 fixture set: synthetic fixtures were not seeded.**', '']), `Startup (${startup.nRuns} runs): p50/p95/p99 = ${Object.values(startup.milliseconds).map(fixed).join(' / ')} ms.`,
    `Median bootstrap 95% CI: ${fixed(startup.median95CI.low)}–${fixed(startup.median95CI.high)} ms (${BOOTSTRAP_RESAMPLES} resamples, seed ${BOOTSTRAP_SEED}).`, '',
    '| Fixture | Runs / frames | Pooled p50 / p95 / p99 (ms) | Run p95 min / median / max (ms) | Dropped, measured refresh ×1.5 | Dropped, 120 Hz ×1.5 |',
    '|---|---|---|---|---|---|'];
  for (const [id, frame] of Object.entries(summary.fixtures)) lines.push(`| ${id} | ${frame.nRuns} / ${frame.nFrames} | ${Object.values(frame.pooledMs).map(fixed).join(' / ')} | ${Object.values(frame.perRunP95Ms).map(fixed).join(' / ')} | ${percent(frame.droppedFrameShare.measured.share)} | ${percent(frame.droppedFrameShare.target120Hz.share)} |`);
  lines.push('', '120 Hz budget = 8.33 ms; dropped threshold = 12.50 ms. Counts are late-frame shares, not an estimate of missed refresh slots.', '',
    '| Memory phase | Runs | Per-run process sum min / median / max (MiB) |', '|---|---|---|');
  for (const [phase, sample] of Object.entries(summary.memory)) lines.push(`| ${phase} | ${sample.nRuns} | ${Object.values(sample.bytes).map((bytes) => fixed(bytes / 1024 ** 2)).join(' / ')} |`);
  lines.push('', ...summary.notes.map((note) => `- ${note}`), '');
  return lines.join('\n');
}

async function main() {
  if (process.argv.includes('--help')) { console.log('Usage: node stats.mjs <outDir>\nWrites summary.json and summary.md from startup.json, events.jsonl and memory/sample.json.'); return; }
  if (process.argv.length !== 3) throw new Error('Usage: node stats.mjs <outDir>');
  const outDir = resolve(process.argv[2]);
  const summary = await summarizeDirectory(outDir);
  await writeFile(join(outDir, 'summary.json'), `${JSON.stringify(summary, null, 2)}\n`);
  await writeFile(join(outDir, 'summary.md'), markdown(summary));
  console.log(`Summaries: ${join(outDir, 'summary.json')} and ${join(outDir, 'summary.md')}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) await main();
