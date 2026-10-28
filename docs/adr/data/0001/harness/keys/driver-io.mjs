import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { appendFileSync } from 'node:fs';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { setTimeout as delay } from 'node:timers/promises';

const TIMEOUT_MS = 15_000;
const MAX_EVENT_BYTES = 1024 * 1024;
const POLL_MS = 25;

export function timeout(promise, label) {
  let timer;
  const deadline = new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(`${label} timeout`)), TIMEOUT_MS); });
  return Promise.race([promise, deadline]).finally(() => clearTimeout(timer));
}

/** Keep the target-guarded native worker alive so it restores sources on EOF/signals. */
export function openNative(binary, mode, pid, log) {
  const child = spawn(binary, [mode, '--pid', String(pid)], { stdio: ['pipe', 'pipe', 'pipe'] });
  const lines = createInterface({ input: child.stdout });
  const rows = [];
  const exited = new Promise((resolveExit, reject) => {
    child.once('error', reject);
    child.once('exit', (code, signal) => resolveExit({ code, signal }));
  });
  const pending = new Map();
  let sequence = 0;
  const ready = new Promise((resolveReady, reject) => {
    child.once('error', reject);
    child.once('exit', (code) => reject(new Error(`${mode} exited (${code})`)));
    lines.on('line', (line) => {
      appendFileSync(log, `${line}\n`);
      const row = JSON.parse(line);
      rows.push(row);
      if (row.event === `${mode}_ready`) resolveReady(row);
      if (row.event === 'reply') {
        const waiter = pending.get(row.id);
        pending.delete(row.id);
        if (row.ok) waiter?.resolve(row.result);
        else waiter?.reject(new Error(`native ${mode}: ${row.error}`));
      }
    });
  });
  child.stderr.on('data', (chunk) => appendFileSync(log, chunk));
  child.on('exit', () => {
    for (const waiter of pending.values()) waiter.reject(new Error(`${mode} stopped`));
    pending.clear();
  });
  const command = (payload) => new Promise((resolveReply, reject) => {
    const id = String(++sequence);
    pending.set(id, { resolve: resolveReply, reject });
    child.stdin.write(`${JSON.stringify({ id, ...payload })}\n`);
  });
  return { child, rows, exited, ready: timeout(ready, `${mode} ready`), command: (payload) => timeout(command(payload), 'native reply') };
}

/** Loopback command mailbox and raw renderer event journal. */
export async function createKeysCollector(file) {
  const rows = [];
  const commands = [];
  const server = createServer((request, response) => {
    response.setHeader('Access-Control-Allow-Origin', '*');
    response.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
    response.setHeader('Access-Control-Allow-Headers', 'Content-Type');
    if (request.method === 'OPTIONS') { response.writeHead(204).end(); return; }
    if (request.method === 'GET' && request.url === '/keys-command') {
      const command = commands.shift();
      if (!command) { response.writeHead(204).end(); return; }
      response.setHeader('Content-Type', 'application/json');
      response.end(JSON.stringify(command));
      return;
    }
    if (request.method !== 'POST' || request.url !== '/event') { response.writeHead(404).end(); return; }
    const chunks = [];
    let size = 0;
    request.on('data', (chunk) => {
      size += chunk.length;
      if (size > MAX_EVENT_BYTES) request.destroy();
      else chunks.push(chunk);
    });
    request.on('end', () => {
      Promise.resolve().then(() => {
        const row = JSON.parse(Buffer.concat(chunks).toString());
        assert.ok(row && typeof row.event === 'string', 'event required');
        const receipt = { ...row, receivedAt: new Date().toISOString() };
        appendFileSync(file, `${JSON.stringify(receipt)}\n`);
        rows.push(receipt);
        response.writeHead(204).end();
      }).catch(() => response.writeHead(400).end());
    });
  });
  await new Promise((resolveListen, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolveListen); });
  async function waitFor(event, cellId) {
    const start = Date.now();
    while (Date.now() - start < TIMEOUT_MS) {
      const error = rows.find((row) => row.event === 'error');
      assert.ok(!error, error?.message ?? 'renderer failed');
      const row = rows.findLast((entry) => entry.event === event && (!cellId || entry.cellId === cellId));
      if (row) return row;
      await delay(POLL_MS);
    }
    return Promise.reject(new Error(`renderer ${event} timeout`));
  }
  return { url: `http://127.0.0.1:${server.address().port}`, rows, waitFor,
    command: (cell, phase) => commands.push({ cellId: cell.id, actionId: cell.actionId, style: cell.style, focus: cell.focus, phase }),
    close: () => new Promise((resolveClose) => server.close(resolveClose)) };
}
