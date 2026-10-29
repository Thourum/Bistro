/** Dispatches renderer app events, optionally with typed benchmark details. */
export function emitAppEvent(
  name: string,
  detail?: Readonly<Record<string, unknown>>,
): boolean {
  if (typeof document === 'undefined') return true;
  return document.dispatchEvent(
    new CustomEvent(name, { detail, cancelable: detail !== undefined }),
  );
}
