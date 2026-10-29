import type { AuthRedirect } from '@bindings/services/models';
import { addToast } from '../../components/ToastContainer';
import { appStore } from '../../store';
import { openUrlInBrowser } from '../../utils/openUrl';

export function handleAuthRedirect(data: Readonly<AuthRedirect>): void {
  appStore.setState((state) => ({
    ...state,
    authUrl: data.url,
    authStartRequestId: null,
    authLoading: true,
    authStartedAt: Date.now(),
    authTimedOut: false,
    authInterrupted: false,
    authError: null,
  }));
  void openUrlInBrowser(data.url).match(
    () => undefined,
    (fault) =>
      addToast({ type: 'error', message: fault.message, dismissible: true }),
  );
}
