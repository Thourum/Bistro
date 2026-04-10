import { FilterMenu } from '../components/FilterMenu';
import { PRListTable } from '../components/PRListTable';
import { SearchBar } from '../components/SearchBar';
import { useSyncEvents } from '../hooks/useSyncEvents';
import { MainLayout } from '../layouts/MainLayout';

export function PRListPage() {
  useSyncEvents();

  return (
    <MainLayout
      toolbar={
        <div className="flex items-center gap-3 w-full">
          <SearchBar />
          <FilterMenu />
        </div>
      }
    >
      <PRListTable />
    </MainLayout>
  );
}
