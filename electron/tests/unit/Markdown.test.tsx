// @vitest-environment jsdom
import * as matchers from '@testing-library/jest-dom/matchers';
import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Markdown } from '../../src/renderer/components/Markdown';

expect.extend(matchers);

describe('<Markdown> rendering', () => {
  it('renders a GFM table', () => {
    const { container } = render(<Markdown body={'| a | b |\n|---|---|\n| 1 | 2 |'} />);
    expect(container.querySelector('table')).not.toBeNull();
    expect(container.querySelector('td')?.textContent).toBe('1');
  });

  it('renders task lists and strikethrough', () => {
    const { container } = render(<Markdown body={'- [x] done\n\n~~gone~~'} />);
    expect(container.querySelector('input[type="checkbox"]')).not.toBeNull();
    expect(container.querySelector('del')?.textContent).toBe('gone');
  });

  it('highlights a fenced typescript block as a <pre> block', () => {
    const { container } = render(<Markdown body={'```ts\nconst x = 1;\n```'} />);
    const code = container.querySelector('pre code.language-ts');
    expect(code).not.toBeNull();
    expect(code?.innerHTML).toContain('hljs-');
  });

  it('highlights a fenced diff block with add/remove classes', () => {
    const { container } = render(<Markdown body={'```diff\n+new\n-old\n```'} />);
    const html =
      container.querySelector('pre code.language-diff')?.innerHTML ?? '';
    expect(html).toContain('hljs-addition');
    expect(html).toContain('hljs-deletion');
  });

  it('renders an UNLABELED fence as a <pre><code> block, not inline', () => {
    const { container } = render(<Markdown body={'```\nplain text\n```'} />);
    expect(container.querySelector('pre')).not.toBeNull();
    expect(container.querySelector('pre code')?.textContent).toContain(
      'plain text',
    );
  });

  it('renders an INDENTED code block as a <pre><code> block, not inline', () => {
    const { container } = render(<Markdown body={'    indented code line'} />);
    expect(container.querySelector('pre code')?.textContent).toContain(
      'indented code line',
    );
  });

  it('renders inline code as a bare <code> (no <pre>)', () => {
    const { container } = render(<Markdown body={'use `npm test` now'} />);
    const inline = container.querySelector('code');
    expect(inline).not.toBeNull();
    expect(inline?.closest('pre')).toBeNull();
  });

  it('extracts text from a raw <pre><code><span> structure (HAST, not React children)', () => {
    // NOTE: lang label must be a REGISTERED language — use `language-typescript`,
    // NOT `language-js` (only `javascript`/`typescript` etc. are registered; `js` is not an alias).
    const { container } = render(
      <Markdown
        body={
          '<pre><code class="language-typescript"><span>const</span> y</code></pre>'
        }
      />,
    );
    const code = container.querySelector('pre code.language-typescript');
    expect(code?.innerHTML).toContain('hljs-'); // re-highlighted from extracted text
    expect(code?.textContent).toContain('const');
  });

  it('renders <details open><summary> and retains open; nested ```diff colorizes', () => {
    const { container } = render(
      <Markdown
        body={
          '<details open><summary>more</summary>\n\n```diff\n+a\n-b\n```\n\n</details>'
        }
      />,
    );
    const details = container.querySelector('details');
    expect(details).not.toBeNull();
    expect(details?.hasAttribute('open')).toBe(true);
    expect(container.querySelector('summary')?.textContent).toBe('more');
    expect(
      container.querySelector('details pre code.language-diff'),
    ).not.toBeNull();
  });

  it('keeps GitHub refs/mentions as plain text (v1 non-goal)', () => {
    const { container } = render(
      <Markdown body={'see #123 and @octocat'} />,
    );
    expect(container.querySelectorAll('a').length).toBe(0);
    expect(container.textContent).toContain('#123');
    expect(container.textContent).toContain('@octocat');
  });
});

describe('<Markdown> security (raw-HTML path is the highest-risk surface)', () => {
  it('strips <script>', () => {
    const { container } = render(
      <Markdown body={'<script>alert(1)</script>hello'} />,
    );
    expect(container.querySelector('script')).toBeNull();
    expect(container.textContent).toContain('hello');
  });

  it('strips event-handler attributes (img is blocked entirely by schema)', () => {
    const { container } = render(
      <Markdown body={'<img src="x" onerror="alert(1)" />'} />,
    );
    // img is removed by the sanitize schema — onerror cannot exist without img
    expect(container.querySelector('img')).toBeNull();
  });

  it('neutralizes javascript: links', () => {
    const { container } = render(
      <Markdown body={'[click](javascript:alert(1))'} />,
    );
    expect(
      (
        container.querySelector('a')?.getAttribute('href') ?? ''
      ).startsWith('javascript:'),
    ).toBe(false);
  });

  it('does not break out of a code fence via </code><script>', () => {
    const { container } = render(
      <Markdown body={'```\n</code><script>alert(1)</script>\n```'} />,
    );
    expect(container.querySelector('script')).toBeNull();
    expect(container.textContent).toContain('<script>');
  });
});
