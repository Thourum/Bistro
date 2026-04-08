import type { PR } from '../types';
import './PRListItem.css';

interface Props {
  pr: PR;
  isSelected: boolean;
  index: number;
}

export default function PRListItem({ pr, isSelected }: Props) {
  const stateColor: Record<string, string> = {
    open: 'green',
    closed: 'red',
    merged: 'purple',
  };

  return (
    <li className={`pr-list-item ${isSelected ? 'selected' : ''}`}>
      <div className="pr-item-header">
        <span className={`state-badge state-${pr.draft ? 'gray' : (stateColor[pr.state] || 'gray')}`}>
          {pr.draft ? 'DRAFT' : pr.state.toUpperCase()}
        </span>
        <span className="pr-title">
          {pr.title} <span className="pr-number">#{pr.number}</span>
        </span>
      </div>
      <div className="pr-item-meta">
        <span className="pr-repo">{pr.owner}/{pr.repo}</span>
        <span className="separator">•</span>
        <span className="pr-author">@{pr.author}</span>
        <span className="separator">•</span>
        <span className="pr-date">{formatDate(pr.updated_at)}</span>
      </div>
      <div className="pr-item-stats">
        <span className="files">{pr.comment_count} comments</span>
        {pr.ci_status !== null && (
          <>
            <span className="separator">|</span>
            <span className="files">CI: {pr.ci_status}</span>
          </>
        )}
      </div>
    </li>
  );
}

function formatDate(iso: string): string {
  const date = new Date(iso);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays === 0) return 'today';
  if (diffDays === 1) return 'yesterday';
  if (diffDays < 7) return `${diffDays} days ago`;
  return date.toLocaleDateString();
}
