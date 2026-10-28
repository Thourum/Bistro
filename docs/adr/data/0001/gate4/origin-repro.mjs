#!/usr/bin/env node
import { mkdir, readFile, writeFile, realpath, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { parseArgs } from 'node:util';
import { startSeedEngine } from '../harness/browser-common.mjs';
import { launch, stopEngine, stopProcessGroup } from '../harness/common.mjs';

const { values } = parseArgs({ options: Object.fromEntries(['host', 'renderer', 'seed', 'out'].map((key) => [key, { type: 'string' }])) });
for (const key of ['host', 'renderer', 'seed', 'out']) if (!values[key]) throw new Error(`--${key} is required`);
const out = await realpath(await mkdir(values.out, { recursive: true }).then(() => values.out));
await writeFile(join(out, '.harness-owner.json'), JSON.stringify({ root: out }), { flag: 'wx' });
const seed = await realpath(values.seed);
const directory = join(out, 'run-1');
await mkdir(directory);
const { ownedEngine } = await startSeedEngine({ seed, out, engine: join(seed, 'bistro-sync'), directory });
const host = launch(await realpath(values.host), ['asset', join(seed, 'settings.json')], {
  detached: true, log: join(out, 'host.log'), env: { ...process.env, BISTRO_ADR_RENDERER: await realpath(values.renderer) },
});
const REJECTION_TIMEOUT_MS = 15_000;
try {
  const deadline = Date.now() + REJECTION_TIMEOUT_MS;
  while (Date.now() < deadline) {
    const engineLog = await readFile(join(directory, 'engine.log'), 'utf8');
    if (/origin not allowed|origin.*reject|request origin not allowed/i.test(engineLog)) {
      await writeFile(join(out, 'finding.json'), JSON.stringify({ rejected: true,
        evidence: engineLog.split('\n').filter((line) => /ws upgrade failed/.test(line)),
        migrationCost: 'engine isLocalOrigin must explicitly allow the Wails custom asset origin; production engine unchanged' }, null, 2) + '\n');
      break;
    }
    await sleep(100);
  }
  if (!/origin not allowed|origin.*reject|request origin not allowed/i.test(await readFile(join(directory, 'engine.log'), 'utf8'))) throw new Error('no origin rejection captured');
} finally {
  await stopProcessGroup(host).finally(() => stopEngine(ownedEngine));
  await rm(join(directory, 'user-data'), { recursive: true, force: true });
}
