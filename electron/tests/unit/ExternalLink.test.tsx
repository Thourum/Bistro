// @vitest-environment jsdom
import * as matchers from '@testing-library/jest-dom/matchers';
import { fireEvent, render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ExternalLink } from '../../src/renderer/components/ExternalLink';

expect.extend(matchers);

let openExternal: ReturnType<typeof vi.fn>;

// Use Object.defineProperty so we mutate only window.electron — never replace
// the window object itself (which would break jsdom internals).
beforeEach(() => {
  openExternal = vi.fn().mockResolvedValue({ ok: true });
  Object.defineProperty(window, 'electron', {
    value: { openExternal },
    writable: true,
    configurable: true,
  });
});

afterEach(() => {
  Object.defineProperty(window, 'electron', {
    value: undefined,
    writable: true,
    configurable: true,
  });
});

describe('<ExternalLink>', () => {
  it('opens https links via the bridge and prevents navigation', () => {
    const { getByText } = render(
      <ExternalLink href="https://example.com">go</ExternalLink>,
    );
    const event = new MouseEvent('click', { bubbles: true, cancelable: true });
    fireEvent(getByText('go'), event);
    expect(openExternal).toHaveBeenCalledWith('https://example.com');
    expect(event.defaultPrevented).toBe(true);
  });

  it('does not open http:// hrefs', () => {
    const { getByText } = render(
      <ExternalLink href="http://example.com">x</ExternalLink>,
    );
    fireEvent.click(getByText('x'));
    expect(openExternal).not.toHaveBeenCalled();
  });

  it('does not open relative hrefs', () => {
    const { getByText } = render(
      <ExternalLink href="/relative/path">x</ExternalLink>,
    );
    fireEvent.click(getByText('x'));
    expect(openExternal).not.toHaveBeenCalled();
  });

  it('does not open when href is undefined', () => {
    // <a> without href has no ARIA link role — query by text
    const { getByText } = render(<ExternalLink>x</ExternalLink>);
    fireEvent.click(getByText('x'));
    expect(openExternal).not.toHaveBeenCalled();
  });

  it('always prevents default even for non-https hrefs', () => {
    const { getByText } = render(
      <ExternalLink href="ftp://example.com">x</ExternalLink>,
    );
    const event = new MouseEvent('click', { bubbles: true, cancelable: true });
    fireEvent(getByText('x'), event);
    expect(event.defaultPrevented).toBe(true);
    expect(openExternal).not.toHaveBeenCalled();
  });

  it('renders with rel="noopener noreferrer"', () => {
    const { getByText } = render(
      <ExternalLink href="https://example.com">link</ExternalLink>,
    );
    const a = getByText('link').closest('a');
    expect(a).toHaveAttribute('rel', 'noopener noreferrer');
  });
});
