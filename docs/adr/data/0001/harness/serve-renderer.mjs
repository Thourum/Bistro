#!/usr/bin/env node
import { createServer } from 'node:http';
import { readFile, realpath, stat } from 'node:fs/promises';
import { extname, resolve, sep } from 'node:path';
import { pathToFileURL } from 'node:url';
import { parseArgs } from 'node:util';

const MIME = Object.freeze({ '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css',
  '.json': 'application/json', '.wasm': 'application/wasm', '.svg': 'image/svg+xml',
  '.png': 'image/png', '.woff': 'font/woff', '.woff2': 'font/woff2' });

/** Serve an extracted built renderer, never the Vite development server. */
export async function serveRenderer({ root, port = 0 }) {
  const directory = await realpath(root);
  if (!(await stat(resolve(directory, 'index.html'))).isFile()) throw new Error('renderer root requires index.html');
  const server = createServer((request, response) => {
    void respond(request, response).catch((error) => {
      if (!response.headersSent) response.writeHead(error.code === 'ENOENT' ? 404 : 400).end(error.message);
      else response.destroy(error);
    });
  });
  async function respond(request, response) {
    if (!['GET', 'HEAD'].includes(request.method)) { response.writeHead(405).end(); return; }
    const pathname = decodeURIComponent(new URL(request.url, 'http://localhost').pathname);
    const file = await realpath(resolve(directory, `.${pathname === '/' ? '/index.html' : pathname}`));
    if (!file.startsWith(directory + sep)) { response.writeHead(403).end(); return; }
    const content = await readFile(file);
    response.writeHead(200, { 'Content-Type': MIME[extname(file)] ?? 'application/octet-stream',
      'Content-Length': content.length, 'Cache-Control': 'no-store' });
    response.end(request.method === 'HEAD' ? undefined : content);
  }
  await new Promise((accept, reject) => { server.once('error', reject); server.listen(port, '127.0.0.1', accept); });
  return { url: `http://localhost:${server.address().port}`, close: () => new Promise((accept, reject) => {
    server.close((error) => error ? reject(error) : accept());
    server.closeIdleConnections();
  }) };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const { values } = parseArgs({ options: { root: { type: 'string' }, port: { type: 'string' }, help: { type: 'boolean' } } });
  if (values.help) console.log('node serve-renderer.mjs --root <extracted .vite/renderer/main_window> [--port N]');
  else {
    if (!values.root) throw new Error('--root is required');
    const port = Number(values.port ?? 0);
    if (!Number.isInteger(port) || port < 0 || port > 65535) throw new Error('invalid --port');
    const server = await serveRenderer({ root: values.root, port });
    console.log(server.url);
    for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => { void server.close(); });
  }
}
