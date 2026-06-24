// @vitest-environment node
import { describe, it, expect } from 'vitest';
import { detectLanguage, highlightCode, REGISTERED_LANGUAGES } from '../../src/renderer/utils/highlighter';

describe('detectLanguage', () => {
  it('detects TypeScript from .ts extension', () => {
    expect(detectLanguage('src/auth.ts')).toBe('typescript');
  });

  it('detects TypeScript from .tsx extension', () => {
    expect(detectLanguage('src/App.tsx')).toBe('typescript');
  });

  it('detects JavaScript from .js extension', () => {
    expect(detectLanguage('index.js')).toBe('javascript');
  });

  it('detects Go from .go extension', () => {
    expect(detectLanguage('main.go')).toBe('go');
  });

  it('detects Python from .py extension', () => {
    expect(detectLanguage('script.py')).toBe('python');
  });

  it('detects JSON from .json extension', () => {
    expect(detectLanguage('package.json')).toBe('json');
  });

  it('detects YAML from .yml extension', () => {
    expect(detectLanguage('.github/workflows/ci.yml')).toBe('yaml');
  });

  it('detects Rust from .rs extension', () => {
    expect(detectLanguage('src/main.rs')).toBe('rust');
  });

  it('returns null for unknown extensions', () => {
    expect(detectLanguage('file.xyz')).toBeNull();
  });

  it('returns null for files without extension', () => {
    expect(detectLanguage('Makefile')).toBeNull();
  });
});

describe('highlightCode', () => {
  it('returns highlighted HTML for known language', () => {
    const result = highlightCode('const x = 1;', 'typescript');
    expect(result).toContain('hljs-');
  });

  it('returns escaped HTML for null language', () => {
    const result = highlightCode('<div>test</div>', null);
    expect(result).toContain('&lt;div&gt;');
    expect(result).not.toContain('<div>');
  });

  it('handles empty code', () => {
    const result = highlightCode('', 'typescript');
    expect(result).toBe('');
  });

  it('handles empty code with null language', () => {
    const result = highlightCode('', null);
    expect(result).toBe('');
  });

  it('escapes HTML entities for null language', () => {
    const result = highlightCode('a & b', null);
    expect(result).toContain('&amp;');
  });

  it('falls back to escaped HTML for unknown language', () => {
    const result = highlightCode('<unknown>', 'nonexistent-lang');
    expect(result).toContain('&lt;unknown&gt;');
  });
});

describe('diff grammar', () => {
  it('emits addition/deletion token classes', () => {
    const html = highlightCode('+added\n-removed\n context', 'diff');
    expect(html).toContain('hljs-addition');
    expect(html).toContain('hljs-deletion');
  });
  it('exposes diff in the registered-language allowlist', () => {
    expect(REGISTERED_LANGUAGES.has('diff')).toBe(true);
  });
});
