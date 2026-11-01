// @vitest-environment node

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { extname, join } from 'node:path';
import { describe, expect, it } from 'vitest';

const RENDERER_SRC = join(__dirname, '../../src');

function collectFiles(dir: string, exts: string[]): string[] {
  const results: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      results.push(...collectFiles(full, exts));
    } else if (exts.includes(extname(entry)) && !entry.includes('.test.')) {
      results.push(full);
    }
  }
  return results;
}

describe('no raw target="_blank" in renderer src', () => {
  it('renderer TS/TSX files should not contain raw target="_blank" anchors', () => {
    const files = collectFiles(RENDERER_SRC, ['.ts', '.tsx']);
    const violations: string[] = [];

    for (const file of files) {
      // ExternalLink.tsx is the canonical implementation — it wraps the anchor
      if (file.endsWith('ExternalLink.tsx')) continue;
      const content = readFileSync(file, 'utf-8');
      // Match <a ... target="_blank" or target='_blank'
      if (/\btarget=["']_blank["']/.test(content)) {
        violations.push(file.replace(RENDERER_SRC + '/', ''));
      }
    }

    expect(violations).toEqual([]);
  });
});
