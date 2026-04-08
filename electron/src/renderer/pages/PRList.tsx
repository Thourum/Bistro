import { useStore } from '../hooks/useStore';
import type { UIState } from '../store/ui';
import { usePRList } from '../queries/usePRList';
import CommandBar from '../components/CommandBar';
import StatusBar from '../components/StatusBar';
import PRListItem from '../components/PRListItem';
import { appStore } from '../store';

export default function PRList() {
  const filterState = useStore(appStore, (state: UIState) => state.filterState);
  const selectedIndex = useStore(appStore, (state: UIState) => state.selectedPRIndex);
  const { data: prs = [], isLoading, error } = usePRList('github:default', filterState);

  const handlePRClick = (index: number) => {
    appStore.setState((s) => ({
      ...s,
      selectedPRIndex: index,
      mode: 'detail',
    }));
  };

  if (error) {
    return (
      <div className="error-state">
        <p>Failed to load PRs: {error.message}</p>
      </div>
    );
  }

  return (
    <div className="pr-list-view">
      <CommandBar />
      <div className="pr-list-pane">
        {isLoading ? (
          <div className="loading">Loading PRs...</div>
        ) : prs.length === 0 ? (
          <div className="empty-state">No PRs found</div>
        ) : (
          <ul className="pr-list">
            {prs.map((pr, index) => (
              <div
                key={pr.id}
                onClick={() => handlePRClick(index)}
                role="button"
                tabIndex={0}
              >
                <PRListItem
                  pr={pr}
                  isSelected={index === selectedIndex}
                  index={index}
                />
              </div>
            ))}
          </ul>
        )}
      </div>
      <StatusBar />
    </div>
  );
}
