import { Fault, ResultAsync } from '@thourum/fault';
import { useEffect, useState } from 'react';

type MermaidThemeMode = 'light' | 'dark';

const MAX_MERMAID_SOURCE_CHARS = 8000;
const MERMAID_RENDER_TIMEOUT_MS = 2500;

interface MermaidColors {
  readonly background: string;
  readonly surface: string;
  readonly surfaceLow: string;
  readonly outline: string;
  readonly shadow: string;
  readonly text: string;
  readonly metadata: string;
  readonly errorText: string;
  readonly errorContainer: string;
  readonly fontFamily: string;
}

function readMermaidColors(): MermaidColors {
  const styles = getComputedStyle(document.documentElement);
  const color = (token: string) =>
    styles.getPropertyValue(`--color-${token}`).trim();

  return {
    background: color('surface'),
    surface: color('surface-container-lowest'),
    surfaceLow: color('surface-container-low'),
    outline: color('outline-variant'),
    shadow: styles.getPropertyValue('--color-mermaid-shadow').trim(),
    text: color('on-surface'),
    metadata: color('outline'),
    errorText: color('on-error-container') || color('on-error'),
    errorContainer: color('error-container') || color('surface-container-low'),
    fontFamily: styles.getPropertyValue('--app-code-font').trim(),
  };
}

function mermaidThemeCss(colors: MermaidColors): string {
  return `
    .node rect, .node polygon, .cluster rect { rx: 0 !important; ry: 0 !important; }
    .edgePath path { stroke-width: 2px; }
    .label, .nodeLabel, .edgeLabel, .messageText, .loopText, .labelText, .actor, .actor > tspan {
      font-family: ${colors.fontFamily} !important;
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
      stroke: ${colors.outline} !important;
      stroke-width: 2px !important;
    }
  `;
}

function mermaidThemeVariables(colors: MermaidColors, mode: MermaidThemeMode) {
  return {
    darkMode: mode === 'dark',
    background: colors.background,
    primaryColor: colors.surface,
    primaryBorderColor: colors.outline,
    primaryTextColor: colors.text,
    textColor: colors.text,
    lineColor: colors.metadata,
    secondaryColor: colors.surfaceLow,
    tertiaryColor: colors.surfaceLow,
    fontFamily: colors.fontFamily,
    fontSize: '12px',
    actorBkg: colors.surface,
    actorBorder: colors.outline,
    actorTextColor: colors.text,
    actorLineColor: colors.outline,
    signalColor: colors.metadata,
    signalTextColor: colors.text,
    labelBoxBkgColor: colors.surface,
    labelBoxBorderColor: colors.outline,
    labelTextColor: colors.text,
    loopTextColor: colors.text,
    activationBkgColor: colors.surfaceLow,
    activationBorderColor: colors.outline,
    noteBkgColor: colors.surface,
    noteTextColor: colors.text,
    noteBorderColor: colors.outline,
    errorBkgColor: colors.errorContainer,
    errorTextColor: colors.errorText,
  };
}

function runtimeCss(colors: MermaidColors): string {
  return `
    [data-bistro-mermaid] svg {
      background: transparent !important;
      font-family: ${colors.fontFamily} !important;
    }
    [data-bistro-mermaid] rect.actor,
    [data-bistro-mermaid] .actor-man rect,
    [data-bistro-mermaid] .node rect,
    [data-bistro-mermaid] .node polygon,
    [data-bistro-mermaid] .cluster rect,
    [data-bistro-mermaid] .note,
    [data-bistro-mermaid] .labelBox {
      fill: ${colors.surface} !important;
      stroke: ${colors.outline} !important;
      stroke-width: 2px !important;
      rx: 0 !important;
      ry: 0 !important;
      filter: drop-shadow(4px 4px 0 ${colors.shadow}) !important;
    }
    [data-bistro-mermaid] svg.sequenceDiagram rect:not(.actor):not(.note):not(.activation0):not(.activation1):not(.activation2):not(.labelBox),
    [data-bistro-mermaid] svg[class*="sequence"] rect.rect,
    [data-bistro-mermaid] g[class*="sequence"] rect.rect,
    [data-bistro-mermaid] rect.rect {
      fill: ${colors.surfaceLow} !important;
      stroke: ${colors.outline} !important;
      stroke-width: 2px !important;
      opacity: 0.35 !important;
      rx: 0 !important;
      ry: 0 !important;
    }
    [data-bistro-mermaid] .activation0,
    [data-bistro-mermaid] .activation1,
    [data-bistro-mermaid] .activation2 {
      fill: ${colors.surfaceLow} !important;
      stroke: ${colors.outline} !important;
      stroke-width: 2px !important;
      filter: drop-shadow(4px 4px 0 ${colors.shadow}) !important;
    }
    [data-bistro-mermaid] .actor-line,
    [data-bistro-mermaid] .actor-man line,
    [data-bistro-mermaid] .messageLine0,
    [data-bistro-mermaid] .messageLine1,
    [data-bistro-mermaid] .edgePath path,
    [data-bistro-mermaid] .loopLine {
      stroke: ${colors.metadata} !important;
      stroke-width: 2px !important;
    }
    [data-bistro-mermaid] marker path,
    [data-bistro-mermaid] marker polygon {
      fill: ${colors.metadata} !important;
      stroke: ${colors.metadata} !important;
    }
    [data-bistro-mermaid] text,
    [data-bistro-mermaid] .label,
    [data-bistro-mermaid] .nodeLabel,
    [data-bistro-mermaid] .edgeLabel,
    [data-bistro-mermaid] .messageText,
    [data-bistro-mermaid] .loopText,
    [data-bistro-mermaid] .labelText,
    [data-bistro-mermaid] .actor > tspan {
      fill: ${colors.text} !important;
      color: ${colors.text} !important;
      font-family: ${colors.fontFamily} !important;
      font-size: 12px !important;
    }
  `;
}

function neutralizeDecorativeRectFills(
  svg: string,
  colors: MermaidColors,
): string {
  return svg.replace(/<rect\b[^>]*>/g, (tag) => {
    if (!/fill\s*=\s*"rgb\(/i.test(tag) && !/fill\s*:\s*rgb\(/i.test(tag)) {
      return tag;
    }
    const withFill = tag
      .replace(/\sfill\s*=\s*"rgb\([^"']+\)"/gi, ` fill="${colors.surfaceLow}"`)
      .replace(/fill\s*:\s*rgb\([^)]+\)\s*;?/gi, `fill: ${colors.surfaceLow};`);
    const withStroke = /\sstroke\s*=/.test(withFill)
      ? withFill
      : withFill.replace(/>$/, ` stroke="${colors.outline}">`);
    const withStrokeWidth = /\sstroke-width\s*=/.test(withStroke)
      ? withStroke.replace(/\sstroke-width\s*=\s*"[^"]+"/, ' stroke-width="2"')
      : withStroke.replace(/>$/, ' stroke-width="2">');
    return /\sopacity\s*=/.test(withStrokeWidth)
      ? withStrokeWidth.replace(/\sopacity\s*=\s*"[^"]+"/, ' opacity="0.35"')
      : withStrokeWidth.replace(/>$/, ' opacity="0.35">');
  });
}

function mermaidConfig(colors: MermaidColors, mode: MermaidThemeMode) {
  return {
    startOnLoad: false,
    securityLevel: 'strict' as const,
    look: 'classic' as const,
    theme: 'base' as const,
    fontFamily: colors.fontFamily,
    fontSize: 12,
    logLevel: 'error' as const,
    themeVariables: mermaidThemeVariables(colors, mode),
    themeCSS: mermaidThemeCss(colors),
  };
}

interface MermaidRenderResult {
  svg: string;
  colors: MermaidColors;
}

// Mermaid configuration is global, so begin each render after the prior one
// finishes. A timed-out render releases the queue, as the previous semaphore did.
const mermaidRenderQueue = (() => {
  let nextId = 0;
  let lastRender: Promise<void> = Promise.resolve();

  function render(
    source: string,
    isCancelled: () => boolean,
  ): ResultAsync<MermaidRenderResult | null, Fault> {
    const pending = lastRender.then(async () => {
      if (isCancelled()) return null;
      // Loaded on first diagram so Mermaid stays out of the startup chunk.
      const { default: mermaid } = await import('mermaid');
      if (isCancelled()) return null;
      const colors = readMermaidColors();
      const renderId = `bistro-mermaid-${++nextId}`;
      mermaid.initialize(mermaidConfig(colors, currentThemeMode()));
      let timeoutId: ReturnType<typeof setTimeout> | undefined;
      const timeout = new Promise<never>((_, reject) => {
        timeoutId = setTimeout(
          () =>
            reject(
              new Fault('Mermaid render timed out').withTag('TIMEOUT_ERROR'),
            ),
          MERMAID_RENDER_TIMEOUT_MS,
        );
      });
      return Promise.race([
        mermaid.render(renderId, source).then(({ svg }) => ({ svg, colors })),
        timeout,
      ]).finally(() => clearTimeout(timeoutId));
    });
    const result = ResultAsync.fromPromise(pending, (cause) =>
      cause instanceof Fault && cause.tag === 'TIMEOUT_ERROR'
        ? cause
        : new Fault(cause instanceof Error ? cause : 'Mermaid render failed')
            .withTag('MERMAID_RENDER_ERROR')
            .withCause(cause),
    );
    lastRender = Promise.resolve(result).then(() => undefined);
    return result;
  }

  return { render };
})();

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
  const mode = useMermaidThemeMode();
  const [rendered, setRendered] = useState<MermaidRenderResult | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setRendered(null);
    setFailed(false);
    if (source.length > MAX_MERMAID_SOURCE_CHARS) {
      setFailed(true);
      return;
    }

    // Mode invalidates this render; the queue reads the current theme on start.
    void mermaidRenderQueue
      .render(source, () => cancelled || mode !== currentThemeMode())
      .match(
        (result) => {
          if (!cancelled && result !== null) setRendered(result);
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

  if (rendered === null) {
    return (
      <div className="my-3 border border-outline-variant/40 bg-surface-container-low p-3 font-mono text-[10px] uppercase tracking-widest text-outline">
        RENDERING_DIAGRAM…
      </div>
    );
  }

  return (
    <div
      className="my-3 overflow-x-auto border-2 border-outline-variant/60 bg-surface-container-low p-3 shadow-[4px_4px_0_var(--color-mermaid-shadow)] [&_svg]:mx-auto [&_svg]:max-w-full"
      data-bistro-mermaid
    >
      <style>{runtimeCss(rendered.colors)}</style>
      <div
        // biome-ignore lint/security/noDangerouslySetInnerHtml: Mermaid SVG output with securityLevel strict
        dangerouslySetInnerHTML={{
          __html: neutralizeDecorativeRectFills(rendered.svg, rendered.colors),
        }}
      />
    </div>
  );
}
