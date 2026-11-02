import { Link, useParams } from '@tanstack/react-router';
import type React from 'react';
import { useEffect, useRef } from 'react';
import { StatusSquare } from '../components/StatusSquare';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { relativeTime } from '../utils/relativeTime';
import { useCommitsData } from './useCommitsData';

const SHORT_SHA_LENGTH = 7;

export function CommitsPage(): React.ReactElement {
  const { prId } = useParams({ from: '/authed/prs/$prId/commits' });
  const { result, loading, error, toggleViewed } = useCommitsData(prId);
  const selectedIndex = useStore(
    appStore,
    (state) => state.selectedCommitIndex,
  );
  const selectedRow = useRef<HTMLDivElement>(null);
  const commits = result?.commits ?? [];
  const viewedCount = commits.filter((commit) => commit.viewed).length;
  useEffect(() => {
    void selectedIndex;
    selectedRow.current?.scrollIntoView({ block: 'nearest' });
  }, [selectedIndex]);

  return (
    <main className="flex-1 flex flex-col overflow-hidden bg-surface text-on-surface">
      <div className="shrink-0 h-10 flex items-center justify-between px-4 border-b border-outline-variant/30 bg-surface-container-lowest font-mono text-[11px]">
        <Link
          to="/prs/$prId"
          params={{ prId }}
          className="uppercase tracking-widest text-outline hover:text-on-surface"
        >
          Back to PR
        </Link>
        <h1 className="uppercase tracking-widest">Commits</h1>
        <span>
          {viewedCount} of {commits.length} viewed
        </span>
      </div>
      {result?.truncated && (
        <div className="px-4 py-2 border-b border-outline-variant/30 font-mono text-xs text-outline">
          Showing the first 250 commits;{' '}
          <Link
            to="/prs/$prId/diff"
            params={{ prId }}
            search={{ file: '', side: 'right' }}
            className="text-primary underline"
          >
            review the rest in the combined diff
          </Link>
        </div>
      )}
      {loading || error !== null || result?.available === false ? (
        <div
          className={`flex-1 flex items-center justify-center font-mono text-xs ${error === null ? 'text-outline' : 'text-error'}`}
        >
          {loading ? 'Loading commits…' : (error ?? result?.reason)}
        </div>
      ) : commits.length === 0 ? (
        <div className="flex-1 flex items-center justify-center font-mono text-xs text-outline">
          No commits
        </div>
      ) : (
        <section className="flex-1 overflow-auto" aria-label="PR commits">
          {commits.map((commit, index) => (
            <div
              key={commit.sha}
              ref={index === selectedIndex ? selectedRow : undefined}
              className={`flex items-center gap-3 px-4 py-3 border-b border-outline-variant/30 font-mono text-[11px] ${index === selectedIndex ? 'bg-primary/10 border-l-2 border-l-primary' : 'hover:bg-surface-container-low border-l-2 border-l-transparent'}`}
            >
              <button
                type="button"
                aria-label={`${commit.viewed ? 'Mark unviewed' : 'Mark viewed'}: ${commit.subject}`}
                onClick={() => void toggleViewed(commit.sha)}
              >
                <StatusSquare
                  status={commit.viewed ? 'success' : 'neutral'}
                  label={commit.viewed ? 'Viewed' : 'Unviewed'}
                />
              </button>
              <Link
                to="/prs/$prId/commits/$sha"
                params={{ prId, sha: commit.sha }}
                search={{ file: '', side: 'right' }}
                onClick={() =>
                  appStore.setState((state) => ({
                    ...state,
                    selectedCommitIndex: index,
                  }))
                }
                aria-current={index === selectedIndex ? 'true' : undefined}
                className="flex-1 min-w-0"
              >
                <div className="flex items-center gap-3">
                  <span className="text-primary">
                    {commit.sha.slice(0, SHORT_SHA_LENGTH)}
                  </span>
                  <span className="truncate text-on-surface">
                    {commit.subject}
                  </span>
                  {commit.is_merge && (
                    <span className="border border-outline-variant/40 px-2 py-0.5 text-outline uppercase">
                      merge
                    </span>
                  )}
                </div>
                <div className="flex gap-3 mt-1 text-outline">
                  <span>{commit.author_name}</span>
                  <time dateTime={commit.authored_at}>
                    {relativeTime(commit.authored_at)}
                  </time>
                  <span className="text-primary">+{commit.additions}</span>
                  <span className="text-error">−{commit.deletions}</span>
                  <span>{commit.file_count} files</span>
                </div>
              </Link>
            </div>
          ))}
        </section>
      )}
    </main>
  );
}
