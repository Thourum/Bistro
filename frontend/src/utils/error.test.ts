import { Fault } from '@thourum/fault';
import { expect, it } from 'vitest';
import { formatErrorMessage } from './error';

it('does not look up unknown error messages through Object.prototype', () => {
  expect(formatErrorMessage('constructor')).toBe('constructor');
  expect(formatErrorMessage('toString')).toBe('toString');
});

it('maps known Fault tags and sanitizes unknown Fault messages', () => {
  expect(
    formatErrorMessage(
      new Fault('raw rate-limit error').withTag('RATE_LIMITED'),
    ),
  ).toBe('Too many requests. Try again in a few minutes.');
  expect(formatErrorMessage(new Fault('RATE_LIMITED'))).toBe(
    'Too many requests. Try again in a few minutes.',
  );
  expect(
    formatErrorMessage(
      new Fault('failed token=secret123').withTag('CUSTOM_ERROR'),
    ),
  ).toBe('failed [REDACTED]');
});
