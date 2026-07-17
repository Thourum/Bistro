// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useEffect } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { OverlayProvider, useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import type { PR } from '../types';
import { CommandPalette } from './CommandPalette';

const navigateMock = vi.fn();
const requestMock = vi.fn();
const sendMock = vi.fn();
let routeParams: { prId?: string } = {};

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateMock,
  useParams: () => routeParams,
}));

vi.mock('../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: requestMock,
    send: sendMock,
  }),
}));

let visiblePRs: PR[] = [];

vi.mock('../hooks/useVisiblePRs', () => ({
  useVisiblePRs: () => ({ prs: visiblePRs }),
}));

function OpenCommandOnMount(): null {
  const { openCommand } = useOverlays();

  useEffect(() => {
    openCommand();
  }, [openCommand]);

  return null;
}

function ActiveOverlayProbe(): React.ReactElement {
  const { activeOverlay } = useOverlays();
  return <div data-testid="active-overlay">{activeOverlay ?? 'none'}</div>;
}

function makePR(overrides: Partial<PR>): PR {
  return {
    id: overrides.id ?? 'pr-1',
    provider: overrides.provider ?? 'github',
    owner: overrides.owner ?? 'acme',
    repo: overrides.repo ?? 'api',
    number: overrides.number ?? 1,
    title: overrides.title ?? 'PR title',
    body: null,
    author: overrides.author ?? 'alice',
    state: 'open',
    created_at: overrides.created_at ?? '2026-07-15T00:00:00Z',
    updated_at: overrides.updated_at ?? '2026-07-15T00:00:00Z',
    merged_at: null,
    base_branch: 'main',
    head_branch: 'feature',
    base_sha: 'base',
    head_sha: 'head',
    comment_count: 0,
    ci_status: null,
    draft: 0,
    review_status: null,
    approved_by: null,
    changes_requested_by: null,
    requested_reviewers: [],
    touches_you: false,
  };
}

function renderOpenPalette(): void {
  render(
    <OverlayProvider>
      <OpenCommandOnMount />
      <ActiveOverlayProbe />
      <CommandPalette />
    </OverlayProvider>,
  );
}

describe('CommandPalette add repository command', () => {
  beforeEach(() => {
    navigateMock.mockReset();
    requestMock.mockReset();
    sendMock.mockReset();
    routeParams = {};
    visiblePRs = [];
    appStore.setState(() => initialUIState);
  });

  afterEach(() => {
    appStore.setState(() => initialUIState);
  });

  it('sends expertise commands for the routed PR on detail pages', async () => {
    const user = userEvent.setup();
    routeParams = { prId: 'pr-focused' };
    visiblePRs = [
      makePR({ id: 'pr-list-row', owner: 'wrong', repo: 'repo', number: 1 }),
      makePR({ id: 'pr-focused', owner: 'right', repo: 'repo', number: 2 }),
    ];
    appStore.setState((state) => ({ ...state, selectedPRIndex: 0 }));
    renderOpenPalette();

    await user.type(screen.getByRole('combobox'), 'enable expertise');
    await user.click(screen.getByRole('option', { name: /enable expertise/i }));

    expect(sendMock).toHaveBeenCalledWith('EXPERTISE_ENABLE', {
      repo_id: 'github:right/repo',
    });
  });

  it('opens the add repository overlay when selected', async () => {
    const user = userEvent.setup();
    renderOpenPalette();

    await user.type(screen.getByRole('combobox'), 'add repository');
    await user.click(
      screen.getByRole('option', { name: /add repository by url/i }),
    );

    expect(screen.getByTestId('active-overlay')).toHaveTextContent('addRepo');
    expect(navigateMock).not.toHaveBeenCalled();
  });
});
