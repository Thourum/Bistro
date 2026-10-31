import { useEffect } from 'react';
import { settingsStore } from '../store/settings';
import { useStore } from './useStore';

/**
 * Applies user-configurable appearance settings to the document.
 *
 * - Toggles the `dark` class on <html>
 * - Writes `--app-ui-font`, `--app-code-font`, `--app-ui-font-size`,
 *   `--app-code-font-size` CSS variables on :root
 * - Applies the `high-density` class on <html> for conditional styling hooks
 *
 * The CSS variables are consumed by [globals.css](../styles/globals.css)
 * which drives every `font-sans` / `font-mono` utility class.
 */
export function useSettingsEffect(): void {
  const theme = useStore(settingsStore, (s) => s.appearance.theme);
  const uiFont = useStore(settingsStore, (s) => s.appearance.uiFont);
  const codeFont = useStore(settingsStore, (s) => s.appearance.codeFont);
  const uiFontSize = useStore(settingsStore, (s) => s.appearance.uiFontSize);
  const codeFontSize = useStore(
    settingsStore,
    (s) => s.appearance.codeFontSize,
  );
  const highDensity = useStore(settingsStore, (s) => s.appearance.highDensity);

  useEffect(() => {
    const root = document.documentElement;
    root.classList.toggle('dark', theme === 'dark');
  }, [theme]);

  useEffect(() => {
    const root = document.documentElement;
    root.style.setProperty('--app-ui-font', uiFont);
  }, [uiFont]);

  useEffect(() => {
    const root = document.documentElement;
    root.style.setProperty('--app-code-font', codeFont);
  }, [codeFont]);

  useEffect(() => {
    const root = document.documentElement;
    root.style.setProperty('--app-ui-font-size', `${uiFontSize}px`);
  }, [uiFontSize]);

  useEffect(() => {
    const root = document.documentElement;
    root.style.setProperty('--app-code-font-size', `${codeFontSize}px`);
  }, [codeFontSize]);

  useEffect(() => {
    const root = document.documentElement;
    root.classList.toggle('high-density', highDensity);
  }, [highDensity]);
}
