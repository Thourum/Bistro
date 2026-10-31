# Renderer inventory: WS call sites, event handlers, `window.electron`

This is a snapshot at `c40d9ad`. Paths are relative to `electron/src/renderer/`, which becomes `frontend/src/` after Task 13. "R" means `wsClient.request` and "S" means `wsClient.send` (fire-and-forget; the caller relied on a push handler to write `appStore`). Verify against the source.

## Transport to delete (Task 18)

- **`services/ws/connection.ts`:** `wsClient{connect, disconnect, send, request(type,data,{onSent})}`.
  - Request ids are `req-${Date.now()}-${Math.random()}`.
  - Timeouts: 5 s by default. `REQUEST_TIMEOUTS`: `REQUEST_PR_SYNC` 135 s; `REQUEST_REPO_LIST` and `REQUEST_REVIEW_QUEUE` 35 s; `SCAN_DEV_FOLDER` 45 s; `CHECKOUT_PR_HEAD` 60 s.
  - `data.error` becomes `EXTERNAL_ERROR`.
  - On open it sends `AUTH_CANCEL` (if `authInterrupted`), then `AUTH_CHECK`, then the startup `SCAN_DEV_FOLDER {reason:'startup'}`.
  - It writes `syncStatus` `connecting`/`connected`/`disconnected`/`error`, `isWsOpen`, and the auth reset fields.
- **The rest of the WS layer:** `hooks/useWebSocketConnection.ts` (only `App.tsx` `Bootstrap` uses it), `types/wsSchemas.ts` (+ test), `parseResponse` in `utils/error.ts`, and `services/ws/events/context.ts` (`resolveWsResponse`).
- **Bench guard:** `guardBenchWsMessage` (`utils/benchSafety.ts`) with the allow-list in `shared/benchSafety.ts`. This is replaced by `callWrite` (README decision 10).

## Startup behaviour to keep

The old WS `onOpen` did three things. The new `Bootstrap` effect (Task 16) must do them once at mount:
1. `Auth.Check` → `handleAuthCheckResult`.
2. A startup dev-folder scan when `settings.repositories.devFolderPath` is set (`DevFolderScan.Scan {path, reason:'startup'}`).
3. Set `syncStatus:'connected'`.

## Call sites by domain

| Domain | Site | Old message | New call |
|---|---|---|---|
| auth | `services/signIn.ts:29` S | AUTH_START (id stored as `authStartRequestId`) | `Auth.Start` (keep the promise; cancel = `promise.cancel()`) |
| auth | `services/signIn.ts:66,79` S | AUTH_CANCEL | cancel the stored `Auth.Start` promise |
| auth | `pages/settings/ConnectionsSection.tsx:17` S | AUTH_DISCONNECT | `Auth.Disconnect` → `handleAuthDisconnectResult` |
| pr | `hooks/useWebSocketPRList.ts:92` R | REQUEST_PR_LIST (pages until `has_more` is false) | `PR.List` |
| pr | `hooks/usePRHydration.ts:49` R | REQUEST_PR_SYNC (135 s) | `PR.Sync` with `{ timeoutMs: 135_000 }` |
| pr | `pages/usePRDetailData.ts:95` R | REQUEST_CI_CHECKS (**no Go handler**) | delete the call and `ciChecksResponseSchema` |
| diff | `pages/usePRDetailData.ts:72`, `pages/useDiffPageData.ts:71` R | REQUEST_DIFF | `Diff.Get` |
| interdiff | `hooks/useInterdiff.ts:52` R | REQUEST_INTERDIFF | `Interdiff.Get` |
| triage | `hooks/useTriage.ts:60` R | REQUEST_TRIAGE | `Triage.Get` |
| patchsets | `hooks/usePatchsets.ts:42` R | GET_PATCHSETS | `Patchsets.List` |
| comments | `pages/usePRDetailData.ts:58`, `pages/useDiffPageData.ts:108` R | REQUEST_COMMENTS | `Comments.List` |
| actions | `services/prActions.ts:59` R | SUBMIT_ACTION | `callWrite(Threads.SubmitAction)` |
| threads | `hooks/useReviewThreads.ts:53` R | GET_REVIEW_THREADS | `Threads.List` |
| threads | `hooks/useThreadActions.ts:52` R | SUBMIT_ACTION resolve/unresolve | `callWrite(Threads.SubmitAction)` |
| drafts | `services/reviewDrafts.ts:38,56,77` R | DRAFTS_LIST / DRAFT_SAVE / DRAFT_DELETE | `Drafts.List` / `callWrite(Drafts.Save)` / `callWrite(Drafts.Delete)` |
| read | `services/readState.ts:18` R | MARK_PRS_READ | `callWrite(ReadState.MarkRead)` |
| viewed | `services/readState.ts:50` R | MARK_FILE_VIEWED | `callWrite(Viewed.MarkFile)` |
| dismiss | `services/dismissals.ts:42,80` R | DISMISS_COMMENTS / UNDISMISS_COMMENTS | `callWrite(Dismiss.Dismiss / Undismiss)` |
| queue | `services/onboardingSync.ts:57` R | REQUEST_REVIEW_QUEUE (35 s) | `ReviewQueue.Get` with `{ timeoutMs: 35_000 }` |
| sync | `components/CommandPalette.tsx:282` S | FORCE_SYNC | `callWrite(Sync.Force)` → `handleSyncTriggered` |
| repos | `components/CommandPalette.tsx:121`, `components/RepoPicker.tsx:29` S; `components/PRListTable.tsx:384` R | REQUEST_REPO_LIST | new `services/repos.ts` `loadRepos()`: `Repos.List` → `handleRepoListReady` logic (writes `repos`, `reposLoading`, error toast unless onboarding) |
| repos | `services/onboardingSync.ts:47` R | REQUEST_REPO_LIST `{cache_only:true}` | `Repos.List({cache_only:true})` |
| repos | `components/RepoPicker.tsx:54` S, `services/onboardingSync.ts:110` R | TOGGLE_WATCH_REPO | `callWrite(Repos.SetWatched)` → `handleWatchRepoResult` logic |
| location | `components/AddRepoDialog.tsx:230` | ADD_REPO_BY_URL (hand-written `parseAddRepoResponse`) | `callWrite(RepoLocation.AddByURL)`; branch on `fault` reason |
| location | `components/CacheUsageCard.tsx:275,298` | GET_CACHE_STATS, PRUNE/CLEAR/DELETE_CACHE_REPO | `RepoLocation.CacheStats`, `callWrite(PruneCache/ClearCache/DeleteCacheRepo)` |
| location | `components/RepoRecoveryDialog.tsx:150-258` | RECOVER_REPO_PATH, RECOVER_REPO_TO_CACHE, REMOVE_REPO_PREFLIGHT, REMOVE_REPO | `RepoLocation.*` |
| scan | `pages/settings/RepositoriesSection.tsx:167,191`, `services/onboardingSync.ts:75,93,99` | SCAN_DEV_FOLDER (+`onSent`), VALIDATE_DEV_FOLDER, SCAN_CANCEL | `DevFolderScan.Scan` (no timeout; cancel the promise), `DevFolderScan.Validate` |
| expertise | `hooks/useExpertise.ts:50` | GET_EXPERTISE | `Expertise.Get` |
| expertise | `components/ExpertiseNudge.tsx:31,49`, `components/CommandPalette.tsx:162` | EXPERTISE_ENABLE / DISABLE / NUDGE_DISMISS | `callWrite(Expertise.*)`; on `fault` reason, run `handleExpertiseError` logic |
| expertise | `pages/settings/ExpertiseSection.tsx:79,135` | GET/SET_EXPERTISE_SETTINGS | `Expertise.GetSettings` / `callWrite(SetSettings)` → write `expertiseSettings` |
| classify | `pages/settings/ClassificationProvider.tsx:48` | SET_BYOK_KEY | `callWrite(Classification.SetByokKey)` |
| classify | `bench/keys.ts:378` | SET_CLASSIFICATION_SETTINGS rules-only | `call(Classification.SetSettings)` (allowed in bench) |
| editor | `services/editor.ts:32,45` | CHECK_PR_CHECKOUT, CHECKOUT_PR_HEAD (60 s) | `Editor.CheckoutStatus`, `callWrite(Editor.CheckoutHead, …, {timeoutMs:60_000})` |

## Old event table (`services/ws/events/index.ts`)

- **Pure `resolveWsResponse` (34 keys).** They disappear because they are now return values: EXPERTISE_DATA, EXPERTISE_*_ACCEPTED, EXPERTISE_NUDGE_DISMISSED, PR_SYNCED, PRS_READ_ACK, DIFF_READY, PATCHSETS_READY, TRIAGE_READY, INTERDIFF_READY, FILE_VIEWED_ACK, REVIEW_THREADS_READY, ACTION_RESULT, DRAFTS_READY, DRAFT_SAVED, DRAFTS_DELETED, PR_CHECKOUT_STATUS, PR_CHECKOUT_RESULT, VALIDATE_DEV_FOLDER_RESULT, ADD_REPO_BY_URL_RESULT, the cache/recover/remove `*_RESULT`s, SET_EXPERTISE_SETTINGS (dead), CLASSIFICATION_SETTINGS_UPDATED, BYOK_KEY_UPDATED, DISMISS_RESULT, UNDISMISS_RESULT, PR_LIST_READY, REVIEW_QUEUE_READY.
- **Hybrids (side effect + resolve).** The side effect moves to the call site's success/err branch:
  - SCAN_DEV_FOLDER_RESULT (warn)
  - REPO_LIST_READY (writes repos)
  - WATCH_REPO_RESULT (patches watched)
  - EXPERTISE_ERROR (statuses + toast)
  - EXPERTISE_SETTINGS / _UPDATED (writes store)
  - COMMENTS_READY: the pending-request half is gone; the push half becomes `comments:classified` → `commentsRevision++`
- **Pure pushes.** Each becomes a handler keyed by its new event name:
  - `sync.ts`: SYNC_COMPLETE → `sync:complete`; SYNC_ERROR → `sync:error`; RATE_LIMITED → `sync:rate-limited`. SYNC_TRIGGERED becomes `Sync.Force`'s return value.
  - `review.ts`: PR_APPROVED → `pr:approved`; THREAD_RESOLVED/REOPENED → `thread:state-changed`; PR_METADATA_CHANGED → `pr:metadata-changed`; PRS_READ_CHANGED → `prs:read-changed`; CI_STATUS_CHANGED → `pr:ci-status-changed`; DRAFTS_CHANGED → `drafts:changed`.
  - `comments.ts`: COMMENT_POSTED → `comment:posted`.
  - `actions.ts`: ACTION_COMPLETE/ACTION_FAILED → `action:complete` (branch on `success`).
  - `expertise.ts`: EXPERTISE_INDEX_PROGRESS → `expertise:progress`; EXPERTISE_READY → `expertise:ready`; EXPERTISE_ERROR (unprompted) → `expertise:error`; EXPERTISE_NUDGE → `expertise:nudge`.
  - `classification.ts`: BYOK_USAGE → `byok:usage`; BYOK_AUTH_FAILED → `byok:auth-failed`.
  - `auth.ts`: AUTH_REDIRECT → `auth:redirect` (opens the URL; no id matching). AUTH_SUCCESS and AUTH_FAILED become `Auth.Start` resolve/reject. AUTH_CHECK_RESULT and AUTH_DISCONNECT_RESULT become return values. AUTH_TOKEN_EXPIRED is deleted (no emitter).
  - `repos.ts`: LOCATION_ERROR is deleted (no emitter).
  - New: `settings:changed` → `settingsStore.setState`.
- **`events.test.ts`.** It tested resolve routing. Keep only the cases that are still behaviour: COMMENTS_READY invalidation as `comments:classified`, and `expertise:error` mapping.

## Types

- **Engine-data zod schemas, all deleted:** `types/wsSchemas.ts` (whole file) and, in `types/index.ts`: `expertiseDataSchema`, `commentSchema`, `commentsResponseSchema`, `diffFileSchema`, `diffResponseSchema`, `patchsetSchema`, `triageFileStatusSchema`, `triageResultSchema`, `interdiffResultSchema`, `fileViewedAckSchema`, `prsReadAckSchema`, `reviewThreadSchema`, `reviewDraftSchema`, `ciCheckSchema`, `repoInfoSchema`, `repoListResponseSchema`, `reviewQueueSchema`.
- **Local engine-data schemas, also deleted:** `useWebSocketPRList.ts` `prSchema`/`prListReadySchema`, `usePatchsets.ts`, `useReviewThreads.ts`, `usePRDetailData.ts` `ciChecksResponseSchema`, `pages/onboardingResults.ts`, `reviewDrafts.ts`, `prActions.ts`, `dismissals.ts`, `editor.ts` (`checkPRCheckoutSchema`, `checkoutPRHeadSchema`), and the hand-written guards in `CacheUsageCard.tsx`, `AddRepoDialog.tsx` and `RepoRecoveryDialog.tsx`.
- **External input, kept:** `router.tsx` search schemas, `bench/*` collector schemas, `hooks/useDismissals.ts` `dismissBotsDetailSchema` (DOM event), and `store/settings.ts` `legacySettingsSchema` (localStorage is external).
- **Most-imported engine types.** Keep these names as re-exports of generated models in `types/index.ts`: `PR` (15 imports), `Comment` (13), `ReviewThread` (9), `RepoInfo` (9), `ReviewDraft` (9), `ExpertiseFileData`, `Patchset`, `DiffFile`, `TriageResult`, `InterdiffResult`, `ReviewQueue`. UI-only types stay as they are: `PendingAction`, `ActionResult`, `Toast`, `ThreadLifecycle`, `threadLifecycle()`, `CommentSeverity`.

## `window.electron` uses (29 lines, 14 files)

| Site | API | Replacement |
|---|---|---|
| `main.tsx:47`, `sentry.ts:5`, `utils/benchSafety.ts:7` | `isKeysBench` | URL `bench_keys=1` only |
| `bench/index.ts:82,84`, `utils/keybindingActions.ts:375,378` | `quitApp` | `Shell.Quit` |
| `utils/openUrl.ts:13,14` | `openExternal` | `Shell.OpenExternal` |
| `components/RepoRecoveryDialog.tsx:181`, `pages/settings/RepositoriesSection.tsx:137,149`, `services/onboardingSync.ts:66` | `pickFolder` | `Shell.PickFolder` (`""` = cancelled → `null`) |
| `hooks/useSignInScreenData.ts:83` | `platform` | `appStore.platform` from `Shell.AppInfo` |
| `services/editor.ts:62` | `openInEditor` | `Shell.OpenInEditor` |
| `services/engine.ts:53-96` | engine status, restart, log tail, log folder, crash report, app version | file deleted; `Shell.AppInfo` (version) and `Shell.OpenLogFolder` |
| `services/ws/connection.ts:108,133` | `wsPort`, `getWsPort` | file deleted |
| `store/settings.ts:39,163,180,191,238,278,299` | `settings.*`, `on('settings:changed')` | `Shell.GetSettings`/`SetSettings`/`ResetSettings`/`OpenSettingsFile` and the `settings:changed` event |

Engine-status consumers to clean up (decision 7): `components/RootShell.tsx` (`initEngineStatus`), `components/EngineCrashedScreen.tsx`, `components/ConnectingScreen.tsx` (log-tail panel), `hooks/useEngineScreenData.ts`, `hooks/useKeyActionContext.ts`, `hooks/useSignInScreenData.ts`, `pages/LoginPage.tsx`, `store/ui.ts`, and `utils/keybindingActions.ts:324-328`. Also the `connecting.*` entries in `utils/keybindings.ts:180-190` and `utils/keymaps.ts:46-48`.

`syncStatus` consumers (drop `connecting`/`disconnected`; initial becomes `'connected'`):
- `components/AppFooter.tsx` `deriveStatus`
- `components/SyncStatusHoverCard.tsx`
- `pages/OnboardingFooter.tsx` `SYNC_STATUS`
- `store/ui.ts:83,240`

`isWsOpen` is deleted. Its readers are `services/signIn.ts:16,65,78`, `hooks/useKeyActionContext.ts:53`, `hooks/useSignInScreenData.ts:46` and `OnboardingFooter`; treat it as always true.

## Tests that mock `wsClient`

These switch to `vi.mock` of the generated binding modules:
- `hooks/useTriage.test.ts`
- `pages/useDiffPageData.test.tsx`
- `pages/useOnboardingSync.test.ts`
- `services/signIn.test.ts`
- `services/prActions.test.ts`
- `services/reviewDrafts.test.ts`
- `services/onboardingSync.test.ts`

`services/ws/connection.test.ts` and `types/wsSchemas.test.ts` are deleted.
