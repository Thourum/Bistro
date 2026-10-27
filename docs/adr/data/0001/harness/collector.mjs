#!/usr/bin/env node
import { createServer } from 'node:http';
import { open, mkdir } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { parseArgs } from 'node:util';
import { withTimeout } from './common.mjs';

const MAX_BODY_BYTES = 2 * 1024 * 1024;

/** Collect shared-protocol events on loopback; waitFor also handles already-received events. */
export async function createCollector({ file }) {
  await mkdir(dirname(file), { recursive: true });
  const journal = await open(file, 'a');
  const events = new Map();
  const waiters = new Map();
  let failure;
  let rejectFailure;
  const failed = new Promise((_, reject) => { rejectFailure = reject; });
  failed.catch(() => {});
  let pendingWrite = Promise.resolve();
  const fail = (error) => {
    failure ??= error;
    rejectFailure(error);
    for (const listeners of waiters.values()) for (const listener of listeners) listener.reject(error);
    waiters.clear();
  };
  const server = createServer((request, response) => {
    response.setHeader('Access-Control-Allow-Origin', '*');
    response.setHeader('Access-Control-Allow-Methods', 'POST, OPTIONS');
    response.setHeader('Access-Control-Allow-Headers', 'Content-Type');
    if (request.method === 'OPTIONS') { response.writeHead(204).end(); return; }
    if (request.method !== 'POST' || request.url !== '/event') { response.writeHead(404).end(); return; }
    void receive(request, response).catch((error) => {
      fail(error);
      if (!response.headersSent) response.writeHead(400).end(error.message);
      else response.destroy(error);
    });
  });

  async function receive(request, response) {
    const chunks = [];
    let size = 0;
    for await (const chunk of request) {
      size += chunk.length;
      if (size > MAX_BODY_BYTES) throw new Error('collector event exceeds 2 MiB');
      chunks.push(chunk);
    }
    const payload = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    if (!payload || typeof payload.event !== 'string' || !payload.event) throw new Error('collector body requires event name');
    const receipt = { ...payload, receivedAt: new Date().toISOString(), receivedPerfMs: performance.now() };
    pendingWrite = pendingWrite.then(() => journal.write(`${JSON.stringify(receipt)}\n`));
    await pendingWrite;
    events.set(payload.event, receipt);
    if (payload.event === 'error') fail(new Error(`renderer harness: ${payload.message}`));
    for (const listener of waiters.get(payload.event) ?? []) listener.resolve(receipt);
    waiters.delete(payload.event);
    response.writeHead(204).end();
  }

  await new Promise((resolveListen, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolveListen);
  });
  server.on('error', fail);
  return {
    url: `http://127.0.0.1:${server.address().port}`,
    failed,
    async waitFor(name, { timeoutMs = 120_000 } = {}) {
      if (failure) throw failure;
      if (events.has(name)) return events.get(name);
      let listener;
      const promise = new Promise((resolveEvent, reject) => {
        listener = { resolve: resolveEvent, reject };
        waiters.set(name, [...(waiters.get(name) ?? []), listener]);
      });
      try { return await withTimeout(promise, timeoutMs, `collector event ${name}`); }
      finally { waiters.set(name, (waiters.get(name) ?? []).filter((entry) => entry !== listener)); }
    },
    assertHealthy() { if (failure) throw failure; },
    async close() {
      fail(new Error('collector closed'));
      await new Promise((resolveClose, reject) => server.close((error) => error ? reject(error) : resolveClose()));
      try { await pendingWrite; } finally { await journal.close(); }
    },
  };
}

async function main() {
  const { values } = parseArgs({ options: { out: { type: 'string' }, help: { type: 'boolean' } } });
  if (values.help) { console.log('Usage: node collector.mjs --out events.jsonl'); return; }
  if (!values.out) throw new Error('--out is required');
  const collector = await createCollector({ file: resolve(values.out) });
  console.log(collector.url);
  collector.failed.catch((error) => {
    if (error.message === 'collector closed') return;
    console.error(error.message);
    process.exitCode = 1;
    void collector.close().catch((closeError) => console.error(closeError.message));
  });
  for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => {
    void collector.close().catch((error) => { console.error(error.message); process.exitCode = 1; });
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) await main();
