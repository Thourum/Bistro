// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { CommentInput } from '../../src/renderer/components/CommentInput';

expect.extend(matchers);

describe('CommentInput', () => {
  it('renders with line number label', () => {
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={42}
        onSubmit={() => {}}
        onCancel={() => {}}
      />,
    );
    expect(screen.getByText(/Comment on line 42/)).toBeInTheDocument();
  });

  it('renders textarea with placeholder', () => {
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={() => {}}
        onCancel={() => {}}
      />,
    );
    expect(screen.getByPlaceholderText(/Add comment/)).toBeInTheDocument();
  });

  it('renders Send and Cancel buttons', () => {
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={() => {}}
        onCancel={() => {}}
      />,
    );
    expect(screen.getByText('Send')).toBeInTheDocument();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
  });

  it('calls onSubmit with trimmed text when Send is clicked', () => {
    const onSubmit = vi.fn();
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={onSubmit}
        onCancel={() => {}}
      />,
    );
    fireEvent.change(screen.getByPlaceholderText(/Add comment/), {
      target: { value: '  Great work!  ' },
    });
    fireEvent.click(screen.getByText('Send'));
    expect(onSubmit).toHaveBeenCalledWith('Great work!');
  });

  it('disables Send button when text is empty', () => {
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={() => {}}
        onCancel={() => {}}
      />,
    );
    expect(screen.getByText('Send')).toBeDisabled();
  });

  it('enables Send button when text is entered', () => {
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={() => {}}
        onCancel={() => {}}
      />,
    );
    fireEvent.change(screen.getByPlaceholderText(/Add comment/), {
      target: { value: 'hello' },
    });
    expect(screen.getByText('Send')).not.toBeDisabled();
  });

  it('calls onCancel when Cancel is clicked', () => {
    const onCancel = vi.fn();
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={() => {}}
        onCancel={onCancel}
      />,
    );
    fireEvent.click(screen.getByText('Cancel'));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it('calls onSubmit on Ctrl+Enter', () => {
    const onSubmit = vi.fn();
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={onSubmit}
        onCancel={() => {}}
      />,
    );
    const textarea = screen.getByPlaceholderText(/Add comment/);
    fireEvent.change(textarea, { target: { value: 'ctrl+enter test' } });
    fireEvent.keyDown(textarea, { key: 'Enter', ctrlKey: true });
    expect(onSubmit).toHaveBeenCalledWith('ctrl+enter test');
  });

  it('calls onSubmit on Cmd+Enter', () => {
    const onSubmit = vi.fn();
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={onSubmit}
        onCancel={() => {}}
      />,
    );
    const textarea = screen.getByPlaceholderText(/Add comment/);
    fireEvent.change(textarea, { target: { value: 'cmd+enter test' } });
    fireEvent.keyDown(textarea, { key: 'Enter', metaKey: true });
    expect(onSubmit).toHaveBeenCalledWith('cmd+enter test');
  });

  it('calls onCancel on Escape key', () => {
    const onCancel = vi.fn();
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={() => {}}
        onCancel={onCancel}
      />,
    );
    const textarea = screen.getByPlaceholderText(/Add comment/);
    fireEvent.keyDown(textarea, { key: 'Escape' });
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it('does not call onSubmit when text is only whitespace', () => {
    const onSubmit = vi.fn();
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={onSubmit}
        onCancel={() => {}}
      />,
    );
    fireEvent.change(screen.getByPlaceholderText(/Add comment/), {
      target: { value: '   ' },
    });
    fireEvent.click(screen.getByText('Send'));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('auto-focuses the textarea', () => {
    render(
      <CommentInput
        fileIndex={0}
        lineNumber={1}
        onSubmit={() => {}}
        onCancel={() => {}}
      />,
    );
    const textarea = screen.getByPlaceholderText(/Add comment/);
    expect(document.activeElement).toBe(textarea);
  });

  it('renders correct testid with fileIndex and lineNumber', () => {
    render(
      <CommentInput
        fileIndex={2}
        lineNumber={15}
        onSubmit={() => {}}
        onCancel={() => {}}
      />,
    );
    expect(
      screen.getByTestId('comment-input-file-2-line-15'),
    ).toBeInTheDocument();
  });
});
