import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, symlink, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { serveRenderer } from './serve-renderer.mjs';
import { createWindowGuard } from './run-webview.mjs';

test('WKWebView waits for both foreground flags and rejects focus or visibility loss', async () => {
  for (const lostFlag of ['isKey', 'isVisible']) {
    const guard = createWindowGuard();
    let isReady = false;
    guard.ready.then(() => { isReady = true; });
    guard.observe({ event: 'window_state', isKey: false, isVisible: true });
    guard.observe({ event: 'window_state', isKey: true, isVisible: false });
    await Promise.resolve();
    assert.equal(isReady, false);
    guard.observe({ event: 'window_state', isKey: true, isVisible: true });
    await guard.ready;
    assert.equal(isReady, true);
    const failed = assert.rejects(guard.failed, /lost key or visible status/);
    guard.observe({ event: 'window_state', isKey: true, isVisible: true, [lostFlag]: false });
    await failed;
  }
});

test('built renderer server has correct JS/wasm MIME and rejects symlink escapes and writes', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'bistro-renderer-server-'));
  const root = join(directory, 'renderer');
  await mkdir(root);
  await writeFile(join(root, 'index.html'), '<main>Bistro</main>');
  await writeFile(join(root, 'bundle.js'), 'export const ready = true;');
  await writeFile(join(root, 'module.wasm'), 'wasm');
  await writeFile(join(directory, 'private.txt'), 'not served');
  await symlink(join(directory, 'private.txt'), join(root, 'escape.txt'));
  const server = await serveRenderer({ root });
  await (async () => {
    assert.match(await (await fetch(server.url + '/?bench_collector=smoke')).text(), /Bistro/);
    for (const [file, mime] of [['bundle.js', 'text/javascript'], ['module.wasm', 'application/wasm']]) {
      const response = await fetch(server.url + '/' + file);
      assert.equal(response.status, 200);
      assert.equal(response.headers.get('content-type'), mime);
    }
    assert.equal((await fetch(server.url + '/escape.txt')).status, 403);
    assert.equal((await fetch(server.url + '/absent.js')).status, 404);
    assert.equal((await fetch(server.url, { method: 'POST' })).status, 405);
    assert.equal((await fetch(server.url + '/bundle.js', { method: 'HEAD' })).headers.get('content-length'), '26');
  })().finally(async () => { await server.close(); await rm(directory, { recursive: true, force: true }); });
});
