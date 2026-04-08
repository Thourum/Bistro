// @vitest-environment jsdom
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import StatusSidebar from '../../src/renderer/components/StatusSidebar';
import type { PR, CICheck } from '../../src/renderer/types';

const basePR: PR = {
  id: 'pr-1',
  provider: 'github',
  owner: 'org',
  repo: 'repo',
  number: 42,
  title: 'Test PR',
  body: null,
  author: 'alice',
  state: 'open',
  created_at: '2026-01-01',
  updated_at: '2026-01-02',
  merged_at: null,
  base_branch: 'main',
  head_branch: 'feat',
  base_sha: 'abc',
  head_sha: 'def',
  comment_count: 3,
  ci_status: null,
  draft: 0,
  review_status: 'approved',
  approved_by: 'bob',
  changes_requested_by: null,
};

describe('StatusSidebar', () => {
  it('renders review status badge', () => {
    render(<StatusSidebar pr={basePR} ciChecks={[]} />);
    expect(screen.getByText(/approved/i)).toBeInTheDocument();
  });

  it('renders reviewer name', () => {
    render(<StatusSidebar pr={basePR} ciChecks={[]} />);
    expect(screen.getByText(/bob/)).toBeInTheDocument();
  });

  it('renders CI checks with status icons', () => {
    const checks: CICheck[] = [
      { name: 'build', state: 'success' },
      { name: 'lint', state: 'failure' },
      { name: 'test', state: 'pending' },
    ];
    render(<StatusSidebar pr={basePR} ciChecks={checks} />);
    expect(screen.getByText('build')).toBeInTheDocument();
    expect(screen.getByText('lint')).toBeInTheDocument();
    expect(screen.getByText('test')).toBeInTheDocument();
  });

  it('renders "No CI checks" when ciChecks is empty', () => {
    render(<StatusSidebar pr={basePR} ciChecks={[]} />);
    expect(screen.getByText(/no ci checks/i)).toBeInTheDocument();
  });

  it('renders changes_requested status', () => {
    const pr = { ...basePR, review_status: 'changes_requested', approved_by: null, changes_requested_by: 'carol' };
    render(<StatusSidebar pr={pr} ciChecks={[]} />);
    expect(screen.getByText(/changes requested/i)).toBeInTheDocument();
    expect(screen.getByText(/carol/)).toBeInTheDocument();
  });

  it('renders comment count', () => {
    render(<StatusSidebar pr={basePR} ciChecks={[]} />);
    expect(screen.getByText(/3 comments/i)).toBeInTheDocument();
  });

  it('renders CI check link when url is provided', () => {
    const checks: CICheck[] = [{ name: 'ci-build', state: 'success', url: 'https://ci.example.com/1' }];
    render(<StatusSidebar pr={basePR} ciChecks={checks} />);
    const link = screen.getByRole('link', { name: 'ci-build' });
    expect(link).toHaveAttribute('href', 'https://ci.example.com/1');
  });
});
