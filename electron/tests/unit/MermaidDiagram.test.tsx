// @vitest-environment jsdom
import * as matchers from '@testing-library/jest-dom/matchers';
import { act, render, waitFor } from '@testing-library/react';
import mermaid from 'mermaid';
import { StrictMode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Markdown } from '../../src/renderer/components/Markdown';
import { MermaidDiagram } from '../../src/renderer/components/MermaidDiagram';

vi.mock('mermaid', () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn(),
  },
}));

expect.extend(matchers);

const mockedMermaid = vi.mocked(mermaid);

beforeEach(() => {
  vi.clearAllMocks();
  document.documentElement.className = '';
  mockedMermaid.render.mockResolvedValue({
    svg: '<svg data-testid="mermaid-svg"><text>A → B</text></svg>',
    bindFunctions: undefined,
  });
});

afterEach(() => {
  vi.useRealTimers();
});

describe('<MermaidDiagram>', () => {
  it('renders valid Mermaid source as injected SVG', async () => {
    render(<MermaidDiagram source="graph TD; A-->B" />);

    await waitFor(() => {
      expect(
        document.querySelector('[data-testid="mermaid-svg"]'),
      ).not.toBeNull();
    });
    expect(mockedMermaid.render).toHaveBeenCalledWith(
      expect.stringMatching(/^bistro-mermaid-\d+-\d+$/),
      'graph TD; A-->B',
    );
  });

  it('falls back to plaintext when Mermaid rejects invalid source', async () => {
    mockedMermaid.render.mockRejectedValue(new Error('invalid mermaid'));
    const { container } = render(<MermaidDiagram source="not valid" />);

    await waitFor(() => {
      expect(container.querySelector('pre code')?.textContent).toBe(
        'not valid',
      );
    });
    expect(container.querySelector('svg')).toBeNull();
  });

  it('falls back to plaintext without rendering oversized sources', async () => {
    const oversized = `graph TD; A-->B\n${'x'.repeat(8001)}`;
    const { container } = render(<MermaidDiagram source={oversized} />);

    await waitFor(() => {
      expect(container.querySelector('pre code')?.textContent).toBe(oversized);
    });
    expect(mockedMermaid.render).not.toHaveBeenCalled();
  });

  it('falls back to plaintext when Mermaid rendering times out', async () => {
    vi.useFakeTimers();
    mockedMermaid.render.mockReturnValue(new Promise(() => {}));
    const { container } = render(<MermaidDiagram source="graph TD; A-->B" />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });

    expect(container.querySelector('pre code')?.textContent).toBe(
      'graph TD; A-->B',
    );
  });

  it('uses dark theme variables when the app has the dark class', async () => {
    document.documentElement.classList.add('dark');
    render(<MermaidDiagram source="graph TD; A-->B" />);

    await waitFor(() => expect(mockedMermaid.render).toHaveBeenCalled());
    expect(mockedMermaid.initialize).toHaveBeenLastCalledWith(
      expect.objectContaining({
        themeVariables: expect.objectContaining({ darkMode: true }),
      }),
    );
  });

  it('neutralizes Mermaid rect directive colors to Bistro surfaces', async () => {
    mockedMermaid.render.mockResolvedValue({
      svg: '<svg><rect x="0" y="0" fill="rgb(200, 255, 200)" width="100" height="40" class="rect"></rect></svg>',
      bindFunctions: undefined,
    });
    render(
      <MermaidDiagram source="sequenceDiagram\nrect rgb(200, 255, 200)\nA->>B: x\nend" />,
    );

    await waitFor(() => {
      expect(document.querySelector('rect.rect')?.getAttribute('fill')).toBe(
        '#f1f3f4',
      );
    });
    expect(document.querySelector('rect.rect')?.getAttribute('stroke')).toBe(
      '#c3c6d7',
    );
    expect(
      document.querySelector('rect.rect')?.getAttribute('stroke-width'),
    ).toBe('2');
    expect(document.querySelector('rect.rect')?.getAttribute('opacity')).toBe(
      '0.35',
    );
  });

  it('routes ```mermaid fences through MermaidDiagram from Markdown', async () => {
    render(<Markdown body={'```mermaid\ngraph TD; A-->B\n```'} />);

    await waitFor(() => {
      expect(
        document.querySelector('[data-testid="mermaid-svg"]'),
      ).not.toBeNull();
    });
    expect(mockedMermaid.render).toHaveBeenCalledWith(
      expect.any(String),
      'graph TD; A-->B\n',
    );
  });

  it('renders multiple Mermaid fences in the same markdown comment under StrictMode', async () => {
    mockedMermaid.render.mockImplementation((id, source) =>
      Promise.resolve({
        svg: `<svg data-testid="mermaid-svg" data-render-id="${id}"><text>${source.includes('DiffPage') ? 'inline diff comment flow' : 'markdown rendering flow'}</text></svg>`,
        bindFunctions: undefined,
      }),
    );

    render(
      <StrictMode>
        <Markdown
          body={`## Sequence Diagram(s)

**Markdown rendering flow**


\`\`\`mermaid
sequenceDiagram
  participant Markdown
  participant ReactMarkdown
  participant markdownComponents
  participant HighlightedCode
  participant MermaidDiagram
  participant ExternalLink
  Markdown->>ReactMarkdown: render body
  ReactMarkdown->>markdownComponents: resolve markdown nodes
  markdownComponents->>HighlightedCode: render fenced code blocks
  markdownComponents->>MermaidDiagram: render mermaid fences
  markdownComponents->>ExternalLink: render https links
\`\`\`

**Inline diff comment flow**

\`\`\`mermaid
sequenceDiagram
  participant DiffPage
  participant request
  participant PierreDiffView
  participant appStore
  participant keybindings
  participant ReplyComposer
  DiffPage->>request: REQUEST_COMMENTS for pr_id
  request-->>DiffPage: comments
  DiffPage->>PierreDiffView: threads and comments
  PierreDiffView->>appStore: inlineCommentThreadIds and active thread
  keybindings->>appStore: j/k, Enter, and Esc update inline reply state
  PierreDiffView->>ReplyComposer: open inline reply composer
\`\`\``}
        />
      </StrictMode>,
    );

    await waitFor(() => {
      expect(
        document.querySelectorAll('[data-testid="mermaid-svg"]'),
      ).toHaveLength(2);
    });
    expect(document.body.textContent).toContain('markdown rendering flow');
    expect(document.body.textContent).toContain('inline diff comment flow');
  });
});
