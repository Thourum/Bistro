import React from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@renderer/components/ui/table';
import { Badge } from '@renderer/components/ui/badge';
import { usePRList } from '../queries/usePRList';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

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
  return new Date(dateStr).toLocaleDateString();
}

export function PRListTable() {
  const filterState = useStore(appStore, (state: UIState) => state.filterState);
  const selectedIndex = useStore(appStore, (state: UIState) => state.selectedPRIndex);
  const { data: prs = [] } = usePRList('github:default', filterState);

  return (
    <div className="w-full border border-outline-variant shadow-ambient">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-[80px]">State</TableHead>
            <TableHead>Title</TableHead>
            <TableHead className="text-right w-[120px]">Updated</TableHead>
            <TableHead className="text-right w-[100px]">Author</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {prs.map((pr, idx) => (
            <TableRow
              key={pr.id}
              className={
                selectedIndex === idx
                  ? 'cursor-pointer bg-surface-container-low border-l-[3px] border-l-primary'
                  : 'cursor-pointer hover:bg-surface-container-low transition-colors'
              }
            >
              <TableCell>
                <Badge variant={getBadgeVariant(pr.state)}>
                  {pr.state}
                </Badge>
              </TableCell>
              <TableCell className="font-medium text-on-surface">
                {pr.title}
              </TableCell>
              <TableCell className="text-right">
                <span className="font-mono text-[11px] text-on-surface-variant">
                  {formatDate(pr.updated_at)}
                </span>
              </TableCell>
              <TableCell className="text-right font-mono text-[11px] text-primary">
                {pr.author}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
