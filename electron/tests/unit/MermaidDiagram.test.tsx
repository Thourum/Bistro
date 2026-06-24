// @vitest-environment jsdom
import * as matchers from '@testing-library/jest-dom/matchers';
import { render, waitFor } from '@testing-library/react';
import mermaid from 'mermaid';
import { beforeEach, describe, expect, it, vi } from 'vitest';
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
});
