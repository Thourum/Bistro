// @vitest-environment jsdom
import { okAsync } from '@thourum/fault';
import { expect, it, vi } from 'vitest';

vi.mock('../router', () => ({
  router: {
    state: { location: { pathname: '/prs' } },
    navigate: vi.fn(async () => undefined),
    history: { replace: vi.fn() },
  },
}));
vi.mock('../utils/keybindingActions', () => ({
  getBindingAction: vi.fn(),
}));
vi.mock('../utils/benchSafety', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../utils/benchSafety')>()),
  permitsBenchAction: vi.fn(() => false),
}));

import { permitsBenchAction } from '../utils/benchSafety';
import { startBench } from './index';
import { startKeysBench } from './keys';

it('fails closed when a keyboard benchmark collector is missing', () => {
  const previousUrl = location.href;
  window.history.replaceState(
    null,
    '',
    '?bench_keys=1&bench_keys_pr=github:bench/large-diff:1',
  );
  expect(startBench()).toBe(false);
  window.history.replaceState(null, '', previousUrl);
});

it('rejects real PR fixtures and records native textarea outcomes while stopping local submit', () => {
  const previousUrl = location.href;
  const post = vi.fn(() => okAsync(undefined));
  const fail = vi.fn(async () => undefined);
  window.history.replaceState(
    null,
    '',
    '?bench_keys=1&bench_keys_pr=github:real/repo:1',
  );
  expect(startKeysBench('http://127.0.0.1:9000', { post, fail })).toBe(false);
  expect(fail).toHaveBeenCalledOnce();
  window.history.replaceState(
    null,
    '',
    '?bench_keys=1&bench_keys_pr=github:bench/large-diff:1',
  );
  startKeysBench('http://127.0.0.1:9000', { post, fail });
  const textarea = document.createElement('textarea');
  textarea.value = 'native text';
  document.body.append(textarea);
  const nativeHandler = vi.fn();
  textarea.addEventListener('keydown', nativeHandler);
  const keydown = new KeyboardEvent('keydown', {
    key: 'Enter',
    code: 'Enter',
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
  });
  textarea.dispatchEvent(keydown);
  expect(nativeHandler).not.toHaveBeenCalled();
  expect(keydown.defaultPrevented).toBe(true);
  expect(permitsBenchAction).toHaveBeenCalledWith('textarea.submit');
  expect(post).toHaveBeenCalledWith(
    expect.objectContaining({
      event: 'keys_keydown',
      key: 'Enter',
      code: 'Enter',
      ctrlKey: true,
      targetKind: 'textarea',
      text: 'native text',
    }),
  );
  textarea.dispatchEvent(
    new InputEvent('input', {
      inputType: 'insertCompositionText',
      isComposing: true,
      bubbles: true,
    }),
  );
  expect(post).toHaveBeenCalledWith(
    expect.objectContaining({
      event: 'keys_input',
      isComposing: true,
      text: 'native text',
    }),
  );
  const escapeKeydown = new KeyboardEvent('keydown', {
    key: 'Escape',
    bubbles: true,
    cancelable: true,
  });
  textarea.dispatchEvent(escapeKeydown);
  expect(nativeHandler).not.toHaveBeenCalled();
  expect(escapeKeydown.defaultPrevented).toBe(true);
  expect(permitsBenchAction).toHaveBeenCalledWith('textarea.escape');
  textarea.remove();
  window.history.replaceState(null, '', previousUrl);
});
