import { Repos } from '@bindings/services';
import type { Fault, ResultAsync } from '@thourum/fault';
import { addToast } from '../components/ToastContainer';
import { appStore } from '../store';
import type { RepoInfo } from '../types';
import { call, callWrite } from './engine/call';

const REPO_LIST_TIMEOUT_MS = 35_000;

/** Loads repositories and owns discovery loading/error UI. */
export function loadRepos(
  options: Readonly<{ cacheOnly?: boolean }> = {},
): ResultAsync<RepoInfo[], Fault> {
  appStore.setState((state) => ({ ...state, reposLoading: true }));
  return call(() => Repos.List({ cache_only: options.cacheOnly ?? false }), {
    timeoutMs: REPO_LIST_TIMEOUT_MS,
  })
    .map((response) => {
      const repos = response.repos ?? [];
      appStore.setState((state) => ({ ...state, repos, reposLoading: false }));
      return repos;
    })
    .orInspect((fault) => {
      appStore.setState((state) => ({ ...state, reposLoading: false }));
      if (appStore.state.onboardingSync !== null) return;
      addToast({
        type: 'error',
        message: `Failed to load repos: ${fault.message}`,
        dismissible: true,
      });
    });
}

/** Persists a watched state and applies the engine's acknowledged value. */
export function setRepoWatched(
  repoId: string,
  watched: boolean,
): ResultAsync<void, Fault> {
  return callWrite(() => Repos.SetWatched({ repo_id: repoId, watched })).map(
    (response) => {
      appStore.setState((state) => ({
        ...state,
        repos: state.repos.map((repo) =>
          repo.id === response.repo_id
            ? { ...repo, watched: response.watched }
            : repo,
        ),
      }));
      return undefined;
    },
  );
}
