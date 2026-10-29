import type { Settings } from '@bindings/shell/models';
import { settingsStore } from '../../store/settings';
import { appSettingsSchema } from '../../types/electron/settings';
import { createLogger } from '../../utils/logger';

const log = createLogger('EngineEvents');

export function handleSettingsChanged(data: Readonly<Settings>): void {
  // ponytail: Task 18 replaces this legacy settings type bridge.
  const parsed = appSettingsSchema.safeParse({
    ...data,
    keybindings: data.keybindings ?? {},
  });
  if (!parsed.success) {
    log.warn('invalid settings update from engine');
    return;
  }
  settingsStore.setState(() => parsed.data);
}
