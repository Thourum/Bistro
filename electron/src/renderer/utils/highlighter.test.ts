import { expect, it } from 'vitest';
import { detectLanguage } from './highlighter';

it('detects extensions only in the basename, regardless of case', () => {
  expect(detectLanguage('src.v2/Settings.TSX')).toBe('typescript');
  expect(detectLanguage('src.v2/Makefile')).toBeNull();
});
