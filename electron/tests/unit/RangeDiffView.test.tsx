// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import {
  FooterSlotProvider,
  useFooterSlotContent,
} from '../../src/renderer/components/FooterSlot';
import { RangeDiffView } from '../../src/renderer/components/RangeDiffView';
import type { RangeDiffResult } from '../../src/renderer/types';

expect.extend(matchers);

/** Renders whatever the center footer slot currently holds — stands in for AppFooter. */
function FooterProbe(): React.ReactElement {
  const slots = useFooterSlotContent();
  return <div data-testid="footer-center">{slots.center}</div>;
}

function wrap(node: React.ReactNode): React.ReactElement {
  return (
    <FooterSlotProvider>
      {node}
      <FooterProbe />
    </FooterSlotProvider>
  );
}

function result(over: Partial<RangeDiffResult>): RangeDiffResult {
  return {
    pr_id: 'pr-1',
    from_patchset: 1,
    to_patchset: 2,
    from_sha: 'a'.repeat(40),
    to_sha: 'b'.repeat(40),
    raw: '',
    degraded: false,
    fallback_reason: '',
    ...over,
  };
}

describe('RangeDiffView', () => {
  it('renders raw diff text when content is present', () => {
    render(
      wrap(
        <RangeDiffView
          result={result({ raw: '@@ -1 +1 @@\n-old line\n+new line' })}
        />,
      ),
    );
    expect(screen.getByText('-old line')).toBeInTheDocument();
    expect(screen.getByText('+new line')).toBeInTheDocument();
  });

  it('shows a no-content state (not a fake diff) when raw is empty', () => {
    render(
      wrap(
        <RangeDiffView
          result={result({
            raw: '',
            degraded: true,
            fallback_reason: 'no prior review recorded',
          })}
        />,
      ),
    );
    expect(screen.getByText(/nothing to compare/i)).toBeInTheDocument();
    expect(screen.getByText(/no prior review recorded/i)).toBeInTheDocument();
  });

  it('mounts the degraded footer indicator when degraded', () => {
    render(
      wrap(
        <RangeDiffView
          result={result({
            raw: 'diff --git a/x b/x\n+added',
            degraded: true,
            fallback_reason: 'system git not available',
          })}
        />,
      ),
    );
    // The indicator is pushed into the footer center slot, surfaced here by
    // FooterProbe — it is never rendered on the diff surface itself.
    expect(screen.getByText(/range-diff unavailable/i)).toBeInTheDocument();
    // The actual diff still renders alongside the footer indicator.
    expect(screen.getByText('+added')).toBeInTheDocument();
  });

  it('does not show the degraded indicator on a healthy result', () => {
    render(wrap(<RangeDiffView result={result({ raw: '+ok' })} />));
    expect(screen.queryByText(/range-diff unavailable/i)).toBeNull();
  });
});
