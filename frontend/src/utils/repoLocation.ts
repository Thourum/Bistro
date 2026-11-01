import type { RepoInfo } from '../types';

const HEALTHY_LOCATION = 'ok';

/** Missing location status is not a reported degradation. */
export function isRepoLocationDegraded(
  repo: Readonly<RepoInfo> | null,
): boolean {
  return (
    repo !== null &&
    repo.location_status !== null &&
    repo.location_status !== undefined &&
    repo.location_status !== HEALTHY_LOCATION
  );
}
