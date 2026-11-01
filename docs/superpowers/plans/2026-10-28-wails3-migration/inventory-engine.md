# Engine inventory: WS messages → service methods, pushes → events

This is a snapshot of `go/internal/server` and `go/internal/server/handler` at `c40d9ad`. After Task 1, these paths are under `internal/engine/`. Old registration: `handlers.go` (`registerAuthHandlers`, `registerReviewHandlers`, `registerDiffAndCheckoutHandlers`), `expertise.go` (`registerExpertiseHandlers`) and `repo_location.go` (`registerRepoLocationHandlers`). Verify against the source; the tables are a map, not the truth.

## Old wire behaviour worth knowing

- **Handler shape.** `MessageHandler func(ctx, InboundMessage) (any, error)` returns a hand-built `{"id","type","data"}` map. A returned Go error was only logged (`ws/server.go`, "dispatch error"), so the client timed out. Typed errors fix that.
- **ID lowercasing.** `ws.Dispatch` lowercased `github:` IDs in `repo_id`, `pr_id` and `pr_ids` at any depth (`lowercaseGitHubIdentities` / `lowercaseGitHubID`). Services do this with `NormalizeID`.
- **Async replies.** `AUTH_START`, `REQUEST_REVIEW_QUEUE` and `SCAN_DEV_FOLDER` replied asynchronously through Broadcast, with join funcs in `Run`'s defer. They all become blocking methods.
- **Broadcast shapes.** `AuthDeps.Broadcast func(msg interface{})`, `ReviewQueueDeps.Broadcast func(any)`, `ScanDevFolderDeps.Broadcast func(any)`, the interface `broadcaster{Broadcast(any)}` (`DraftDeps.Broadcaster`, and the `bc` param of `NewMarkPRsReadHandler`), plus direct `a.wsServer.Broadcast` calls in `bridge.go`, `syncloop.go`, `expertise.go` and `llm.go` (`TierConfig.Emit`, `Indexer.WithEmitter`). All of these become `services.Emitter`.

## Method map

"→ err" lists the old error replies that become `ServiceError` (`Code`/`Reason`).

| Old type | File / constructor | New method | Request → Response |
|---|---|---|---|
| AUTH_START (+AUTH_CANCEL) | auth.go `NewAuthFlowHandlers` | `Auth.Start` | `AuthStartRequest{provider}` → `AuthIdentity{provider,username}`; emits `auth:redirect{provider,url}` |
| AUTH_CHECK | `NewAuthCheckHandler` | `Auth.Check` | – → `AuthStatus{authenticated,providers[{provider,username}],auth_status,auth_reason,requires_auth,exchange}` |
| AUTH_DISCONNECT | `NewAuthDisconnectHandler` | `Auth.Disconnect` | `AuthDisconnectRequest{provider}` → `AuthStatus` |
| REQUEST_PR_LIST | pr.go `NewRequestPRListHandlerWithExpertise` | `PR.List` | `PRListRequest{filter,limit,offset}` (defaults: limit 100, max 200) → `PRListResponse{prs,count,has_more,viewer_login}` |
| REQUEST_PR_SYNC | sync.go `NewRequestPRSyncHandler` | `PR.Sync` | `PRRef` → `PRRef` |
| FORCE_SYNC | `NewForceSyncHandler` | `Sync.Force` | `ForceSyncRequest{repo_id}` → `SyncTriggered{repo_id,queued}` |
| SET_SYNC_SETTINGS | `NewSetSyncSettingsHandler` | *(engine)* `App.ApplySyncSettings(autoSync bool, interval time.Duration)` | not exposed |
| REQUEST_REPO_LIST | repo.go `NewRequestRepoListHandler` | `Repos.List` | `RepoListRequest{cache_only}` → `RepoListResponse{repos}`; → err "not authenticated" = `unauthenticated` |
| TOGGLE_WATCH_REPO | `NewToggleWatchHandler` | `Repos.SetWatched` | `SetWatchedRequest{repo_id,watched *bool}` → `WatchResult{repo_id,watched}` |
| REQUEST_REVIEW_QUEUE | review_queue.go | `ReviewQueue.Get` | – → `provider.ReviewQueue`; 30 s internal timeout |
| REQUEST_DIFF | diff.go `NewRequestDiffHandlerWithDeps` | `Diff.Get` | `PRRef` → `DiffResponse{pr_id,diff,files,additions,deletions}` |
| GET_PATCHSETS | patchsets.go | `Patchsets.List` | `PRRef` → `PatchsetsResponse{pr_id,patchsets}` |
| REQUEST_TRIAGE | triage.go | `Triage.Get` | `TriageRequest{pr_id,from_patchset,to_patchset}` → `TriageResponse` (keeps state/reason) |
| REQUEST_INTERDIFF | interdiff.go | `Interdiff.Get` | `InterdiffRequest{pr_id,path,from_patchset,to_patchset}` → `InterdiffResponse` (keeps available/reason) |
| MARK_FILE_VIEWED | `NewMarkFileViewedHandlerWithDeps` | `Viewed.MarkFile` | `MarkFileViewedRequest{pr_id,file_path,content_hash}` → `FileViewed{pr_id,file_path}`; `ok:false reason` → `invalid`+Reason |
| MARK_PRS_READ | readstate.go | `ReadState.MarkRead` | `MarkReadRequest{pr_ids}` (≤500) → `MarkReadResult{marked}`; emits `prs:read-changed` |
| REQUEST_COMMENTS | comments.go `NewRequestCommentsHandlerWithLLMKick` | `Comments.List` | `PRRef` → `CommentsResponse{pr_id,comments}` |
| DISMISS_COMMENTS | dismiss.go | `Dismiss.Dismiss` | `DismissRequest{pr_id,comment_ids}` → `DismissResult{pr_id,batch_id,dismissed_ids,resolved_thread_ids}` |
| UNDISMISS_COMMENTS | dismiss.go | `Dismiss.Undismiss` | `UndismissRequest{comment_ids,unresolve_thread_ids}` → `UndismissResult{undismissed_ids,unresolved_thread_ids}` |
| GET_REVIEW_THREADS | threads.go | `Threads.List` | `PRRef` → `ThreadsResponse{pr_id,threads}` |
| SUBMIT_ACTION | threads.go `NewSubmitActionHandler` | `Threads.SubmitAction` | `ActionRequest{action,pr_id,body,thread_id,comment_id,in_reply_to,line,review_event}` → `ActionResult{pr_id,action}`; `success:false` → `internal`, Reason `action_failed` |
| DRAFTS_LIST | drafts.go | `Drafts.List` | `PRRef` → `DraftsResponse{pr_id,drafts}` |
| DRAFT_SAVE | drafts.go | `Drafts.Save` | `DraftSaveRequest{pr_id,id,path,side,start_line,line,body,commit_sha}` → `DraftSaved{draft}`; emits `drafts:changed` |
| DRAFT_DELETE | drafts.go | `Drafts.Delete` | `DraftDeleteRequest{pr_id,draft_ids}` → `DraftsDeleted{pr_id,deleted}`; emits `drafts:changed` |
| CHECK_PR_CHECKOUT | editor.go | `Editor.CheckoutStatus` | `PRRef` → `CheckoutStatus{pr_id,local_path,head_sha,head_matches}` |
| CHECKOUT_PR_HEAD | editor.go | `Editor.CheckoutHead` | `CheckoutHeadRequest{pr_id,expected_head_sha}` → `CheckoutResult{pr_id,local_path}`; `ok:false reason` → `invalid`+Reason |
| SET_CLASSIFICATION_SETTINGS | classification_settings.go | `Classification.SetSettings` | `ClassificationSettings{mode,dismissal_scope,provider,model,endpoint}` → same |
| SET_BYOK_KEY | classification_settings.go | `Classification.SetByokKey` | `ByokKeyRequest{provider,key}` → `ByokKeyStatus{provider,present}` |
| DELETE_BYOK_KEY | classification_settings.go | `Classification.DeleteByokKey` | `ByokProviderRequest{provider}` → `ByokKeyStatus` |
| EXPERTISE_ENABLE | expertise.go | `Expertise.Enable` | `RepoRef` → error only; codes `repo_not_local`/`git_unavailable` → `invalid`+Reason |
| EXPERTISE_DISABLE | expertise.go | `Expertise.Disable` | `RepoRef` → error only |
| GET_EXPERTISE | expertise.go | `Expertise.Get` | `PRRef` → `ExpertiseData{pr_id,repo_id,status,reason,files}` |
| EXPERTISE_NUDGE_DISMISS | expertise.go | `Expertise.DismissNudge` | `RepoRef` → error only |
| GET_EXPERTISE_SETTINGS | expertise.go | `Expertise.GetSettings` | – → `ExpertiseSettings{alias_emails,min_surviving_lines,excludes}` |
| SET_EXPERTISE_SETTINGS | expertise.go | `Expertise.SetSettings` | `ExpertiseSettings` → `ExpertiseSettings` |
| VALIDATE_DEV_FOLDER | repolocation.go | `DevFolderScan.Validate` | `ValidateDevFolderRequest{path}` → `ValidateDevFolderResult{canonical_path}` |
| SCAN_DEV_FOLDER (+SCAN_CANCEL) | dev_folder_scan.go `NewScanDevFolderHandlers` | `DevFolderScan.Scan` | `ScanDevFolderRequest{path,reason}` → `ScanDevFolderResult{discovered,healthy,degraded,skipped,partial}`; cancelled by ctx or by a newer Scan |
| ADD_REPO_BY_URL | repolocation_add.go | `RepoLocation.AddByURL` | `AddRepoByURLRequest{url,destination *string,dev_folder_path *string}` → `AddRepoResult{repo_id,source}` |
| REMOVE_REPO_PREFLIGHT | repolocation_remove.go | `RepoLocation.RemovePreflight` | `RepoRef` → `RepoCounts{prs,comments,threads,patchsets,actions}` |
| REMOVE_REPO | repolocation_remove.go | `RepoLocation.Remove` | `RepoRef` → `RepoCounts` (old `deleted`) |
| GET_CACHE_STATS | repolocation_cache.go | `RepoLocation.CacheStats` | – → `CacheStats{total_bytes,active_bytes,stale_bytes,repos}` |
| PRUNE_CACHE / CLEAR_CACHE | repolocation_cache.go | `RepoLocation.PruneCache` / `ClearCache` | – → `CacheCleanup{removed,failed,orphans_removed}` |
| DELETE_CACHE_REPO | repolocation_cache.go | `RepoLocation.DeleteCacheRepo` | `RepoRef` → error only |
| RECOVER_REPO_PATH | repolocation_clone.go | `RepoLocation.RecoverPath` | `RecoverPathRequest{repo_id,path}` → error only |
| RECOVER_REPO_TO_CACHE | repolocation_clone.go | `RepoLocation.RecoverToCache` | `RepoRef` → error only |

**Repo-location error codes → `Code`** (`Reason` = the old code):
- `invalid`: `invalid_url`, `destination_required`, `invalid_path`, `destination_conflict`, `remote_mismatch`, `path_missing`
- `unauthenticated`: `auth_failed`
- `cancelled`: `cancelled`
- `internal`: everything else (`clone_failed`, `delete_failed`, `scan_failed`, `persistence_failed`, `permission_denied`)

**Auth error strings → `Code`/`Reason`:**
- `invalid`: unsupported provider, or `auth_already_in_progress`
- `internal`, Reason `client_id_missing`: `GITHUB_CLIENT_ID not configured`
- `internal`, Reason `callback_unavailable`: `authentication callback unavailable`
- `unauthenticated`, Reason `authentication_failed`: `authentication failed`
- `timeout`: `timeout`
- `cancelled`: `cancelled`
- `internal`, Reason `persist_failed`: `failed to persist token`

## Events

Every event is declared in `internal/services/events.go`. Payload json tags match the old `data` keys.

| Name (const) | Payload | Emitted by (old key) | Task |
|---|---|---|---|
| `auth:redirect` (`EventAuthRedirect`) | `AuthRedirect{provider,url}` | `Auth.Start` (AUTH_REDIRECT) | 3 |
| `prs:read-changed` (`EventPRsReadChanged`) | `PRsReadChanged{pr_ids}` | `ReadState.MarkRead` (PRS_READ_CHANGED) | 5 |
| `drafts:changed` (`EventDraftsChanged`) | `DraftsChanged{pr_id}` | `Drafts.Save`/`Delete`, prsync/review.go (DRAFTS_CHANGED) | 6 |
| `sync:complete` (`EventSyncComplete`) | `SyncComplete{scope,repo_id}` (both omitempty) | syncloop.go (SYNC_COMPLETE) | 9 |
| `sync:error` (`EventSyncError`) | `SyncError{error_code,message}` | syncloop.go (SYNC_ERROR) | 9 |
| `sync:rate-limited` (`EventRateLimited`) | `RateLimited{reset_in float64}` | orchestrator (rate_limited) | 9 |
| `pr:metadata-changed` (`EventPRMetadataChanged`) | `PRMetadataChanged{pr_id,owner,repo,number,field,old,new}` (old/new as `string`) | prsync/writer (PR_METADATA_CHANGED) | 9 |
| `pr:ci-status-changed` (`EventCIStatusChanged`) | `CIStatusChanged{pr_id,owner,repo,number,old_status,new_status}` | prsync/writer (CI_STATUS_CHANGED) | 9 |
| `pr:approved` (`EventPRApproved`) | `PRApproved{pr_id}` | actions/executor (pr_approved) | 9 |
| `comment:posted` (`EventCommentPosted`) | `CommentPosted{pr_id,thread_id,in_reply_to}` | prsync/actions (COMMENT_POSTED) | 9 |
| `thread:state-changed` (`EventThreadStateChanged`) | `ThreadStateChanged{thread_id,pr_id,resolved}` | prsync/actions (THREAD_RESOLVED / THREAD_REOPENED) | 9 |
| `action:complete` (`EventActionComplete`) | `ActionComplete{pr,action,success,error}` | actions/executor (action_complete) | 9 |
| `expertise:progress` (`EventExpertiseProgress`) | `ExpertiseProgress{repo_id,files_done,files_total}` | expertise indexer | 9 |
| `expertise:ready` (`EventExpertiseReady`) | `ExpertiseReady{repo_id,indexed_sha,files_total,skipped_files}` | expertise indexer | 9 |
| `expertise:error` (`EventExpertiseError`) | `ExpertiseError{repo_id,code,message}` | expertise indexer | 9 |
| `expertise:nudge` (`EventExpertiseNudge`) | `ExpertiseNudge{repo_id,reviewed_count}` | engine/expertise.go `broadcastExpertiseNudge` | 9 |
| `comments:classified` (`EventCommentsClassified`) | `CommentsClassified{pr_id}` | classify/llm tier (COMMENTS_READY push) | 9 |
| `byok:auth-failed` (`EventByokAuthFailed`) | `ByokAuthFailed{}` | llm tier (BYOK_AUTH_FAILED) | 9 |
| `byok:usage` (`EventByokUsage`) | `ByokUsage{session_tokens,est_cost}` | llm tier (BYOK_USAGE) | 9 |
| `settings:changed` (`EventSettingsChanged`) | `shell.Settings` | settings store change listener | 10 |

These orchestrator event types are **not bridged** because nothing consumes them: `sync_started`, `sync_progress`, `sync_complete`, `sync_error`, `changes_requested`. The bridge ignores them. Orchestrator event sources: `prsync/orchestrator.go` (sync_*, rate_limited), `prsync/writer/writer.go:166`, `prsync/actions.go:202,232`, `prsync/review.go:232`, `actions/executor.go:263-289`. The event channel buffers 10 events and drops when full; that behaviour is unchanged.

## Lifecycle today (`internal/engine/app.go` after Task 1)

- **`New(wsPort, dbPath)`:** creates the provider cache at `<UserCacheDir>/bistro/providers.json`, `keychain.NewTokenManager()`, `syncTrigger` (chan of 8) and `syncSettings` (chan of 4), with a 5 m interval.
- **`Run(ctx)`:**
  1. `openDatabase` (WAL, busy 5 s, 4 conns, migrations)
  2. `prepareStores`
  3. `prepareExpertise`
  4. `prepareLLMTier`
  5. `initializeStartupProvider` (orchestrator, event bridge, optional replay)
  6. `register*` handlers
  7. `wsServer.Start`
  8. `emitReady`
  9. `go runSyncLoop`
  10. block on signal or ctx
- **`cmd/bistro-sync/main.go`:**
  - Flags: `-ws-port 8080 -db-path ./data/sync.db -replay-queue true -log-level info -log-file "" -sync-interval 5m -auto-sync true -auth-callback-timeout 5m -exit-on-stdin-close -rules-dir ""`.
  - Setup: logging, `MkdirAll` of the db dir, `flock(dbPath+".lock")`, `SetProvider` from the keychain token, stdin watchdog.
  - Env: `GITHUB_CLIENT_ID` (falls back to the embedded `-X main.embeddedGitHubClientID`), `GITHUB_CLIENT_SECRET`, `BISTRO_REPO_DIR`, `GITHUB_TOKEN` (git fallback).
- **OAuth callback:** `internal/auth/callback.go`, `localhost:19284/callback`; the listener is opened in handler/auth.go. It stays, because it is a redirect target, not an API.
- **Tests that die with WS:** `internal/ws/*_test.go`, `engine/app_repolocation_test.go` (gorilla dial), `wsaddr_test.go`, `ready_test.go`, and the real-server test in `handler/dev_folder_scan_test.go` (~line 50). `bridge_test.go`, `blockers_test.go` and `syncloop_test.go` use `ws.NewServerWithLogger` only as a sink, so switch them to `servicestest.Recorder`. Port the lowercase cases from `ws/dispatch_test.go` to `services/ids_test.go`.
