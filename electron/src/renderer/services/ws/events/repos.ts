import { Fault } from '@thourum/fault';
import { addToast } from '../../../components/ToastContainer';
import { appStore } from '../../../store';
import type { WSInboundMessage } from '../../../types/wsSchemas';
import { createLogger } from '../../../utils/logger';
import type { WsEventContext } from './context';

const log = createLogger('WebSocket');

export function handleScanDevFolderResult(
  message: Readonly<WSInboundMessage<'SCAN_DEV_FOLDER_RESULT'>>,
  context: WsEventContext,
): void {
  const { data } = message;
  if ('errors' in data && data.errors.length > 0) {
    log.warn(
      { type: message.type, id: message.id },
      'dev-folder scan completed with location errors',
    );
  }
  context.resolveResponse(message.id, data);
}

export function handleLocationError(
  message: Readonly<WSInboundMessage<'LOCATION_ERROR'>>,
): void {
  const fault = new Fault(message.data.message);
  log.warn(
    { type: message.type, id: message.id, message: fault.message },
    'repository location error',
  );
}

export function handleRepoListReady(
  message: Readonly<WSInboundMessage<'REPO_LIST_READY'>>,
  context: WsEventContext,
): void {
  const { data } = message;
  if (data.error) {
    appStore.setState((state) => ({ ...state, reposLoading: false }));
    addToast({
      type: 'error',
      message: `Failed to load repos: ${data.error}`,
      dismissible: true,
    });
  } else {
    appStore.setState((state) => ({
      ...state,
      repos: (data.repos ?? []).map(
        ({
          source,
          location_status,
          location_error_code,
          location_error,
          ...repo
        }) => ({
          ...repo,
          ...(source === undefined ? {} : { source }),
          ...(location_status === undefined ? {} : { location_status }),
          ...(location_error_code === undefined ? {} : { location_error_code }),
          ...(location_error === undefined ? {} : { location_error }),
        }),
      ),
      reposLoading: false,
    }));
  }
  context.resolveResponse(message.id, data);
}

export function handleWatchRepoResult(
  message: Readonly<WSInboundMessage<'WATCH_REPO_RESULT'>>,
  context: WsEventContext,
): void {
  const { data } = message;
  appStore.setState((state) => ({
    ...state,
    repos: state.repos.map((repo) =>
      repo.id === data.repo_id ? { ...repo, watched: data.watched } : repo,
    ),
  }));
  context.resolveResponse(message.id, data);
}
