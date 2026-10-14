import type { WSInboundMessage, WsEventType } from '../../../types/wsSchemas';
import { handleActionComplete, handleActionFailed } from './actions';
import {
  handleAuthCheckResult,
  handleAuthDisconnectResult,
  handleAuthExpired,
  handleAuthFailed,
  handleAuthRedirect,
  handleAuthSuccess,
} from './auth';
import { handleByokAuthFailed, handleByokUsage } from './classification';
import { handleCommentPosted, handleCommentsReady } from './comments';
import {
  resolveWsResponse,
  type WsEventContext,
  type WsEventHandler,
} from './context';
import {
  handleExpertiseError,
  handleExpertiseIndexProgress,
  handleExpertiseNudge,
  handleExpertiseReady,
  handleExpertiseSettings,
} from './expertise';
import {
  handleLocationError,
  handleRepoListReady,
  handleScanDevFolderResult,
  handleWatchRepoResult,
} from './repos';
import {
  handleCIStatusChanged,
  handlePRApproved,
  handlePRListChanged,
  handleThreadStateChanged,
} from './review';
import {
  handleRateLimited,
  handleSyncComplete,
  handleSyncError,
  handleSyncTriggered,
} from './sync';

export type { WsEventContext } from './context';

type WsEventHandlers = {
  readonly [Type in WsEventType]: WsEventHandler<Type>;
};

const eventHandlers: WsEventHandlers = {
  SYNC_COMPLETE: handleSyncComplete,
  SYNC_TRIGGERED: handleSyncTriggered,
  PR_APPROVED: handlePRApproved,
  COMMENT_POSTED: handleCommentPosted,
  SYNC_ERROR: handleSyncError,
  AUTH_TOKEN_EXPIRED: handleAuthExpired,
  RATE_LIMITED: handleRateLimited,
  ACTION_COMPLETE: handleActionComplete,
  ACTION_FAILED: handleActionFailed,
  THREAD_RESOLVED: handleThreadStateChanged,
  THREAD_REOPENED: handleThreadStateChanged,
  PR_METADATA_CHANGED: handlePRListChanged,
  PRS_READ_CHANGED: handlePRListChanged,
  CI_STATUS_CHANGED: handleCIStatusChanged,
  SCAN_DEV_FOLDER_RESULT: handleScanDevFolderResult,
  LOCATION_ERROR: handleLocationError,
  EXPERTISE_INDEX_PROGRESS: handleExpertiseIndexProgress,
  EXPERTISE_READY: handleExpertiseReady,
  EXPERTISE_ERROR: handleExpertiseError,
  EXPERTISE_NUDGE: handleExpertiseNudge,
  EXPERTISE_DATA: resolveWsResponse,
  EXPERTISE_ENABLE_ACCEPTED: resolveWsResponse,
  EXPERTISE_DISABLE_ACCEPTED: resolveWsResponse,
  EXPERTISE_NUDGE_DISMISSED: resolveWsResponse,
  EXPERTISE_SETTINGS: handleExpertiseSettings,
  EXPERTISE_SETTINGS_UPDATED: handleExpertiseSettings,
  AUTH_REDIRECT: handleAuthRedirect,
  AUTH_CHECK_RESULT: handleAuthCheckResult,
  AUTH_DISCONNECT_RESULT: handleAuthDisconnectResult,
  AUTH_SUCCESS: handleAuthSuccess,
  AUTH_FAILED: handleAuthFailed,
  COMMENTS_READY: handleCommentsReady,
  BYOK_USAGE: handleByokUsage,
  BYOK_AUTH_FAILED: handleByokAuthFailed,
  PR_SYNCED: resolveWsResponse,
  PRS_READ_ACK: resolveWsResponse,
  DIFF_READY: resolveWsResponse,
  PATCHSETS_READY: resolveWsResponse,
  RANGE_DIFF_READY: resolveWsResponse,
  FILE_VIEWED_ACK: resolveWsResponse,
  REVIEW_THREADS_READY: resolveWsResponse,
  ACTION_RESULT: resolveWsResponse,
  PR_CHECKOUT_STATUS: resolveWsResponse,
  PR_CHECKOUT_RESULT: resolveWsResponse,
  VALIDATE_DEV_FOLDER_RESULT: resolveWsResponse,
  ADD_REPO_BY_URL_RESULT: resolveWsResponse,
  GET_CACHE_STATS_RESULT: resolveWsResponse,
  SET_EXPERTISE_SETTINGS: resolveWsResponse,
  PRUNE_CACHE_RESULT: resolveWsResponse,
  CLEAR_CACHE_RESULT: resolveWsResponse,
  DELETE_CACHE_REPO_RESULT: resolveWsResponse,
  RECOVER_REPO_PATH_RESULT: resolveWsResponse,
  RECOVER_REPO_TO_CACHE_RESULT: resolveWsResponse,
  REMOVE_REPO_PREFLIGHT_RESULT: resolveWsResponse,
  REMOVE_REPO_RESULT: resolveWsResponse,
  CLASSIFICATION_SETTINGS_UPDATED: resolveWsResponse,
  BYOK_KEY_UPDATED: resolveWsResponse,
  DISMISS_RESULT: resolveWsResponse,
  UNDISMISS_RESULT: resolveWsResponse,
  PR_LIST_READY: resolveWsResponse,
  REPO_LIST_READY: handleRepoListReady,
  WATCH_REPO_RESULT: handleWatchRepoResult,
} satisfies WsEventHandlers;

/** Dispatches a validated event to its domain without owning the connection. */
export function dispatchWsEvent<Type extends WsEventType>(
  message: Readonly<WSInboundMessage<Type>>,
  context: WsEventContext,
): void {
  eventHandlers[message.type](message, context);
}
