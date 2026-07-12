import type { Comment, CommentSeverity } from '../types';

export const SEVERITY_RANK: Record<CommentSeverity, number> = {
  critical: 5,
  warning: 4,
  suggestion: 3,
  nit: 2,
  info: 1,
};

const UNCLASSIFIED_RANK = 0;
const AUTO_COLLAPSED_SEVERITIES = new Set<CommentSeverity>(['nit', 'info']);
const KNOWN_BOT_VENDORS = new Set(['coderabbit', 'greptile']);

export interface ClassificationCounts {
  human: number;
  bySeverity: Record<CommentSeverity | 'unclassified', number>;
  byVendor: Record<string, number>;
}

export interface SeverityFilterOptions {
  minSeverity?: CommentSeverity;
  humanOnly?: boolean;
}

function severityRank(severity: CommentSeverity | undefined): number {
  if (!severity) return UNCLASSIFIED_RANK;
  return SEVERITY_RANK[severity];
}

function emptySeverityCounts(): Record<
  CommentSeverity | 'unclassified',
  number
> {
  return {
    critical: 0,
    warning: 0,
    suggestion: 0,
    nit: 0,
    info: 0,
    unclassified: 0,
  };
}

function incrementCount(
  counts: Record<string, number>,
  key: string,
): Record<string, number> {
  return {
    ...counts,
    [key]: (counts[key] ?? 0) + 1,
  };
}

export function sortBySeverity(comments: readonly Comment[]): Comment[] {
  return [...comments].sort((firstComment, secondComment) => {
    const rankDifference =
      severityRank(secondComment.severity) -
      severityRank(firstComment.severity);
    if (rankDifference !== 0) return rankDifference;

    return firstComment.created_at.localeCompare(secondComment.created_at);
  });
}

export function isAutoCollapsed(comment: Comment): boolean {
  return comment.severity
    ? AUTO_COLLAPSED_SEVERITIES.has(comment.severity)
    : false;
}

export function aggregateCounts(
  comments: readonly Comment[],
): ClassificationCounts {
  return comments.reduce<ClassificationCounts>(
    (counts, comment) => {
      const severityKey = comment.severity ?? 'unclassified';
      const isKnownBotVendor =
        comment.author_type === 'bot' && comment.bot_vendor;
      const vendorKey = isKnownBotVendor
        ? KNOWN_BOT_VENDORS.has(comment.bot_vendor as string)
          ? (comment.bot_vendor as string)
          : 'other'
        : undefined;

      return {
        human:
          comment.author_type === 'human' ? counts.human + 1 : counts.human,
        bySeverity: incrementCount(counts.bySeverity, severityKey) as Record<
          CommentSeverity | 'unclassified',
          number
        >,
        byVendor: vendorKey
          ? incrementCount(counts.byVendor, vendorKey)
          : counts.byVendor,
      };
    },
    {
      human: 0,
      bySeverity: emptySeverityCounts(),
      byVendor: {},
    },
  );
}

export function humanizeSource(source: string | undefined): string {
  if (!source || source === 'unclassified') return 'Not classified';

  const [kind, firstPart, ...remainingParts] = source.split(':');
  const detail = remainingParts.join(':');

  if (kind === 'rule' && firstPart && detail) {
    return `Rule: ${firstPart} / ${detail}`;
  }

  if (kind === 'byok' && firstPart && detail) {
    return `AI: ${firstPart} (${detail})`;
  }

  return source;
}

export function filterBySeverity(
  comments: readonly Comment[],
  options: SeverityFilterOptions = {},
): Comment[] {
  const minimumRank = options.minSeverity
    ? SEVERITY_RANK[options.minSeverity]
    : UNCLASSIFIED_RANK;

  return comments.filter((comment) => {
    if (options.humanOnly && comment.author_type !== 'human') return false;
    return severityRank(comment.severity) >= minimumRank;
  });
}

/**
 * One renderable unit after dedup grouping: either a single comment or a
 * merged multi-bot finding. A group is a VIEW, never a dismissal unit — it is
 * built from the comments that survived dismiss/filter, so members recompute
 * and a fully-hidden group disappears (spec 3.6).
 */
export interface MergedFinding {
  /** Anchor member (first visible, in input order). */
  anchor: Comment;
  /** All visible members, anchor first. Length 1 = plain comment. */
  members: Comment[];
  /** Max severity among visible members; undefined when none classified. */
  severity: CommentSeverity | undefined;
  /** Distinct vendor labels of visible members, e.g. ["coderabbit","greptile"]. */
  vendors: string[];
}

/**
 * Group VISIBLE comments by dedup_group_id, preserving input order.
 * Call AFTER dismiss/filter have been applied.
 */
export function groupVisibleFindings(
  comments: readonly Comment[],
): MergedFinding[] {
  const byGroup = new Map<string, Comment[]>();
  for (const comment of comments) {
    if (!comment.dedup_group_id) continue;
    byGroup.set(comment.dedup_group_id, [
      ...(byGroup.get(comment.dedup_group_id) ?? []),
      comment,
    ]);
  }

  const emitted = new Set<string>();
  const findings: MergedFinding[] = [];
  for (const comment of comments) {
    const groupId = comment.dedup_group_id;
    const members =
      groupId && (byGroup.get(groupId)?.length ?? 0) >= 2
        ? (byGroup.get(groupId) as Comment[])
        : [comment];

    if (groupId && members.length >= 2) {
      if (emitted.has(groupId)) continue;
      emitted.add(groupId);
    }

    const severity = members.reduce<CommentSeverity | undefined>(
      (max, member) =>
        severityRank(member.severity) > severityRank(max)
          ? member.severity
          : max,
      undefined,
    );
    const vendors = [
      ...new Set(
        members
          .map((member) => member.bot_vendor)
          .filter((vendor): vendor is string => Boolean(vendor)),
      ),
    ];
    findings.push({
      anchor: members[0] as Comment,
      members,
      severity,
      vendors,
    });
  }
  return findings;
}
