import type { AuthRedirect } from '@bindings/services/models';
import { addToast } from '../../components/ToastContainer';
import { appStore } from '../../store';
import { openUrlInBrowser } from '../../utils/openUrl';

export function handleAuthRedirect(data: Readonly<AuthRedirect>): void {
  if (!appStore.state.authLoading) return;
  appStore.setState((state) => ({
    ...state,
    authUrl: data.url,
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
