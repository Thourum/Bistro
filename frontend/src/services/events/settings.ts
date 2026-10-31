import type { Settings } from '@bindings/shell/models';
import { applySettings } from '../../store/settings';

export function handleSettingsChanged(settings: Readonly<Settings>): void {
  applySettings(settings);
}
