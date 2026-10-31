import { useEffect } from 'react';
import { useStore } from '../hooks/useStore';
import { dismissToast, type Toast } from '../services/toasts';
import { appStore } from '../store';

const AUTO_DISMISS_MS = 5000;

const TOAST_STYLES: Record<Toast['type'], string> = {
  error: 'bg-error text-on-error',
  success: 'bg-green-600 text-white',
  info: 'bg-surface-container-high text-on-surface border border-outline-variant',
  warning: 'bg-yellow-600 text-white',
};

const TOAST_ICONS: Record<Toast['type'], string> = {
  error: '✗',
  success: '✓',
  info: 'ℹ',
  warning: '⚠',
};

interface ToastItemProps {
  toast: Toast;
}

function ToastItem({ toast }: ToastItemProps): React.ReactElement {
  useEffect(() => {
    if (toast.type === 'success' || toast.type === 'info') {
      const timer = setTimeout(() => dismissToast(toast.id), AUTO_DISMISS_MS);
      return () => clearTimeout(timer);
    }
    return undefined;
  }, [toast.id, toast.type]);

  return (
    <div
      data-toast
      className={`flex items-center gap-2 px-4 py-2 rounded shadow-md text-sm font-mono ${TOAST_STYLES[toast.type]}`}
      role="alert"
    >
      <span>{TOAST_ICONS[toast.type]}</span>
      <span className="flex-1">{toast.message}</span>
      {toast.dismissible && (
        <button
          type="button"
          onClick={() => dismissToast(toast.id)}
          className="ml-2 text-xs opacity-70 hover:opacity-100"
          aria-label="Dismiss"
        >
          ×
        </button>
      )}
    </div>
  );
}

export default function ToastContainer(): React.ReactElement | null {
  const toasts = useStore(appStore, (s) => s.toasts);

  if (toasts.length === 0) {
    return null;
  }

  return (
    <div className="fixed bottom-8 right-4 z-50 flex flex-col gap-2 max-w-sm w-full pointer-events-none">
      {toasts.map((toast) => (
        <div key={toast.id} className="pointer-events-auto">
          <ToastItem toast={toast} />
        </div>
      ))}
    </div>
  );
}
