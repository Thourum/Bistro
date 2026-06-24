import mermaid from 'mermaid';
import { ResultAsync } from 'neverthrow';
import { useEffect, useRef, useState } from 'react';

type MermaidThemeMode = 'light' | 'dark';

const MERMAID_THEME_CSS = `
  .node rect, .node polygon, .cluster rect { rx: 0 !important; ry: 0 !important; }
  .edgePath path { stroke-width: 2px; }
  .label, .nodeLabel, .edgeLabel, .messageText, .loopText, .labelText, .actor, .actor > tspan {
    font-family: "JetBrains Mono", monospace !important;
  }
  .actor, .actor-man line, .actor-line, .messageLine0, .messageLine1, .loopLine, .note, .labelBox {
    stroke-width: 2px !important;
  }
  rect.actor, .actor-man rect, .labelBox, .note, .activation0, .activation1, .activation2 {
    rx: 0 !important;
    ry: 0 !important;
  }
  rect.rect, .rect {
    fill: transparent !important;
    stroke: #c3c6d7 !important;
    stroke-width: 2px !important;
  }
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
  actorBkg: '#ffffff',
  actorBorder: '#c3c6d7',
  actorTextColor: '#191c1d',
  actorLineColor: '#c3c6d7',
  signalColor: '#737686',
  signalTextColor: '#191c1d',
  labelBoxBkgColor: '#ffffff',
  labelBoxBorderColor: '#c3c6d7',
  labelTextColor: '#191c1d',
  loopTextColor: '#191c1d',
  activationBkgColor: '#f1f3f4',
  activationBorderColor: '#c3c6d7',
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
  actorBkg: '#1f2223',
  actorBorder: '#4d5357',
  actorTextColor: '#e1e3e4',
  actorLineColor: '#4d5357',
  signalColor: '#8d9199',
  signalTextColor: '#e1e3e4',
  labelBoxBkgColor: '#1f2223',
  labelBoxBorderColor: '#4d5357',
  labelTextColor: '#e1e3e4',
  loopTextColor: '#e1e3e4',
  activationBkgColor: '#25292b',
  activationBorderColor: '#4d5357',
  noteBkgColor: '#1f2223',
  noteTextColor: '#e1e3e4',
  noteBorderColor: '#4d5357',
  errorBkgColor: '#67060c',
  errorTextColor: '#ffdcd7',
};

function runtimeCss(mode: MermaidThemeMode): string {
  const surface = mode === 'dark' ? '#1f2223' : '#ffffff';
  const surfaceLow = mode === 'dark' ? '#25292b' : '#f1f3f4';
  const outline = mode === 'dark' ? '#4d5357' : '#c3c6d7';
  const shadow = mode === 'dark' ? '#111314' : '#c3c6d7';
  const text = mode === 'dark' ? '#e1e3e4' : '#191c1d';
  const metadata = mode === 'dark' ? '#8d9199' : '#737686';

  return `
    [data-bistro-mermaid] svg {
      background: transparent !important;
      font-family: "JetBrains Mono", monospace !important;
    }
    [data-bistro-mermaid] .actor,
    [data-bistro-mermaid] rect.actor,
    [data-bistro-mermaid] .actor-man rect,
    [data-bistro-mermaid] .node rect,
    [data-bistro-mermaid] .node polygon,
    [data-bistro-mermaid] .cluster rect,
    [data-bistro-mermaid] .note,
    [data-bistro-mermaid] .labelBox {
      fill: ${surface} !important;
      stroke: ${outline} !important;
      stroke-width: 2px !important;
      rx: 0 !important;
      ry: 0 !important;
      filter: drop-shadow(3px 3px 0 ${shadow}) !important;
    }
    [data-bistro-mermaid] svg.sequenceDiagram rect:not(.actor):not(.note):not(.activation0):not(.activation1):not(.activation2):not(.labelBox),
    [data-bistro-mermaid] svg[class*="sequence"] rect.rect,
    [data-bistro-mermaid] g[class*="sequence"] rect.rect,
    [data-bistro-mermaid] rect.rect {
      fill: ${surfaceLow} !important;
      stroke: ${outline} !important;
      stroke-width: 2px !important;
      opacity: 0.35 !important;
      rx: 0 !important;
      ry: 0 !important;
    }
    [data-bistro-mermaid] .activation0,
    [data-bistro-mermaid] .activation1,
    [data-bistro-mermaid] .activation2 {
      fill: ${surfaceLow} !important;
      stroke: ${outline} !important;
      stroke-width: 2px !important;
      filter: drop-shadow(3px 3px 0 ${shadow}) !important;
    }
    [data-bistro-mermaid] .actor-line,
    [data-bistro-mermaid] .actor-man line,
    [data-bistro-mermaid] .messageLine0,
    [data-bistro-mermaid] .messageLine1,
    [data-bistro-mermaid] .edgePath path,
    [data-bistro-mermaid] .loopLine {
      stroke: ${metadata} !important;
      stroke-width: 2px !important;
    }
    [data-bistro-mermaid] marker path,
    [data-bistro-mermaid] marker polygon {
      fill: ${metadata} !important;
      stroke: ${metadata} !important;
    }
    [data-bistro-mermaid] text,
    [data-bistro-mermaid] .label,
    [data-bistro-mermaid] .nodeLabel,
    [data-bistro-mermaid] .edgeLabel,
    [data-bistro-mermaid] .messageText,
    [data-bistro-mermaid] .loopText,
    [data-bistro-mermaid] .labelText,
    [data-bistro-mermaid] .actor > tspan {
      fill: ${text} !important;
      color: ${text} !important;
      font-family: "JetBrains Mono", monospace !important;
      font-size: 12px !important;
    }
  `;
}

function neutralizeDecorativeRectFills(
  svg: string,
  mode: MermaidThemeMode,
): string {
  const surfaceLow = mode === 'dark' ? '#25292b' : '#f1f3f4';
  const outline = mode === 'dark' ? '#4d5357' : '#c3c6d7';

  return svg.replace(/<rect\b[^>]*>/g, (tag) => {
    if (!/fill\s*=\s*"rgb\(/i.test(tag) && !/fill\s*:\s*rgb\(/i.test(tag)) {
      return tag;
    }
    const withFill = tag
      .replace(/\sfill\s*=\s*"rgb\([^"']+\)"/gi, ` fill="${surfaceLow}"`)
      .replace(/fill\s*:\s*rgb\([^)]+\)\s*;?/gi, `fill: ${surfaceLow};`);
    const withStroke = /\sstroke\s*=/.test(withFill)
      ? withFill
      : withFill.replace(/>$/, ` stroke="${outline}">`);
    const withStrokeWidth = /\sstroke-width\s*=/.test(withStroke)
      ? withStroke.replace(/\sstroke-width\s*=\s*"[^"]+"/, ' stroke-width="2"')
      : withStroke.replace(/>$/, ' stroke-width="2">');
    return /\sopacity\s*=/.test(withStrokeWidth)
      ? withStrokeWidth.replace(/\sopacity\s*=\s*"[^"]+"/, ' opacity="0.35"')
      : withStrokeWidth.replace(/>$/, ' opacity="0.35">');
  });
}

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
        if (!cancelled) setSvg(neutralizeDecorativeRectFills(result.svg, mode));
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
    <div
      className="my-3 overflow-x-auto border-2 border-outline-variant/60 bg-surface-container-low p-3 shadow-[4px_4px_0_var(--color-outline-variant)] [&_svg]:mx-auto [&_svg]:max-w-full"
      data-bistro-mermaid
    >
      <style>{runtimeCss(mode)}</style>
      {/* biome-ignore lint/security/noDangerouslySetInnerHtml: Mermaid SVG output with securityLevel strict */}
      <div dangerouslySetInnerHTML={{ __html: svg }} />
    </div>
  );
}
