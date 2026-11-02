import { Link, useParams, useSearch } from '@tanstack/react-router';
import type React from 'react';
import { useCallback, useLayoutEffect, useState } from 'react';
import { DiffView } from '../components/diff/DiffView';
import { StatusSquare } from '../components/StatusSquare';
import { usePRList } from '../hooks/usePRList';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { relativeTime } from '../utils/relativeTime';
import { prBrowserUrl } from '../utils/triage';
import { useCommitData } from './useCommitData';

const SHORT_SHA_LENGTH = 7;

export function CommitPage(): React.ReactElement {
  const { prId, sha } = useParams({ from: '/authed/prs/$prId/commits/$sha' });
  const { file } = useSearch({ from: '/authed/prs/$prId/commits/$sha' });
  const {
    commit,
    position,
    diff,
    loading,
    error,
    notInPr,
    unavailableReason,
    toggleViewed,
  } = useCommitData(prId, sha);
  const theme = useStore(settingsStore, (state) => state.appearance.theme);
  const diffStyle = useStore(settingsStore, (state) => state.diff.style);
  const { data: allPRs = [] } = usePRList('all');
  const pr = allPRs.find((entry) => entry.id === prId);
  const [findQuery, setFindQuery] = useState('');

  useLayoutEffect(() => {
    void prId;
    void sha;
    appStore.setState((state) => ({
      ...state,
      diffFileKeys: [],
      diffCollapseOverrides: [],
      hideWhitespaceHunks: false,
    }));
  }, [prId, sha]);
  const handleFilesResolved = useCallback((canonicalKeys: string[]): void => {
    appStore.setState((state) => ({ ...state, diffFileKeys: canonicalKeys }));
  }, []);

  return (
    <main className="flex-1 flex flex-col overflow-hidden bg-surface text-on-surface">
      <div className="shrink-0 h-10 flex items-center justify-between gap-4 px-4 border-b border-outline-variant/30 bg-surface-container-lowest font-mono text-[11px]">
        <Link
          to="/prs/$prId/commits"
          params={{ prId }}
          className="uppercase tracking-widest text-outline hover:text-on-surface"
        >
          Back to commits
        </Link>
        <span className="text-primary">{sha.slice(0, SHORT_SHA_LENGTH)}</span>
        {position !== null && (
          <span>
            commit {position.index + 1}/{position.total}
          </span>
        )}
        <input
          type="search"
          aria-label="Find files"
          placeholder="Find files…"
          value={findQuery}
          onChange={(event) => setFindQuery(event.target.value)}
          className="px-3 py-1 w-64 bg-surface border border-outline-variant/40 placeholder:text-outline-variant"
        />
      </div>
      {commit !== null && (
        <header className="shrink-0 px-4 py-3 border-b border-outline-variant/30 font-mono text-xs">
          <div className="flex items-center justify-between gap-4">
            <h1 className="font-bold">{commit.subject}</h1>
            <button
              type="button"
              onClick={() => void toggleViewed()}
              aria-label={
                commit.viewed ? 'Mark commit unviewed' : 'Mark commit viewed'
              }
              className="flex items-center gap-2 text-outline"
            >
              <StatusSquare
                status={commit.viewed ? 'success' : 'neutral'}
                label={commit.viewed ? 'Viewed' : 'Unviewed'}
              />
              {commit.viewed ? 'Viewed' : 'Unviewed'}
            </button>
          </div>
          <div className="mt-1 text-outline">
            {commit.author_name} ·{' '}
            <time dateTime={commit.authored_at}>
              {relativeTime(commit.authored_at)}
            </time>
          </div>
          {commit.body !== '' && (
            <pre className="mt-3 whitespace-pre-wrap break-words max-h-40 overflow-auto text-on-surface-variant">
              {commit.body}
            </pre>
          )}
        </header>
      )}
      <div className="flex-1 min-h-0">
        {notInPr ? (
          <CommitMessage>
            This commit is no longer part of the PR.{' '}
            <Link
              to="/prs/$prId/commits"
              params={{ prId }}
              className="text-primary underline"
            >
              Back to commits
            </Link>
          </CommitMessage>
        ) : loading ? (
          <CommitMessage>Loading commit…</CommitMessage>
        ) : error !== null ? (
          <CommitMessage>Could not load commit: {error}</CommitMessage>
        ) : unavailableReason !== null ? (
          <CommitMessage>{unavailableReason}</CommitMessage>
        ) : commit?.is_merge ? (
          <CommitMessage>
            Merge commit — its diff is not shown. Edits made while resolving
            conflicts live only in the merge, so they appear in the combined
            diff but in none of the listed commits. Review them there.{' '}
            <Link
              to="/prs/$prId/diff"
              params={{ prId }}
              search={{ file: '', side: 'right' }}
              className="text-primary underline"
            >
              Combined diff
            </Link>
          </CommitMessage>
        ) : (
          <DiffView
            key={`${prId}:${sha}`}
            diff={diff?.diff ?? ''}
            theme={theme}
            diffStyle={diffStyle}
            findQuery={findQuery}
            selectedFileKey={file}
            onFilesResolved={handleFilesResolved}
            syncFileSelection
            prBrowserUrl={pr === undefined ? null : prBrowserUrl(pr)}
            headSha={sha}
          />
        )}
      </div>
    </main>
  );
}

function CommitMessage({
  children,
}: {
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <div className="w-full h-full flex items-center justify-center px-4 font-mono text-xs text-outline">
      <p className="max-w-2xl">{children}</p>
    </div>
  );
}
