import React from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@renderer/components/ui/tabs';
import { Checkbox } from '@renderer/components/ui/checkbox';
import { Button } from '@renderer/components/ui/button';
import { SettingsLayout } from '../layouts/SettingsLayout';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { persistSettings } from '../store/ui';
import type { UIState } from '../store/ui';

export function SettingsPage() {
  const darkMode = useStore(appStore, (s: UIState) => s.darkMode);
  const showFileTree = useStore(appStore, (s: UIState) => s.showFileTree);
  const autoSync = useStore(appStore, (s: UIState) => s.autoSync);
  const syncInterval = useStore(appStore, (s: UIState) => s.syncInterval);
  const notifications = useStore(appStore, (s: UIState) => s.notifications);

  const updateSetting = <K extends keyof UIState>(key: K, value: UIState[K]): void => {
    appStore.setState((s) => {
      const next = { ...s, [key]: value };
      persistSettings(next);
      return next;
    });
  };

  const handleOpenShortcuts = (): void => {
    // TODO: Integrate with HelpOverlay via keyboard bindings
    appStore.setState((s) => ({ ...s, mode: 'help' }));
  };

  return (
    <SettingsLayout
      title="Settings"
      description="Configure Bistro to your preferences"
    >
      <Tabs defaultValue="appearance" className="w-full">
        <TabsList className="grid w-full grid-cols-3 border-b border-outline-variant bg-transparent p-0 rounded-none">
          <TabsTrigger
            value="appearance"
            className="border-b-2 border-b-transparent data-[state=active]:border-b-primary rounded-none"
          >
            Appearance
          </TabsTrigger>
          <TabsTrigger
            value="sync"
            className="border-b-2 border-b-transparent data-[state=active]:border-b-primary rounded-none"
          >
            Sync
          </TabsTrigger>
          <TabsTrigger
            value="keyboard"
            className="border-b-2 border-b-transparent data-[state=active]:border-b-primary rounded-none"
          >
            Keyboard
          </TabsTrigger>
        </TabsList>

        <TabsContent value="appearance" className="space-y-6 mt-6">
          <div className="flex items-center space-x-3">
            <Checkbox
              id="dark-mode"
              checked={darkMode}
              onCheckedChange={() => updateSetting('darkMode', !darkMode)}
            />
            <label
              htmlFor="dark-mode"
              className="text-sm cursor-pointer text-on-surface font-medium"
            >
              Dark mode
            </label>
          </div>
          <div className="flex items-center space-x-3">
            <Checkbox
              id="file-tree"
              checked={showFileTree}
              onCheckedChange={() => updateSetting('showFileTree', !showFileTree)}
            />
            <label
              htmlFor="file-tree"
              className="text-sm cursor-pointer text-on-surface font-medium"
            >
              Show file tree in diff view
            </label>
          </div>
        </TabsContent>

        <TabsContent value="sync" className="space-y-6 mt-6">
          <div className="flex items-center space-x-3">
            <Checkbox
              id="auto-sync"
              checked={autoSync}
              onCheckedChange={() => updateSetting('autoSync', !autoSync)}
            />
            <label
              htmlFor="auto-sync"
              className="text-sm cursor-pointer text-on-surface font-medium"
            >
              Auto-sync
            </label>
          </div>
          <div className="flex items-center space-x-3 gap-3">
            <label
              htmlFor="sync-interval"
              className="text-sm text-on-surface font-medium"
            >
              Sync interval (seconds)
            </label>
            <input
              id="sync-interval"
              type="number"
              min={10}
              max={3600}
              value={syncInterval}
              onChange={(e) => {
                const val = parseInt(e.target.value, 10);
                if (!Number.isNaN(val) && val >= 10 && val <= 3600) {
                  updateSetting('syncInterval', val);
                }
              }}
              className="w-20 px-2 py-1 border border-outline-variant bg-surface-container-lowest text-on-surface font-mono text-sm"
            />
          </div>
          <div className="flex items-center space-x-3">
            <Checkbox
              id="notifications"
              checked={notifications}
              onCheckedChange={() => updateSetting('notifications', !notifications)}
            />
            <label
              htmlFor="notifications"
              className="text-sm cursor-pointer text-on-surface font-medium"
            >
              Notify on new PRs
            </label>
          </div>
        </TabsContent>

        <TabsContent value="keyboard" className="mt-6">
          <Button
            onClick={handleOpenShortcuts}
            className="bg-primary text-on-primary hover:bg-primary-container"
          >
            View All Shortcuts
          </Button>
        </TabsContent>
      </Tabs>
    </SettingsLayout>
  );
}
