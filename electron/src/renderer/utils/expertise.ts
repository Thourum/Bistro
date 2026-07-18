import type { ExpertiseDataPayload, ExpertiseExpert, PR } from '../types';

export function touchesYouCount(prs: readonly PR[]): number {
  return prs.filter((pr) => pr.touches_you).length;
}

export function focusedExpertisePR(
  prs: readonly PR[],
  routedPrId: string | undefined,
  selectedPRId: string | undefined,
): PR | undefined {
  const prId = routedPrId ?? selectedPRId;
  return prId === undefined ? undefined : prs.find((pr) => pr.id === prId);
}

export function repoIdForPR(pr: PR): string {
  return `${pr.provider}:${pr.owner}/${pr.repo}`;
}

export function formatWhy(expert: ExpertiseExpert): string {
  const author = expert.author_login ?? expert.author_email;
  const originalAuthor = expert.is_original ? ', original author' : '';
  const roundedScore = Math.round(expert.score);

  return `${author}: ${expert.surviving_lines}/${expert.total_lines} surviving lines${originalAuthor} → ${roundedScore}`;
}

export function expertisePayloadForPR(
  payload: ExpertiseDataPayload,
  prId: string,
): ExpertiseDataPayload {
  return {
    ...payload,
    pr_id: payload.pr_id ?? prId,
    files: payload.files ?? [],
  };
}

export function isExpertisePayloadForPR(
  payload: ExpertiseDataPayload | null | undefined,
  prId: string | undefined,
): payload is ExpertiseDataPayload {
  return (
    payload !== null &&
    payload !== undefined &&
    prId !== undefined &&
    payload.pr_id === prId
  );
}
