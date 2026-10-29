import type { ByokAuthFailed, ByokUsage } from '@bindings/services/models';
import { addToast } from '../../components/ToastContainer';
import { appStore } from '../../store';

export function handleByokUsage(data: Readonly<ByokUsage>): void {
  appStore.setState((state) => ({
    ...state,
    byokSessionTokens: data.session_tokens,
    byokEstCost: data.est_cost,
  }));
}

export function handleByokAuthFailed(_data: Readonly<ByokAuthFailed>): void {
  appStore.setState((state) => ({ ...state, byokAuthFailed: true }));
  addToast({
    type: 'error',
    message: 'AI classification: API key rejected — check Settings',
    dismissible: true,
  });
}
