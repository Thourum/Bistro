// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import { expect } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
import { DiffViewer } from '../../src/renderer/components/DiffViewer';

expect.extend(matchers);

const sampleDiff = `--- a/src/auth.ts
+++ b/src/auth.ts
@@ -10,7 +10,8 @@
 export function login(username: string) {
   const user = db.find(username);
-  if (!user) throw new Error('Not found');
+  if (!user) {
+    throw new Error('User not found');
+  }
   return user;
 }`;

describe('DiffViewer', () => {
  it('renders diff content', () => {
    render(<DiffViewer diff={sampleDiff} />);
    const matches = screen.getAllByText(/auth\.ts/);
    expect(matches.length).toBeGreaterThan(0);
  });

  it('displays file paths', () => {
    render(<DiffViewer diff={sampleDiff} />);
    const matches = screen.getAllByText(/src\/auth\.ts/);
    expect(matches.length).toBeGreaterThan(0);
  });

  it('renders context lines', () => {
    render(<DiffViewer diff={sampleDiff} />);
    expect(screen.getByText(/export function login/)).toBeInTheDocument();
  });

  it('renders added lines with green styling', () => {
    render(<DiffViewer diff={sampleDiff} />);
    const container = screen.getByText(/User not found/);
    expect(container.closest('.diff-added')).toBeInTheDocument();
  });

  it('renders removed lines with red styling', () => {
    render(<DiffViewer diff={sampleDiff} />);
    const container = screen.getByText(/Not found/);
    expect(container.closest('.diff-removed')).toBeInTheDocument();
  });

  it('displays hunk headers', () => {
    render(<DiffViewer diff={sampleDiff} />);
    expect(screen.getByText(/@@ -10,7 \+10,8 @@/)).toBeInTheDocument();
  });

  it('renders scrollable container', () => {
    const { container } = render(<DiffViewer diff={sampleDiff} />);
    const scrollArea = container.querySelector('[class*="overflow"]');
    expect(scrollArea).toBeInTheDocument();
  });

  it('handles empty diff gracefully', () => {
    render(<DiffViewer diff="" />);
    expect(screen.getByText(/No changes/i)).toBeInTheDocument();
  });
});
