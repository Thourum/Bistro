import React from 'react';
import { MainLayout } from '../layouts/MainLayout';
import { PRListTable } from '../components/PRListTable';
import { FilterMenu } from '../components/FilterMenu';
import { SearchBar } from '../components/SearchBar';
import { useSyncEvents } from '../hooks/useSyncEvents';

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
