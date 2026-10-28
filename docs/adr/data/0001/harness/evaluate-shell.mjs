#!/usr/bin/env node
// Applies the pre-registered S1, S3, S4 and S5 formulas (ADR 0001) to committed full-run evidence.
// Usage: node evaluate-shell.mjs <slim gate1 dir> <candidate dir> [<candidate wails .app KiB> <candidate DMG bytes>]
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

const MIB = 2 ** 20;
// S4 uses the largest real fixture by diff size; google-cloud-go:20526 is +65,122/−45,071 against kubernetes' +5,904/−1,680.
const LARGEST_REAL = { fixture: 'github:googleapis/google-cloud-go:20526', memoryPhase: 'generated' };

const [slimDir, candidateDir, appKiB, dmgBytes] = process.argv.slice(2);
const slim = JSON.parse(readFileSync(join(slimDir, 'summary.json'), 'utf8'));
const candidate = JSON.parse(readFileSync(join(candidateDir, 'frame-summary.json'), 'utf8'));
const median = (values) => {
  const sorted = values.toSorted((a, b) => a - b);
  const middle = (sorted.length - 1) / 2;
  return (sorted[Math.floor(middle)] + sorted[Math.ceil(middle)]) / 2;
};

function candidateMemory(phase) {
  const root = phase === 'idle' ? join(candidateDir, 'idle') : join(candidateDir, 'frames', phase);
  // Only the first five runs of each phase sample memory.
  return readdirSync(root).map((run) => join(root, run, 'memory', 'sample.json')).filter(existsSync).map((file) =>
    JSON.parse(readFileSync(file, 'utf8')).processes.reduce((sum, process) => sum + process.bytes, 0) / MIB);
}

const s1 = Object.entries(candidate.fixtures).map(([fixture, frames]) => {
  const base = slim.fixtures[fixture];
  const checks = {
    p95: frames.pooledMs.p95 <= base.pooledMs.p95 + 2,
    p99: frames.pooledMs.p99 <= Math.max(base.pooledMs.p99 * 1.1, base.pooledMs.p99 + 3),
    lateShare: frames.droppedFrameShare.measured.share <= base.droppedFrameShare.measured.share + 0.02,
    perRunP95: frames.perRunP95Ms.median <= base.perRunP95Ms.median + 2,
  };
  return { fixture, pass: Object.values(checks).every(Boolean), checks,
    candidate: { ...frames.pooledMs, lateShare: frames.droppedFrameShare.measured.share, perRunP95Median: frames.perRunP95Ms.median },
    slim: { ...base.pooledMs, lateShare: base.droppedFrameShare.measured.share, perRunP95Median: base.perRunP95Ms.median } };
});

const slimMiB = (phase) => Object.fromEntries(Object.entries(slim.memory[phase].bytes).map(([key, bytes]) => [key, bytes / MIB]));
const slimIdle = slimMiB('idle');
const idle = candidateMemory('idle');
const s3 = { candidateMedian: median(idle), candidateMax: Math.max(...idle), slimMedian: slimIdle.median, slimMin: slimIdle.min,
  pass: median(idle) <= 0.7 * slimIdle.median && Math.max(...idle) < slimIdle.min };

const slimLarge = slimMiB(`post-diff/${LARGEST_REAL.memoryPhase}`);
const large = median(candidateMemory(LARGEST_REAL.memoryPhase));
const s4 = { fixture: LARGEST_REAL.fixture, candidateMedian: large, slimMedian: slimLarge.median, pass: large <= 0.8 * slimLarge.median };

const sizes = JSON.parse(readFileSync(join(slimDir, 'sizes.json'), 'utf8'));
const s5 = appKiB && dmgBytes ? { candidateAppKiB: Number(appKiB), candidateDmgBytes: Number(dmgBytes), slim: sizes,
  pass: Number(appKiB) <= 0.4 * sizes.appAllocatedKiB && Number(dmgBytes) <= 0.4 * sizes.dmgBytes } : { pass: null, reason: 'no candidate size given' };

console.log(JSON.stringify({ s1, s3, s4, s5 }, null, 2));
