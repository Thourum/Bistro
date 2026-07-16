import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { builtinModules } from 'node:module';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { build } from 'vite';
import { afterEach, describe, expect, it } from 'vitest';

const TEST_OUTPUT_ROOT = resolve('.vite');

let temporaryDirectories: string[] = [];

afterEach(async () => {
  await Promise.all(
    temporaryDirectories.map((directory) =>
      rm(directory, { force: true, recursive: true }),
    ),
  );
  temporaryDirectories = [];
});

describe('Electron main bundle', () => {
  it('loads the CommonJS go-spawn bundle without initialization errors', async () => {
    await mkdir(TEST_OUTPUT_ROOT, { recursive: true });
    const temporaryDirectory = await mkdtemp(
      join(TEST_OUTPUT_ROOT, 'go-spawn-bundle-'),
    );
    temporaryDirectories = [...temporaryDirectories, temporaryDirectory];

    const buildResult = await build({
      configFile: resolve('vite.main.config.ts'),
      build: {
        write: false,
        lib: {
          entry: resolve('src/main/go-spawn.ts'),
          fileName: 'go-spawn',
          formats: ['cjs'],
        },
        rollupOptions: {
          external: builtinModules.flatMap((moduleName) => [
            moduleName,
            `node:${moduleName}`,
          ]),
        },
      },
    });
    const output = Array.isArray(buildResult) ? buildResult[0] : buildResult;
    const chunk = output?.output.find((item) => item.type === 'chunk');

    expect(chunk?.type).toBe('chunk');
    if (chunk?.type !== 'chunk') {
      return;
    }

    const bundlePath = join(temporaryDirectory, 'go-spawn.cjs');
    await writeFile(bundlePath, chunk.code);

    await expect(import(pathToFileURL(bundlePath).href)).resolves.toBeDefined();
  });
});
