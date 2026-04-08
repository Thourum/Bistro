import { Store } from '@tanstack/store';
import type { UIState } from './ui';
import { createUIStore } from './ui';

export type AppStore = UIState;

export const appStore: Store<UIState> = createUIStore();
