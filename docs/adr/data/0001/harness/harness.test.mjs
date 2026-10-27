import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, mkdir, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createCollector } from './collector.mjs';
import { bootstrapMedian, summarizeDirectory, summarizeFrames } from './stats.mjs';
import { execute, launch, resetUserData, stopProcessGroup } from './common.mjs';

async function temporaryDirectory(run) {
  const directory = await mkdtemp(join(tmpdir(), 'bistro-harness-test-'));
  try { await run(directory); }
  finally { await rm(directory, { recursive: true, force: true }); }
}

test('collector journals concurrent protocol events, resolves late waiters, and propagates renderer failure', async () => {
  await temporaryDirectory(async (directory) => {
    const file = join(directory, 'events.jsonl');
    const collector = await createCollector({ file });
    try {
      const waiting = collector.waitFor('frames');
      const preflight = await fetch(`${collector.url}/event`, { method: 'OPTIONS' });
      assert.equal(preflight.status, 204);
      assert.equal(preflight.headers.get('access-control-allow-origin'), '*');
      const payloads = [{ event: 'frames', prId: 'fixture', deltas: [8, 16] }, { event: 'diff_done' }];
      const responses = await Promise.all(payloads.map((payload) => fetch(`${collector.url}/event`, {
        method: 'POST', headers: { 'Content-Type': 'text/plain' }, body: JSON.stringify(payload),
      })));
      assert.ok(responses.every((response) => response.status === 204));
      assert.equal((await waiting).prId, 'fixture');
      assert.equal((await collector.waitFor('diff_done')).event, 'diff_done');
      const receipts = (await readFile(file, 'utf8')).trim().split('\n').map(JSON.parse);
      assert.equal(receipts.length, 2);
      assert.ok(receipts.every((receipt) => Number.isFinite(receipt.receivedPerfMs) && !Number.isNaN(Date.parse(receipt.receivedAt))));
      const pending = collector.waitFor('never');
      const rejected = assert.rejects(pending, /renderer harness: broken viewport/);
      await fetch(`${collector.url}/event`, { method: 'POST', body: JSON.stringify({ event: 'error', message: 'broken viewport' }) });
      await rejected;
      assert.throws(() => collector.assertHealthy(), /broken viewport/);
    } finally { await collector.close(); }
  });
});

test('pooled frame tails and per-run tails retain weights and separate measured refresh from 120 Hz', () => {
  const summary = summarizeFrames([
    { refreshMs: 10, deltas: [10, 20] },
    { refreshMs: 20, deltas: [20, 20, 40] },
  ]);
  assert.equal(summary.nFrames, 5);
  assert.equal(summary.nRuns, 2);
  assert.equal(summary.pooledMs.p50, 20);
  assert.ok(Math.abs(summary.pooledMs.p95 - 36) < 1e-10);
  assert.equal(summary.perRunP95Ms.min, 19.5);
  assert.equal(summary.perRunP95Ms.median, 28.75);
  assert.equal(summary.perRunP95Ms.max, 38);
  assert.equal(summary.droppedFrameShare.measured.share, 2 / 5);
  assert.equal(summary.droppedFrameShare.target120Hz.share, 4 / 5);
  assert.deepEqual(bootstrapMedian([1, 2, 3, 4]), bootstrapMedian([1, 2, 3, 4]));
  assert.deepEqual(bootstrapMedian([5, 5, 5]), { low: 5, high: 5, resamples: 1000, seed: 0xB15720 });
});

test('profile preparation rejects an unowned directory without deleting its contents', async () => {
  await temporaryDirectory(async (directory) => {
    const seed = join(directory, 'seed'), out = join(directory, 'out'), userData = join(directory, 'unrelated/user-data');
    await mkdir(seed);
    await mkdir(out);
    await mkdir(userData, { recursive: true });
    await writeFile(join(seed, 'sync.db'), 'database');
    await writeFile(join(seed, 'settings.json'), '{}');
    await writeFile(join(out, '.harness-owner.json'), JSON.stringify({ root: await realpath(out) }));
    await writeFile(join(userData, 'keep'), 'untouched');
    await assert.rejects(resetUserData({ seed, userData, ownerRoot: out }), /not inside/);
    assert.equal(await readFile(join(userData, 'keep'), 'utf8'), 'untouched');
  });
});

test('a hanging sampler is killed when the measurement is interrupted', async () => {
  const interrupted = new Promise((_, reject) => setTimeout(() => reject(new Error('interrupted sample')), 50));
  await assert.rejects(execute(process.execPath, ['-e', 'setInterval(()=>{},1000)'], { cancelled: interrupted, timeoutMs: 5000 }), /interrupted sample/);
});

test('owned detached process group terminates even after its root has exited', async () => {
  if (process.platform === 'win32') return;
  const parentCode = `const {spawn}=require('node:child_process'); const child=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{stdio:'ignore'}); console.log(child.pid); child.unref();`;
  const owned = launch(process.execPath, ['-e', parentCode], { detached: true });
  owned.child.stdin.end();
  const childPid = Number((await owned.done).trim());
  assert.ok(childPid > 0);
  await stopProcessGroup(owned);
  assert.throws(() => process.kill(childPid, 0), { code: 'ESRCH' });
});

test('synthetic raw files produce startup milliseconds and memory sums, and reject incomplete diff runs', async () => {
  await temporaryDirectory(async (directory) => {
    const run = join(directory, 'frames', 'synthetic', 'run-1');
    await mkdir(join(run, 'memory'), { recursive: true });
    await mkdir(join(directory, 'startup'), { recursive: true });
    await mkdir(join(directory, 'idle/run-1/memory'), { recursive: true });
    await writeFile(join(directory, 'run-manifest.json'), JSON.stringify({ counts: { startup: 3, memory: 1, frames: 1 }, seedManifest: { syntheticComplete: true, fixtures: [{ name: 'synthetic', prId: 'github:bench/large-diff:1' }] } }));
    await writeFile(join(directory, 'completed.json'), '{}');
    const ready = `${JSON.stringify({ event: 'pr_list_rendered', rows: 1 })}\n`;
    await writeFile(join(directory, 'startup/events.jsonl'), ready.repeat(4));
    await writeFile(join(directory, 'idle/run-1/events.jsonl'), ready);
    await writeFile(join(directory, 'idle/run-1/memory/sample.json'), JSON.stringify({ processes: [{ bytes: 5 }], totalBytes: 5 }));
    await writeFile(join(directory, 'startup.json'), JSON.stringify({ results: [{ times: [1, 2, 3], exit_codes: [0, 0, 0] }] }));
    const frame = { event: 'frames', prId: 'github:bench/large-diff:1', refreshMs: 10, deltas: [10, 20] };
    const events = join(run, 'events.jsonl');
    await writeFile(events, `${ready}${JSON.stringify(frame)}\n${JSON.stringify({ event: 'diff_done' })}\n`);
    await writeFile(join(run, 'memory', 'sample.json'), JSON.stringify({ processes: [{ bytes: 10 }, { bytes: 20 }], totalBytes: 30 }));
    const summary = await summarizeDirectory(directory);
    assert.equal(summary.startup.milliseconds.p50, 2000);
    assert.equal(summary.memory['post-diff/synthetic'].bytes.median, 30);
    assert.equal(summary.fixtures[frame.prId].nFrames, 2);
    await writeFile(events, `${ready}${JSON.stringify(frame)}\n`);
    await assert.rejects(summarizeDirectory(directory), /incomplete diff run/);
    await writeFile(events, ready);
    await assert.rejects(summarizeDirectory(directory), /incomplete diff run/);
  });
});
