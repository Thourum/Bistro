import { describe, expect, it } from 'vitest';
import type { Comment } from '../../src/renderer/types';
import {
  aggregateCounts,
  filterBySeverity,
  humanizeSource,
  isAutoCollapsed,
  sortBySeverity,
} from '../../src/renderer/utils/classification';

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

describe('sortBySeverity', () => {
  it('sorts comments by severity with unclassified comments last', () => {
    // Arrange
    const comments = [
      buildComment({ id: 'info', severity: 'info' }),
      buildComment({ id: 'unclassified' }),
      buildComment({ id: 'critical', severity: 'critical' }),
      buildComment({ id: 'nit', severity: 'nit' }),
      buildComment({ id: 'warning', severity: 'warning' }),
      buildComment({ id: 'suggestion', severity: 'suggestion' }),
    ];

    // Act
    const sorted = sortBySeverity(comments);

    // Assert
    expect(sorted.map((comment) => comment.id)).toEqual([
      'critical',
      'warning',
      'suggestion',
      'nit',
      'info',
      'unclassified',
    ]);
    expect(comments.map((comment) => comment.id)).toEqual([
      'info',
      'unclassified',
      'critical',
      'nit',
      'warning',
      'suggestion',
    ]);
  });

  it('sorts comments by created_at ascending within the same severity rank', () => {
    // Arrange
    const comments = [
      buildComment({
        id: 'newer',
        severity: 'warning',
        created_at: '2026-07-12T12:00:00Z',
      }),
      buildComment({
        id: 'critical',
        severity: 'critical',
        created_at: '2026-07-12T11:00:00Z',
      }),
      buildComment({
        id: 'older',
        severity: 'warning',
        created_at: '2026-07-12T09:00:00Z',
      }),
    ];

    // Act
    const sorted = sortBySeverity(comments);

    // Assert
    expect(sorted.map((comment) => comment.id)).toEqual([
      'critical',
      'older',
      'newer',
    ]);
  });
});

describe('isAutoCollapsed', () => {
  it('returns true for nit and info comments only', () => {
    // Arrange
    const comments = [
      buildComment({ id: 'critical', severity: 'critical' }),
      buildComment({ id: 'warning', severity: 'warning' }),
      buildComment({ id: 'suggestion', severity: 'suggestion' }),
      buildComment({ id: 'nit', severity: 'nit' }),
      buildComment({ id: 'info', severity: 'info' }),
      buildComment({ id: 'unclassified' }),
    ];

    // Act
    const collapsedById = Object.fromEntries(
      comments.map((comment) => [comment.id, isAutoCollapsed(comment)]),
    );

    // Assert
    expect(collapsedById).toEqual({
      critical: false,
      warning: false,
      suggestion: false,
      nit: true,
      info: true,
      unclassified: false,
    });
  });
});

describe('aggregateCounts', () => {
  it('counts human authors, severities, and known vendors with unknown vendors bucketed as other', () => {
    // Arrange
    const comments = [
      buildComment({ id: 'human', author_type: 'human', severity: 'critical' }),
      buildComment({
        id: 'coderabbit',
        author_type: 'bot',
        bot_vendor: 'coderabbit',
        severity: 'warning',
      }),
      buildComment({
        id: 'greptile',
        author_type: 'bot',
        bot_vendor: 'greptile',
        severity: 'warning',
      }),
      buildComment({
        id: 'unknown-vendor',
        author_type: 'bot',
        bot_vendor: 'reviewpad',
        severity: 'nit',
      }),
      buildComment({
        id: 'unclassified',
        author_type: 'unknown',
      }),
    ];

    // Act
    const counts = aggregateCounts(comments);

    // Assert
    expect(counts).toEqual({
      human: 1,
      bySeverity: {
        critical: 1,
        warning: 2,
        suggestion: 0,
        nit: 1,
        info: 0,
        unclassified: 1,
      },
      byVendor: {
        coderabbit: 1,
        greptile: 1,
        other: 1,
      },
    });
  });
});

describe('humanizeSource', () => {
  it('humanizes deterministic rule sources', () => {
    // Arrange
    const source = 'rule:coderabbit.yml:emoji_severity';

    // Act
    const label = humanizeSource(source);

    // Assert
    expect(label).toBe('Rule: coderabbit.yml / emoji_severity');
  });

  it('humanizes BYOK AI sources', () => {
    // Arrange
    const source = 'byok:anthropic:claude-haiku-4-5';

    // Act
    const label = humanizeSource(source);

    // Assert
    expect(label).toBe('AI: anthropic (claude-haiku-4-5)');
  });

  it('humanizes unclassified and absent sources', () => {
    // Arrange
    const sources = ['unclassified', undefined];

    // Act
    const labels = sources.map((source) => humanizeSource(source));

    // Assert
    expect(labels).toEqual(['Not classified', 'Not classified']);
  });
});

describe('filterBySeverity', () => {
  it('returns comments at or above the minimum severity', () => {
    // Arrange
    const comments = [
      buildComment({ id: 'critical', severity: 'critical' }),
      buildComment({ id: 'warning', severity: 'warning' }),
      buildComment({ id: 'suggestion', severity: 'suggestion' }),
      buildComment({ id: 'nit', severity: 'nit' }),
      buildComment({ id: 'info', severity: 'info' }),
      buildComment({ id: 'unclassified' }),
    ];

    // Act
    const filtered = filterBySeverity(comments, { minSeverity: 'suggestion' });

    // Assert
    expect(filtered.map((comment) => comment.id)).toEqual([
      'critical',
      'warning',
      'suggestion',
    ]);
  });

  it('returns only human comments when humanOnly is enabled', () => {
    // Arrange
    const comments = [
      buildComment({
        id: 'human-warning',
        author_type: 'human',
        severity: 'warning',
      }),
      buildComment({
        id: 'bot-critical',
        author_type: 'bot',
        severity: 'critical',
      }),
      buildComment({ id: 'human-nit', author_type: 'human', severity: 'nit' }),
    ];

    // Act
    const filtered = filterBySeverity(comments, {
      minSeverity: 'warning',
      humanOnly: true,
    });

    // Assert
    expect(filtered.map((comment) => comment.id)).toEqual(['human-warning']);
  });
});
