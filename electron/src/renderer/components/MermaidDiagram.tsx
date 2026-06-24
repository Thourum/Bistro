import mermaid from 'mermaid';
import { ResultAsync } from 'neverthrow';
import { useEffect, useRef, useState } from 'react';

type MermaidThemeMode = 'light' | 'dark';

const MERMAID_THEME_CSS = `
  .node rect, .node polygon, .cluster rect { rx: 0 !important; ry: 0 !important; }
  .edgePath path { stroke-width: 1px; }
  .label, .nodeLabel, .edgeLabel { font-family: "JetBrains Mono", monospace !important; }
`;

const LIGHT_THEME_VARIABLES = {
  darkMode: false,
  background: '#f8f9fa',
  primaryColor: '#ffffff',
  primaryBorderColor: '#c3c6d7',
  primaryTextColor: '#191c1d',
  textColor: '#191c1d',
  lineColor: '#737686',
  secondaryColor: '#f1f3f4',
  tertiaryColor: '#f1f3f4',
  fontFamily: '"JetBrains Mono", monospace',
  fontSize: '12px',
  noteBkgColor: '#ffffff',
  noteTextColor: '#191c1d',
  noteBorderColor: '#c3c6d7',
  errorBkgColor: '#ffeef0',
  errorTextColor: '#ba1a1a',
};

const DARK_THEME_VARIABLES = {
  darkMode: true,
  background: '#191c1d',
  primaryColor: '#1f2223',
  primaryBorderColor: '#4d5357',
  primaryTextColor: '#e1e3e4',
  textColor: '#e1e3e4',
  lineColor: '#8d9199',
  secondaryColor: '#25292b',
  tertiaryColor: '#25292b',
  fontFamily: '"JetBrains Mono", monospace',
  fontSize: '12px',
  noteBkgColor: '#1f2223',
  noteTextColor: '#e1e3e4',
  noteBorderColor: '#4d5357',
  errorBkgColor: '#67060c',
  errorTextColor: '#ffdcd7',
};

function mermaidConfig(mode: MermaidThemeMode) {
  return {
    startOnLoad: false,
    securityLevel: 'strict' as const,
    look: 'classic' as const,
    theme: 'base' as const,
    fontFamily: '"JetBrains Mono", monospace',
    fontSize: 12,
    logLevel: 'error' as const,
    themeVariables:
      mode === 'dark' ? DARK_THEME_VARIABLES : LIGHT_THEME_VARIABLES,
    themeCSS: MERMAID_THEME_CSS,
  };
}

mermaid.initialize(mermaidConfig('light'));

let componentCounter = 0;
let renderCounter = 0;

function nextComponentId(): string {
  componentCounter += 1;
  return `bistro-mermaid-${componentCounter}`;
}

function nextRenderId(componentId: string): string {
  renderCounter += 1;
  return `${componentId}-${renderCounter}`;
}

function currentThemeMode(): MermaidThemeMode {
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light';
}

function useMermaidThemeMode(): MermaidThemeMode {
  const [mode, setMode] = useState<MermaidThemeMode>(() => currentThemeMode());

  useEffect(() => {
    const observer = new MutationObserver(() => setMode(currentThemeMode()));
    observer.observe(document.documentElement, {
      attributeFilter: ['class'],
      attributes: true,
    });
    return () => observer.disconnect();
  }, []);

  return mode;
}

interface MermaidDiagramProps {
  source: string;
}

/**
 * Renders a Mermaid diagram from an untrusted markdown fence.
 *
 * Mermaid runs with `securityLevel: strict`; invalid diagrams fall back to a
 * plaintext block so comments never crash. The SVG injection is isolated here
 * and mirrors the reviewed HighlightedCode boundary.
 */
export function MermaidDiagram({
  source,
}: MermaidDiagramProps): React.JSX.Element {
  const componentId = useRef<string>(nextComponentId());
  const mode = useMermaidThemeMode();
  const [svg, setSvg] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setSvg(null);
    setFailed(false);
    mermaid.initialize(mermaidConfig(mode));
    const renderId = nextRenderId(componentId.current);

    void ResultAsync.fromPromise(
      mermaid.render(renderId, source),
      () => null,
    ).match(
      (result) => {
        if (!cancelled) setSvg(result.svg);
      },
      () => {
        if (!cancelled) setFailed(true);
      },
    );

    return () => {
      cancelled = true;
    };
  }, [source, mode]);

  if (failed) {
    return (
      <pre className="my-3 overflow-x-auto border border-outline-variant/40 bg-surface-container-low p-3 font-mono text-xs text-on-surface">
        <code>{source}</code>
      </pre>
    );
  }

  if (svg === null) {
    return (
      <div className="my-3 border border-outline-variant/40 bg-surface-container-low p-3 font-mono text-[10px] uppercase tracking-widest text-outline">
        RENDERING_DIAGRAM…
      </div>
    );
  }

  return (
    <div className="my-3 overflow-x-auto border border-outline-variant/40 bg-surface-container-low p-3 [&_svg]:mx-auto [&_svg]:max-w-full">
      {/* biome-ignore lint/security/noDangerouslySetInnerHtml: Mermaid SVG output with securityLevel strict */}
      <div dangerouslySetInnerHTML={{ __html: svg }} />
    </div>
  );
}
