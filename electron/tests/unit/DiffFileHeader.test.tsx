// @vitest-environment jsdom
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
expect.extend(matchers);
import { DiffFileHeader } from '../../src/renderer/components/DiffFileHeader';
import type { DiffSection } from '../../src/renderer/utils/diff-parser';

const mockSection: DiffSection = {
  oldFile: 'a/src/auth.ts',
  newFile: 'b/src/auth.ts',
  hunks: [
    {
      header: '@@ -1,3 +1,4 @@',
      lines: [
        { type: 'context', content: 'line 1', oldLineNumber: 1, newLineNumber: 1 },
        { type: 'removed', content: 'old line', oldLineNumber: 2 },
        { type: 'added', content: 'new line', newLineNumber: 2 },
        { type: 'added', content: 'extra line', newLineNumber: 3 },
      ],
    },
  ],
};

describe('DiffFileHeader', () => {
  it('renders the file name without a/ b/ prefix', () => {
    render(
      <DiffFileHeader
        section={mockSection}
        index={0}
        isSelected={false}
        isCollapsed={false}
        onToggleCollapse={vi.fn()}
        onClick={vi.fn()}
      />
    );
    expect(screen.getByText('src/auth.ts')).toBeInTheDocument();
  });

  it('shows additions count', () => {
    render(
      <DiffFileHeader
        section={mockSection}
        index={0}
        isSelected={false}
        isCollapsed={false}
        onToggleCollapse={vi.fn()}
        onClick={vi.fn()}
      />
    );
    expect(screen.getByText('+2')).toBeInTheDocument();
  });

  it('shows deletions count', () => {
    render(
      <DiffFileHeader
        section={mockSection}
        index={0}
        isSelected={false}
        isCollapsed={false}
        onToggleCollapse={vi.fn()}
        onClick={vi.fn()}
      />
    );
    expect(screen.getByText('-1')).toBeInTheDocument();
  });

  it('shows collapse toggle button with down arrow when expanded', () => {
    render(
      <DiffFileHeader
        section={mockSection}
        index={0}
        isSelected={false}
        isCollapsed={false}
        onToggleCollapse={vi.fn()}
        onClick={vi.fn()}
      />
    );
    expect(screen.getByLabelText('Collapse file')).toBeInTheDocument();
  });

  it('shows expand toggle button with right arrow when collapsed', () => {
    render(
      <DiffFileHeader
        section={mockSection}
        index={0}
        isSelected={false}
        isCollapsed={true}
        onToggleCollapse={vi.fn()}
        onClick={vi.fn()}
      />
    );
    expect(screen.getByLabelText('Expand file')).toBeInTheDocument();
  });

  it('calls onToggleCollapse when collapse button is clicked', () => {
    const onToggleCollapse = vi.fn();
    render(
      <DiffFileHeader
        section={mockSection}
        index={0}
        isSelected={false}
        isCollapsed={false}
        onToggleCollapse={onToggleCollapse}
        onClick={vi.fn()}
      />
    );
    fireEvent.click(screen.getByLabelText('Collapse file'));
    expect(onToggleCollapse).toHaveBeenCalledOnce();
  });

  it('calls onClick when header row is clicked', () => {
    const onClick = vi.fn();
    render(
      <DiffFileHeader
        section={mockSection}
        index={0}
        isSelected={false}
        isCollapsed={false}
        onToggleCollapse={vi.fn()}
        onClick={onClick}
      />
    );
    fireEvent.click(screen.getByTestId('diff-file-header-0'));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it('applies selected styling when isSelected is true', () => {
    render(
      <DiffFileHeader
        section={mockSection}
        index={0}
        isSelected={true}
        isCollapsed={false}
        onToggleCollapse={vi.fn()}
        onClick={vi.fn()}
      />
    );
    const header = screen.getByTestId('diff-file-header-0');
    expect(header.className).toContain('bg-primary/10');
  });

  it('uses oldFile when newFile is empty', () => {
    const sectionNoNew: DiffSection = { ...mockSection, newFile: '' };
    render(
      <DiffFileHeader
        section={sectionNoNew}
        index={1}
        isSelected={false}
        isCollapsed={false}
        onToggleCollapse={vi.fn()}
        onClick={vi.fn()}
      />
    );
    expect(screen.getByText('src/auth.ts')).toBeInTheDocument();
  });
});
