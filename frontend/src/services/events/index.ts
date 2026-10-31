import { Events } from '@wailsio/runtime';
import type {} from '../../../bindings/github.com/wailsapp/wails/v3/internal/eventdata';
import { handleActionComplete } from './actions';
import { handleAuthRedirect } from './auth';
import { handleByokAuthFailed, handleByokUsage } from './classification';
import { handleCommentPosted, handleCommentsClassified } from './comments';
import {
  handleExpertiseError,
  handleExpertiseNudge,
  handleExpertiseProgress,
  handleExpertiseReady,
} from './expertise';
import { handleMenuAction } from './menu';
import {
  handleDraftsChanged,
  handlePRApproved,
  handlePRListChanged,
  handleThreadStateChanged,
} from './review';
import { handleSettingsChanged } from './settings';
import {
  handleRateLimited,
  handleSyncComplete,
  handleSyncError,
  handleSyncStatus,
} from './sync';

export type EngineEvent = Exclude<keyof Events.CustomEvents, `wails:${string}`>;
export type EngineEventHandlers = {
  readonly [K in EngineEvent]: (data: Events.CustomEvents[K]) => void;
};

export const eventHandlers: EngineEventHandlers = {
  'action:complete': handleActionComplete,
  'auth:redirect': handleAuthRedirect,
  'byok:auth-failed': handleByokAuthFailed,
  'byok:usage': handleByokUsage,
  'comment:posted': handleCommentPosted,
  'comments:classified': handleCommentsClassified,
  'drafts:changed': handleDraftsChanged,
  'expertise:error': handleExpertiseError,
  'expertise:nudge': handleExpertiseNudge,
  'expertise:progress': handleExpertiseProgress,
  'expertise:ready': handleExpertiseReady,
  'menu:action': handleMenuAction,
  'pr:approved': handlePRApproved,
  'pr:ci-status-changed': handlePRListChanged,
  'pr:metadata-changed': handlePRListChanged,
  'prs:read-changed': handlePRListChanged,
  'settings:changed': handleSettingsChanged,
  'sync:complete': handleSyncComplete,
  'sync:error': handleSyncError,
  'sync:rate-limited': handleRateLimited,
  'sync:status': handleSyncStatus,
  'thread:state-changed': handleThreadStateChanged,
} satisfies EngineEventHandlers;

function subscribe<K extends EngineEvent>(name: K): () => void {
  const handler: EngineEventHandlers[K] = eventHandlers[name];
  return Events.On(name, (event) => handler(event.data));
}

/** Subscribes once per engine event and returns cleanup for every listener. */
export function subscribeEngineEvents(): () => void {
  const names = Object.keys(eventHandlers).filter(
    (name): name is EngineEvent => name in eventHandlers,
  );
  const offs = names.map((name) => subscribe(name));
  return () => {
    for (const off of offs) off();
  };
}
