import { Badge } from '@renderer/components/ui/badge';
import { Button } from '@renderer/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@renderer/components/ui/table';
import type React from 'react';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { useWebSocketPRList as usePRList } from '../hooks/useWebSocketPRList';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { PR } from '../types';

type PRState = 'open' | 'closed' | 'draft' | 'merged';

type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'outline';

function getBadgeVariant(state: PRState): BadgeVariant {
  switch (state) {
    case 'open':
      return 'default';
    case 'merged':
      return 'secondary';
    case 'closed':
      return 'destructive';
    case 'draft':
      return 'outline';
  }
}

function formatDate(dateStr: string): string {
  // Use en-US with numeric (non-padded) month/day for consistent cross-platform output
  return new Date(dateStr).toLocaleDateString('en-US', {
    month: 'numeric',
    day: 'numeric',
    year: 'numeric',
  });
}

interface PRRowProps {
  pr: PR;
  idx: number;
  selectedIndex: number;
  onApprove: (prId: string, event: React.MouseEvent) => void;
  showRepo?: boolean;
}

function PRRow({
  pr,
  idx,
  selectedIndex,
  onApprove,
  showRepo = true,
}: PRRowProps): React.ReactElement {
  return (
    <TableRow
      key={pr.id}
      tabIndex={0}
      className={
        selectedIndex === idx
          ? 'cursor-pointer bg-surface-container-low border-l-[3px] border-l-primary'
          : 'cursor-pointer hover:bg-surface-container-low transition-colors'
      }
      onClick={() => {
        appStore.setState((s) => ({
          ...s,
          selectedPRIndex: idx,
          mode: 'detail',
        }));
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter' && e.target === e.currentTarget) {
          appStore.setState((s) => ({
            ...s,
            selectedPRIndex: idx,
            mode: 'detail',
          }));
        }
      }}
    >
      <TableCell>
        <Badge variant={getBadgeVariant(pr.state)}>{pr.state}</Badge>
      </TableCell>
      <TableCell className="font-medium text-on-surface">{pr.title}</TableCell>
      {showRepo && (
        <TableCell className="font-mono text-[11px] text-on-surface-variant">
          {pr.owner}/{pr.repo}
        </TableCell>
      )}
      <TableCell className="text-right">
        <span className="font-mono text-[11px] text-on-surface-variant">
          {formatDate(pr.updated_at)}
        </span>
      </TableCell>
      <TableCell className="text-right font-mono text-[11px] text-primary">
        {pr.author}
      </TableCell>
      <TableCell className="text-right">
        {pr.state === 'open' && (
          <Button
            variant="outline"
            size="sm"
            className="text-[10px] h-6 px-2"
            onClick={(e) => onApprove(pr.id, e)}
          >
            Approve
          </Button>
        )}
      </TableCell>
    </TableRow>
  );
}

export function PRListTable() {
  const filterState = useStore(appStore, (state: UIState) => state.filterState);
  const selectedIndex = useStore(
    appStore,
    (state: UIState) => state.selectedPRIndex,
  );
  const prListView = useStore(appStore, (state: UIState) => state.prListView);
  const repoFilter = useStore(appStore, (state: UIState) => state.repoFilter);
  const repos = useStore(appStore, (state: UIState) => state.repos);
  const { data: prs = [], isLoading, error } = usePRList(filterState);
  const { send } = useWebSocket();

  const handleApprove = (prId: string, event: React.MouseEvent): void => {
    event.stopPropagation();
    send('APPROVE_PR', { pr_id: prId });
  };

  const handleClearRepoFilter = (): void => {
    appStore.setState((s) => ({ ...s, repoFilter: null }));
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-32 text-on-surface-variant">
        Loading PRs...
      </div>
    );
  }

  if (error !== null) {
    return (
      <div className="flex items-center justify-center h-32 text-error">
        Failed to load PRs: {(error as Error).message}
      </div>
    );
  }

  // Empty state when no repos watched
  const watchedRepos = (repos ?? []).filter((r) => r.watched);
  if (prs.length === 0 && watchedRepos.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-64 text-on-surface-variant">
        <p className="text-lg font-semibold">No repositories watched</p>
        <p className="text-sm mt-2">
          Press{' '}
          <kbd className="px-1 py-0.5 bg-surface-container rounded text-xs font-mono">
            :
          </kbd>{' '}
          then type{' '}
          <kbd className="px-1 py-0.5 bg-surface-container rounded text-xs font-mono">
            repos
          </kbd>{' '}
          to discover and watch your GitHub repositories.
        </p>
      </div>
    );
  }

  // Apply repo filter
  const filteredPRs = repoFilter
    ? prs.filter((pr) => `${pr.owner}/${pr.repo}` === repoFilter)
    : prs;

  return (
    <div className="w-full border border-outline-variant shadow-ambient">
      {repoFilter !== null && (
        <div className="flex items-center gap-2 px-4 py-2 bg-surface-container border-b border-outline-variant">
          <span className="text-xs text-on-surface-variant font-mono">
            Filtered: {repoFilter}
          </span>
          <button
            onClick={handleClearRepoFilter}
            className="text-xs text-primary hover:underline"
            type="button"
          >
            Clear
          </button>
        </div>
      )}

      {prListView === 'grouped' ? (
        <GroupedPRTable
          prs={filteredPRs}
          selectedIndex={selectedIndex}
          onApprove={handleApprove}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[80px]">State</TableHead>
              <TableHead>Title</TableHead>
              <TableHead className="w-[160px]">Repo</TableHead>
              <TableHead className="text-right w-[120px]">Updated</TableHead>
              <TableHead className="text-right w-[100px]">Author</TableHead>
              <TableHead className="w-[90px]"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredPRs.map((pr, idx) => (
              <PRRow
                key={pr.id}
                pr={pr}
                idx={idx}
                selectedIndex={selectedIndex}
                onApprove={handleApprove}
                showRepo={true}
              />
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}

interface GroupedPRTableProps {
  prs: PR[];
  selectedIndex: number;
  onApprove: (prId: string, event: React.MouseEvent) => void;
}

function GroupedPRTable({
  prs,
  selectedIndex,
  onApprove,
}: GroupedPRTableProps): React.ReactElement {
  // Group by owner/repo
  const groups = new Map<string, PR[]>();
  for (const pr of prs) {
    const key = `${pr.owner}/${pr.repo}`;
    const existing = groups.get(key);
    if (existing) {
      existing.push(pr);
    } else {
      groups.set(key, [pr]);
    }
  }

  // Build flat index for selection alignment
  let globalIdx = 0;

  return (
    <div>
      {Array.from(groups.entries()).map(([repoKey, repoPRs]) => {
        const startIdx = globalIdx;
        globalIdx += repoPRs.length;
        return (
          <div key={repoKey} className="mb-2">
            <div className="px-4 py-2 bg-surface-container border-b border-outline-variant flex items-center justify-between">
              <span className="font-mono text-sm font-semibold text-on-surface">
                {repoKey}
              </span>
              <span className="text-xs text-on-surface-variant">
                {repoPRs.length} PR{repoPRs.length !== 1 ? 's' : ''}
              </span>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[80px]">State</TableHead>
                  <TableHead>Title</TableHead>
                  <TableHead className="text-right w-[120px]">
                    Updated
                  </TableHead>
                  <TableHead className="text-right w-[100px]">Author</TableHead>
                  <TableHead className="w-[90px]"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {repoPRs.map((pr, i) => (
                  <PRRow
                    key={pr.id}
                    pr={pr}
                    idx={startIdx + i}
                    selectedIndex={selectedIndex}
                    onApprove={onApprove}
                    showRepo={false}
                  />
                ))}
              </TableBody>
            </Table>
          </div>
        );
      })}
    </div>
  );
}
