import { afterAll } from 'vitest';

// @wailsio/runtime's drag.js starts a 50 ms window.setInterval on import whose
// first tick calls window.clearInterval; if a jsdom file tears down before it
// fires, the tick throws "window is not defined". Outlive one tick.
const WAILS_DRAG_POLL_MS = 60;

if (typeof window !== 'undefined') {
  afterAll(
    () => new Promise((resolve) => setTimeout(resolve, WAILS_DRAG_POLL_MS)),
  );
}
