import { addToast } from '../../../components/ToastContainer';
import { appStore } from '../../../store';
import type { WSInboundMessage } from '../../../types/wsSchemas';

export function handleByokUsage({
  data,
}: Readonly<WSInboundMessage<'BYOK_USAGE'>>): void {
  appStore.setState((state) => ({
    ...state,
    byokSessionTokens: data.session_tokens ?? 0,
    byokEstCost: data.est_cost ?? 0,
  }));
}

export function handleByokAuthFailed(): void {
  appStore.setState((state) => ({ ...state, byokAuthFailed: true }));
  addToast({
    type: 'error',
    message: 'AI classification: API key rejected — check Settings',
    dismissible: true,
  });
}
