import type { MenuAction } from '@bindings/services/models';
import { appStore } from '../../store';

/** Menu clicks queue as store requests; useAppHotkeys drains them with the live action context. */
export function handleMenuAction(data: Readonly<MenuAction>): void {
  appStore.setState((state) => ({
    ...state,
    menuActionRequests: [
      ...state.menuActionRequests,
      { actionId: data.action_id, sequence: data.sequence },
    ],
  }));
}
