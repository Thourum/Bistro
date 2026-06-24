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
