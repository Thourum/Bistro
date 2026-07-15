import type { ExpertiseExpert, PR } from '../types';

export function touchesYouCount(prs: readonly PR[]): number {
  return prs.filter((pr) => pr.touches_you).length;
}

export function formatWhy(expert: ExpertiseExpert): string {
  const author = expert.author_login ?? expert.author_email;
  const originalAuthor = expert.is_original ? ', original author' : '';
  const roundedScore = Math.round(expert.score);

  return `${author}: ${expert.surviving_lines}/${expert.total_lines} surviving lines${originalAuthor} → ${roundedScore}`;
}
