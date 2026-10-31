import { beforeEach, describe, expect, it } from 'vitest';
import { appStore } from '../../store';
import { settingsStore } from '../../store/settings';
import { DEFAULT_SETTINGS } from '../../types/settings';
import { handleActionComplete } from './actions';
import { handleCommentsClassified } from './comments';
import { handleExpertiseError } from './expertise';
import { handleMenuAction } from './menu';
import { handleSettingsChanged } from './settings';

const initialState = appStore.state;

beforeEach(() => {
  appStore.setState(() => initialState);
  settingsStore.setState(() => DEFAULT_SETTINGS);
});

describe('engine push handlers', () => {
  it('queues repeated menu clicks without losing earlier requests', () => {
    handleMenuAction({ action_id: 'global.inbox', sequence: 1 });
    handleMenuAction({ action_id: 'global.inbox', sequence: 2 });
    expect(appStore.state.menuActionRequests).toEqual([
      { actionId: 'global.inbox', sequence: 1 },
      { actionId: 'global.inbox', sequence: 2 },
    ]);
    expect(appStore.state.selectedPRIndex).toBe(initialState.selectedPRIndex);
  });

  it('normalizes nullable settings overrides and preserves settings on invalid pushes', () => {
    const settings = {
      ...DEFAULT_SETTINGS,
      appearance: { ...DEFAULT_SETTINGS.appearance, highDensity: true },
      keybindings: null,
      window: { x: 0, y: 0, width: 0, height: 0, maximised: false },
    };
    handleSettingsChanged(settings);
    expect(settingsStore.state.appearance.highDensity).toBe(true);
    expect(settingsStore.state.keybindings).toEqual({});
    const accepted = settingsStore.state;
    handleSettingsChanged({ ...settings, $schema: -1 });
    expect(settingsStore.state).toBe(accepted);
    expect(appStore.state.toasts).toHaveLength(0);
  });

  it('invalidates comments after asynchronous classification', () => {
    const revision = appStore.state.commentsRevision;
    handleCommentsClassified({ pr_id: 'github:acme/api#1' });
    expect(appStore.state.commentsRevision).toBe(revision + 1);
  });

  it('uses the expertise message or code in both the toast and status', () => {
    handleExpertiseError({
      repo_id: 'repo-1',
      code: 'INDEX_ERROR',
      message: 'index failed',
    });
    expect(appStore.state.expertiseRepoStatuses['repo-1']).toMatchObject({
      status: 'error',
      code: 'INDEX_ERROR',
      message: 'index failed',
    });
    expect(appStore.state.toasts.at(-1)?.message).toBe(
      'Expertise indexing failed: index failed',
    );

    handleExpertiseError({
      repo_id: 'repo-1',
      code: 'INDEX_ERROR',
      message: '',
    });
    expect(appStore.state.expertiseRepoStatuses['repo-1']).toMatchObject({
      status: 'error',
      code: 'INDEX_ERROR',
      message: 'INDEX_ERROR',
    });
    expect(appStore.state.toasts.at(-1)?.message).toBe(
      'Expertise indexing failed: INDEX_ERROR',
    );
  });

  it('leaves pending PR review feedback untouched for thread completions', () => {
    appStore.setState((state) => ({
      ...state,
      pendingAction: { type: 'approve', prId: 'other-pr', startedAt: 1 },
    }));
    const before = appStore.state;
    for (const action of ['resolve_thread', 'unresolve_thread']) {
      handleActionComplete({ pr: 'pr-1', action, success: true, error: '' });
      handleActionComplete({
        pr: 'pr-1',
        action,
        success: false,
        error: 'failed',
      });
      expect(appStore.state).toBe(before);
    }
  });

  it('rejects an action outside the renderer action vocabulary', () => {
    handleActionComplete({
      pr: 'pr-1',
      action: 'unsupported',
      success: true,
      error: '',
    });
    expect(appStore.state.actionResult).toBeNull();
    expect(appStore.state.toasts.at(-1)?.message).toBe(
      'Unknown completed review action',
    );
  });

  it('distinguishes successful and failed action completion', () => {
    handleActionComplete({
      pr: 'pr-1',
      action: 'approve',
      success: true,
      error: '',
    });
    expect(appStore.state.actionResult).toMatchObject({
      prId: 'pr-1',
      type: 'approve',
      success: true,
    });
    expect(appStore.state.toasts).toHaveLength(0);
    handleActionComplete({
      pr: 'pr-1',
      action: 'approve',
      success: false,
      error: 'action failed',
    });
    expect(appStore.state.actionResult).toMatchObject({
      prId: 'pr-1',
      type: 'approve',
      success: false,
      message: 'action failed',
    });
    expect(appStore.state.pendingAction).toBeNull();
    expect(appStore.state.toasts.at(-1)?.type).toBe('error');
  });
});
