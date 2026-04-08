import React, { useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@renderer/components/ui/tabs';
import { Checkbox } from '@renderer/components/ui/checkbox';
import { Button } from '@renderer/components/ui/button';
import { SettingsLayout } from '../layouts/SettingsLayout';

interface AppearanceSettings {
  darkMode: boolean;
  showFileTree: boolean;
}

interface SyncSettings {
  autoSync: boolean;
  syncInterval: number;
  notifications: boolean;
}

export function SettingsPage() {
  const [appearance, setAppearance] = useState<AppearanceSettings>({
    darkMode: true,
    showFileTree: false,
  });

  const [sync, setSync] = useState<SyncSettings>({
    autoSync: true,
    syncInterval: 60,
    notifications: true,
  });

  const toggleAppearance = (key: keyof AppearanceSettings) => {
    setAppearance((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  const toggleSync = (key: keyof Pick<SyncSettings, 'autoSync' | 'notifications'>) => {
    setSync((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  const handleOpenShortcuts = () => {
    // TODO: Integrate with HelpOverlay via keyboard bindings
    console.log('Opening shortcuts overlay...');
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
              checked={appearance.darkMode}
              onCheckedChange={() => toggleAppearance('darkMode')}
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
              checked={appearance.showFileTree}
              onCheckedChange={() => toggleAppearance('showFileTree')}
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
              checked={sync.autoSync}
              onCheckedChange={() => toggleSync('autoSync')}
            />
            <label
              htmlFor="auto-sync"
              className="text-sm cursor-pointer text-on-surface font-medium"
            >
              Auto-sync every {sync.syncInterval} seconds
            </label>
          </div>
          <div className="flex items-center space-x-3">
            <Checkbox
              id="notifications"
              checked={sync.notifications}
              onCheckedChange={() => toggleSync('notifications')}
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
