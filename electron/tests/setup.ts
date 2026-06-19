// Test-only DOM shims. jsdom does not implement ResizeObserver or
// Element.scrollIntoView, both of which `cmdk` uses at runtime. Install no-op
// versions only when missing so real browsers / the Electron renderer keep
// their native implementations. Wired in via vitest.config.ts `setupFiles`.
if (typeof globalThis.ResizeObserver === 'undefined') {
  class ResizeObserverShim {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  (globalThis as { ResizeObserver?: unknown }).ResizeObserver =
    ResizeObserverShim;
}

if (
  typeof Element !== 'undefined' &&
  typeof Element.prototype.scrollIntoView !== 'function'
) {
  Element.prototype.scrollIntoView = function scrollIntoView(): void {};
}
