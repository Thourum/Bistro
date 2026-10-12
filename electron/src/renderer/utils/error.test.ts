import { expect, it } from 'vitest';
import { formatErrorMessage } from './error';

it('does not look up unknown error messages through Object.prototype', () => {
  expect(formatErrorMessage('constructor')).toBe('constructor');
  expect(formatErrorMessage('toString')).toBe('toString');
});
