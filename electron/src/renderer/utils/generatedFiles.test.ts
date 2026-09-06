import { describe, expect, it } from 'vitest';
import { isGeneratedFile } from './generatedFiles';

describe('isGeneratedFile', () => {
  it.each([
    'pnpm-lock.yaml',
    'package-lock.json',
    'yarn.lock',
    'Cargo.lock',
    'go.sum',
    'electron/pnpm-lock.yaml',
    'api/v1/service.pb.go',
    'src/queries_generated.ts',
    'dist/bundle.js',
    'electron/dist/main.js',
    'assets/app.min.js',
    'assets/app.min.css',
  ])('treats %s as generated', (path) => {
    expect(isGeneratedFile(path)).toBe(true);
  });

  it.each([
    'src/renderer/utils/lanes.ts',
    'go/internal/sync/writer.go',
    'README.md',
    'docs/DESIGN.md',
    'src/distance.ts',
    'src/lockfile-reader.ts',
    'internal/sumdb/client.go',
  ])('treats %s as hand-written', (path) => {
    expect(isGeneratedFile(path)).toBe(false);
  });
});
