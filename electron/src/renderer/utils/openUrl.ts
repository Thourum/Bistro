/**
 * Open a URL in the user's real browser.
 *
 * In Electron this goes through the main process (`openExternal`); under
 * `pnpm dev:web` it falls back to `window.open`.
 */
export function openUrlInBrowser(url: string): void {
  if (window.electron?.openExternal !== undefined) {
    void window.electron.openExternal(url);
    return;
  }
  window.open(url, '_blank', 'noopener,noreferrer');
}
