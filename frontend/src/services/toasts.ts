import { appStore } from '../store';
import type { Toast } from '../types';

export type { Toast } from '../types';

export function dismissToast(id: string): void {
  appStore.setState((s) => ({
    ...s,
    toasts: s.toasts.filter((t) => t.id !== id),
  }));
}

/**
 * Add a toast to the store queue (max 3, oldest dropped first).
 */
export function addToast(toast: Omit<Toast, 'id'>): void {
  const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  appStore.setState((s) => {
    const next = [...s.toasts, { ...toast, id }];
    return {
      ...s,
      toasts: next.length > 3 ? next.slice(next.length - 3) : next,
    };
  });
}
