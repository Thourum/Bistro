// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { CommentThread } from '../../src/renderer/components/CommentThread';
import type { Comment } from '../../src/renderer/types';

expect.extend(matchers);

const mockComment: Comment = {
  id: 'c1',
  pr_id: 'pr1',
  author: 'alice',
  body: 'This looks good',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  line: 10,
  file: 'src/auth.ts',
};

const mockReply: Comment = {
  id: 'c2',
  pr_id: 'pr1',
  author: 'bob',
  body: 'Agreed!',
  created_at: '2026-01-02T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
  is_reply_to: 'c1',
};

describe('CommentThread', () => {
  it('renders comment author and body', () => {
    render(<CommentThread comments={[mockComment]} />);
    expect(screen.getByText(/@alice/)).toBeInTheDocument();
    expect(screen.getByText(/This looks good/)).toBeInTheDocument();
  });

  it('shows comment count for single comment', () => {
    render(<CommentThread comments={[mockComment]} />);
    expect(screen.getByText(/1 comment/)).toBeInTheDocument();
  });

  it('shows comment count for multiple comments', () => {
    render(<CommentThread comments={[mockComment, mockReply]} />);
    expect(screen.getByText(/2 comments/)).toBeInTheDocument();
  });

  it('renders nothing for empty comments', () => {
    const { container } = render(<CommentThread comments={[]} />);
    expect(container.innerHTML).toBe('');
  });

  it('shows Reply... button when onReply provided', () => {
    render(<CommentThread comments={[mockComment]} onReply={() => {}} />);
    expect(screen.getByText(/Reply\.\.\./)).toBeInTheDocument();
  });

  it('does not show Reply... button when onReply not provided', () => {
    render(<CommentThread comments={[mockComment]} />);
    expect(screen.queryByText(/Reply\.\.\./)).not.toBeInTheDocument();
  });

  it('shows edit and delete buttons for comment author', () => {
    render(
      <CommentThread
        comments={[mockComment]}
        currentUser="alice"
        onDelete={() => {}}
      />,
    );
    expect(screen.getByText('edit')).toBeInTheDocument();
    expect(screen.getByText('delete')).toBeInTheDocument();
  });

  it('hides edit/delete for non-author', () => {
    render(<CommentThread comments={[mockComment]} currentUser="bob" />);
    expect(screen.queryByText('edit')).not.toBeInTheDocument();
    expect(screen.queryByText('delete')).not.toBeInTheDocument();
  });

  it('can collapse thread by clicking header', () => {
    render(<CommentThread comments={[mockComment]} />);
    // Body initially visible
    expect(screen.getByText(/This looks good/)).toBeInTheDocument();
    // Click the collapse button (the header row)
    fireEvent.click(screen.getByRole('button', { name: /▼.*comment/i }));
    // Body no longer visible
    expect(screen.queryByText(/This looks good/)).not.toBeInTheDocument();
  });

  it('can expand thread after collapsing', () => {
    render(<CommentThread comments={[mockComment]} />);
    const header = screen.getByRole('button', { name: /▼.*comment/i });
    fireEvent.click(header); // collapse
    fireEvent.click(screen.getByRole('button', { name: /▶.*comment/i })); // expand
    expect(screen.getByText(/This looks good/)).toBeInTheDocument();
  });

  it('shows reply input when Reply... is clicked', () => {
    render(<CommentThread comments={[mockComment]} onReply={() => {}} />);
    fireEvent.click(screen.getByText(/Reply\.\.\./));
    expect(screen.getByPlaceholderText(/Write a reply/)).toBeInTheDocument();
  });

  it('calls onReply with text when Reply button is clicked', () => {
    const onReply = vi.fn();
    render(<CommentThread comments={[mockComment]} onReply={onReply} />);
    fireEvent.click(screen.getByText(/Reply\.\.\./));
    fireEvent.change(screen.getByPlaceholderText(/Write a reply/), {
      target: { value: 'LGTM!' },
    });
    fireEvent.click(screen.getByText('Reply'));
    expect(onReply).toHaveBeenCalledWith('LGTM!');
  });

  it('calls onDelete when delete button clicked', () => {
    const onDelete = vi.fn();
    render(
      <CommentThread
        comments={[mockComment]}
        currentUser="alice"
        onDelete={onDelete}
      />,
    );
    fireEvent.click(screen.getByText('delete'));
    expect(onDelete).toHaveBeenCalledWith('c1');
  });

  it('shows edit textarea when edit button clicked', () => {
    render(
      <CommentThread
        comments={[mockComment]}
        currentUser="alice"
        onEdit={() => {}}
      />,
    );
    fireEvent.click(screen.getByText('edit'));
    expect(screen.getByDisplayValue(/This looks good/)).toBeInTheDocument();
  });
});
