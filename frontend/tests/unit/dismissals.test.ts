import { describe, expect, it } from 'vitest';
import type { Comment } from '../../src/types';
import {
  type DismissBatch,
  dismissReport,
  popBatch,
  pushBatch,
  selectBotIds,
  selectNitIds,
  visibleBotVendors,
} from '../../src/utils/dismissals';

function buildComment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 'comment-1',
    pr_id: 'pr-1',
    author: 'reviewer',
    body: 'Looks good.',
    created_at: '2026-07-12T10:00:00Z',
    updated_at: '2026-07-12T10:00:00Z',
    ...overrides,
  };
}

describe('selectNitIds', () => {
  it('selects nit and info comments that are not yet dismissed', () => {
    // Arrange
    const comments = [
      buildComment({ id: 'nit', severity: 'nit' }),
      buildComment({ id: 'info', severity: 'info' }),
      buildComment({ id: 'warning', severity: 'warning' }),
      buildComment({ id: 'dismissed-nit', severity: 'nit', dismissed: true }),
      buildComment({ id: 'unclassified' }),
    ];

    // Act / Assert
    expect(selectNitIds(comments)).toEqual(['nit', 'info']);
  });
});

describe('selectBotIds', () => {
  it('selects undismissed bot comments, optionally filtered by vendor', () => {
    // Arrange
    const comments = [
      buildComment({
        id: 'rabbit',
        author_type: 'bot',
        bot_vendor: 'coderabbit',
      }),
      buildComment({
        id: 'greptile',
        author_type: 'bot',
        bot_vendor: 'greptile',
      }),
      buildComment({ id: 'human', author_type: 'human' }),
      buildComment({
        id: 'gone',
        author_type: 'bot',
        bot_vendor: 'coderabbit',
        dismissed: true,
      }),
    ];

    // Act / Assert
    expect(selectBotIds(comments)).toEqual(['rabbit', 'greptile']);
    expect(selectBotIds(comments, 'coderabbit')).toEqual(['rabbit']);
  });
});

describe('visibleBotVendors', () => {
  it('lists distinct vendors of visible bot comments', () => {
    // Arrange
    const comments = [
      buildComment({ id: 'a', author_type: 'bot', bot_vendor: 'coderabbit' }),
      buildComment({ id: 'b', author_type: 'bot', bot_vendor: 'coderabbit' }),
      buildComment({ id: 'c', author_type: 'bot', bot_vendor: 'greptile' }),
      buildComment({
        id: 'd',
        author_type: 'bot',
        bot_vendor: 'greptile',
        dismissed: true,
      }),
      buildComment({ id: 'e', author_type: 'human' }),
    ];

    // Act / Assert
    expect(visibleBotVendors(comments)).toEqual(['coderabbit', 'greptile']);
  });
});

describe('undo stack', () => {
  const batchA: DismissBatch = {
    batch_id: 'a',
    dismissed_ids: ['c1'],
    resolved_thread_ids: [],
  };
  const batchB: DismissBatch = {
    batch_id: 'b',
    dismissed_ids: ['c2', 'c3'],
    resolved_thread_ids: ['t1'],
  };

  it('pushes immutably and pops LIFO', () => {
    // Arrange
    const empty: DismissBatch[] = [];

    // Act
    const one = pushBatch(empty, batchA);
    const two = pushBatch(one, batchB);
    const { batch, rest } = popBatch(two);

    // Assert
    expect(empty).toEqual([]); // original untouched
    expect(one).toEqual([batchA]);
    expect(batch).toBe(batchB);
    expect(rest).toEqual([batchA]);
  });

  it('pop on an empty stack is a no-op', () => {
    // Act
    const { batch, rest } = popBatch([]);

    // Assert
    expect(batch).toBeUndefined();
    expect(rest).toEqual([]);
  });
});

describe('dismissReport', () => {
  it('reports the batch size and enqueued resolves', () => {
    expect(
      dismissReport({
        batch_id: 'x',
        dismissed_ids: Array.from({ length: 12 }, (_, i) => `c${i}`),
        resolved_thread_ids: ['t1', 't2', 't3'],
      }),
    ).toBe('Dismissed 12 · 3 resolved on GitHub');
  });

  it('omits the resolve suffix for local-only batches', () => {
    expect(
      dismissReport({
        batch_id: 'x',
        dismissed_ids: ['c1'],
        resolved_thread_ids: [],
      }),
    ).toBe('Dismissed 1');
  });
});
