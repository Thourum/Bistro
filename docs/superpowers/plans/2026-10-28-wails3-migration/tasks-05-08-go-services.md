# Tasks 5–8: Remaining domain services

Read first: [README](README.md) (Global Constraints, decisions, **Service port recipe**) and [inventory-engine.md](inventory-engine.md). Every task here follows the recipe. Struct and json-tag details are in the inventory's method map. Old handler files live in `internal/engine/handler/`.

Tasks 2–4 provide the shared pieces: `guard`, `guardErr`, `Errorf`, `WithReason`, `NormalizeID`, `NormalizeIDs`, `PRRef`, `RepoRef`, `Emitter`, `servicestest.Recorder`, `services.Set` and `(*engine.App).Services()`.

### Task 5: `Diff`, `Patchsets`, `Triage`, `Interdiff`, `Viewed`, `ReadState`

**Files:**
- Create: `internal/services/{diff,patchsets,triage,interdiff,viewed,readstate}.go` plus tests ported from `handler/{diff,patchsets,triage,interdiff,viewed_baseline,readstate}_test.go`
- Delete: those handler files and tests, and their registrations in `registerDiffAndCheckoutHandlers`
- Modify: `internal/services/events.go`, `set.go`, `internal/engine/services.go`

**Interfaces:**
- Produces:
  - `func (s *Diff) Get(ctx, PRRef) (DiffResponse, error)`. `DiffResponse{PRID, Diff, Files []DiffFile, Additions, Deletions}`, and `DiffFile` keeps every old `diffFileDetail` tag: `path,status,old_path,is_binary,is_truncated,diff_status,patch_bytes,hunk_count,additions,deletions,content_hash,viewed`.
  - `func (s *Patchsets) List(ctx, PRRef) (PatchsetsResponse, error)`
  - `func (s *Triage) Get(ctx, TriageRequest) (TriageResponse, error)`
  - `func (s *Interdiff) Get(ctx, InterdiffRequest) (InterdiffResponse, error)`
  - `func (s *Viewed) MarkFile(ctx, MarkFileViewedRequest) (FileViewed, error)`
  - `func (s *ReadState) MarkRead(ctx, MarkReadRequest) (MarkReadResult, error)`
  - Event `EventPRsReadChanged = "prs:read-changed"` with `PRsReadChanged{PRIDs []string \`json:"pr_ids"\`}`
  - `Set.Diff`, `Set.Patchsets`, `Set.Triage`, `Set.Interdiff`, `Set.Viewed`, `Set.ReadState`

- [ ] **Step 1: Port `Diff.Get`.** It is the recipe's worked example. The old `{diff:"", error}` reply becomes `CodeNotFound` when the PR or diff row is missing, and `CodeInternal` otherwise. Keep the viewed-flag join and the current-user lookup.
- [ ] **Step 2: Port `Patchsets.List`, `Triage.Get` and `Interdiff.Get`.**
  - `Triage` keeps `state`, `reason`, `baseline *{source,reviewed_at}`, `files`, `changed_count` and `total_count`. Requests with `0/0` patchsets still mean "since last review".
  - `Interdiff` keeps `available:false` + `reason` as data and keeps the `RepoForPR` / `Interdiff` function deps.
  - These are read paths, so a missing PR → `CodeNotFound`.
- [ ] **Step 3: Port `Viewed.MarkFile`.**
  - Success returns `FileViewed{PRID, FilePath}`.
  - The old `ok:false` + `reason` (for example a stale `content_hash`) becomes `WithReason(CodeInvalid, reason, …)`, with the reason strings unchanged so the renderer can match them.
  - A missing current user → `CodeUnauthenticated`.
  - `viewed_baseline.go` (no WS import) moves along with it as an unexported helper file `viewed_baseline.go` in `services`.
- [ ] **Step 4: Port `ReadState.MarkRead`.**
  - More than 500 IDs → `CodeInvalid`. No current user → `CodeUnauthenticated`.
  - On success, return `MarkReadResult{Marked int \`json:"marked"\`}` and emit `PRsReadChanged{PRIDs: normalized}`.
  - The `broadcaster` param becomes the `Events Emitter` field.
- [ ] **Step 5: Wire and register.** Add the `Set` fields, construct them in `services.go`, register the event in `events.go`, and delete the registrations.
- [ ] **Step 6: Tests.** Port the existing tests. Required coverage:
  - the diff viewed join
  - a diff for a missing PR → `CodeNotFound`
  - triage since-last-review
  - interdiff unavailable reason
  - a stale content hash → `CodeInvalid` with the old reason string
  - mark-read emits exactly one `prs:read-changed` with lowercased IDs
  - 501 IDs → `CodeInvalid`
- [ ] **Step 7: Verify.** `go vet ./... && go test -race ./...`
- [ ] **Step 8: Commit.** `git add -A && git commit -m "feat: port diff, triage, interdiff, viewed and read-state handlers to services"`

### Task 6: `Comments`, `Dismiss`, `Threads`, `Drafts`

**Files:**
- Create: `internal/services/{comments,dismiss,threads,drafts}.go` plus tests ported from `handler/comments*_test.go` (7 files: `comments_test.go`, `comments_anchor_*_test.go`, `comments_dismissed_test.go`, `comments_patchset_test.go`, `comments_refresh_test.go`), `dismiss_test.go`, `threads_test.go`, `drafts_test.go`
- Delete: those handler files and tests, and their registrations in `handlers.go`
- Modify: `events.go`, `set.go`, `engine/services.go`

**Interfaces:**
- Produces:
  - `func (s *Comments) List(ctx, PRRef) (CommentsResponse, error)`. `CommentRow` keeps every json tag of the old `commentRow` (comments.go:30).
  - `func (s *Dismiss) Dismiss(ctx, DismissRequest) (DismissResult, error)`
  - `func (s *Dismiss) Undismiss(ctx, UndismissRequest) (UndismissResult, error)`
  - `func (s *Threads) List(ctx, PRRef) (ThreadsResponse, error)`
  - `func (s *Threads) SubmitAction(ctx, ActionRequest) (ActionResult, error)`
  - `func (s *Drafts) List(ctx, PRRef) (DraftsResponse, error)`
  - `func (s *Drafts) Save(ctx, DraftSaveRequest) (DraftSaved, error)`
  - `func (s *Drafts) Delete(ctx, DraftDeleteRequest) (DraftsDeleted, error)`
  - Event `EventDraftsChanged = "drafts:changed"` with `DraftsChanged{PRID string \`json:"pr_id"\`}`
  - `Set.Comments`, `Set.Dismiss`, `Set.Threads`, `Set.Drafts`

- [ ] **Step 1: Port `Comments.List`.** Keep the LLM kick (`kick func(prID string)`) and the `*classify.EngineProvider`. The kick still runs after a successful read.
- [ ] **Step 2: Port `Dismiss`.** No account → `CodeUnauthenticated`. The old code returned a bare Go error here, so the renderer got nothing; this fixes that. Keep `GetApplier` and `Scope` for provider-side resolution.
- [ ] **Step 3: Port `Threads`.**
  - `List` is a straight port.
  - `SubmitAction` keeps every action (`approve`, `request_changes`, `submit_review`, `post_comment`, `reply_comment`, `resolve_thread`, `unresolve_thread`) and the `ApplierGetter`.
  - The old `success:false` + `error` becomes `WithReason(CodeInternal, "action_failed", errMsg)`. A missing applier/provider → `CodeUnauthenticated`.
  - Success returns `ActionResult{PRID, Action}`.
  - The `approve` / `request_changes` absorption of drafts stays inside `Orchestrator.ApplyAction`. Do not move it.
- [ ] **Step 4: Port `Drafts`.** Save and Delete emit `DraftsChanged{PRID}` after a successful write. The old `{error}` replies become `CodeInvalid` for validation failures and `CodeInternal` otherwise; no current user → `CodeUnauthenticated`. Also switch `prsync/review.go:232`'s orchestrator `DRAFTS_CHANGED` to the same name: leave it emitting through the orchestrator, and Task 9's bridge maps it to `EventDraftsChanged`.
- [ ] **Step 5: Wire and register,** as in the recipe.
- [ ] **Step 6: Tests.** Port the existing ones; the anchor, patchset, dismissed and refresh suites are the valuable coverage. Add, or keep if it exists:
  - Save emits one `drafts:changed` with a lowercased `pr_id`.
  - Delete of unknown IDs returns `deleted: 0` and still emits nothing if nothing changed. Mirror the old behaviour; if the old code emitted anyway, keep that and assert it.
  - `SubmitAction` with a failing applier → Reason `action_failed`.
- [ ] **Step 7: Verify.** `go vet ./... && go test -race ./...`
- [ ] **Step 8: Commit.** `git add -A && git commit -m "feat: port comments, dismiss, threads and drafts handlers to services"`

### Task 7: `RepoLocation`, `DevFolderScan`

**Files:**
- Create: `internal/services/{repolocation,repolocation_add,repolocation_cache,repolocation_clone,repolocation_remove,dev_folder_scan}.go` plus tests ported from `handler/repolocation*_test.go` and `dev_folder_scan_test.go`
- Delete: those handler files and tests; `registerRepoLocationHandlers` in `internal/engine/repo_location.go`; the scan `wait` defer in `Run`
- Modify: `set.go`, `engine/services.go`, `engine/repo_location.go`

**Interfaces:**
- Produces:
  - `func (s *DevFolderScan) Validate(ctx, ValidateDevFolderRequest) (ValidateDevFolderResult, error)`
  - `func (s *DevFolderScan) Scan(ctx, ScanDevFolderRequest) (ScanDevFolderResult, error)`
  - `func (s *RepoLocation) AddByURL(ctx, AddRepoByURLRequest) (AddRepoResult, error)`
  - `RemovePreflight(ctx, RepoRef) (RepoCounts, error)`
  - `Remove(ctx, RepoRef) (RepoCounts, error)`
  - `CacheStats(ctx) (CacheStats, error)`
  - `PruneCache(ctx) (CacheCleanup, error)`
  - `ClearCache(ctx) (CacheCleanup, error)`
  - `DeleteCacheRepo(ctx, RepoRef) error`
  - `RecoverPath(ctx, RecoverPathRequest) error`
  - `RecoverToCache(ctx, RepoRef) error`
  - `func locationError(code, message string) *ServiceError`
  - `Set.RepoLocation`, `Set.DevFolderScan`

- [ ] **Step 1: Add `locationError`** in `repolocation.go`. It uses the inventory's repo-location table, so `Reason` = the old code:
  ```go
  // locationError maps a repo-location failure code onto the closed Code set,
  // keeping the original code as Reason for the UI.
  func locationError(code, message string) *ServiceError {
  	c := CodeInternal
  	switch code {
  	case "invalid_url", "destination_required", "invalid_path", "destination_conflict", "remote_mismatch", "path_missing":
  		c = CodeInvalid
  	case "auth_failed":
  		c = CodeUnauthenticated
  	case "cancelled":
  		c = CodeCancelled
  	}
  	return WithReason(c, code, message)
  }
  ```
  Replace every `LocationErrorPayload` reply with `locationError(code, message)`. Delete `LocationErrorPayload` and `locationResponse` once nothing uses them.
- [ ] **Step 2: Port the RepoLocation methods.** Keep the deps (`Clone`, `CacheRoot`, `Locks *repolock.Locks`, `DeleteCacheDir`, `SweepOrphans`, `Store`, `DB`).
  - `AddByURL`'s clone must honour `ctx`. Check that the `Clone` dep takes ctx (it does: `func(ctx, repoURL, destination string, bare bool) error`). That makes a renderer cancel abort the clone, mapped to `cancelled`.
  - `PruneCache` and `ClearCache` keep partial results: the `failed[{repo_id,code,message}]` entries are data, not an error.
- [ ] **Step 3: Port `DevFolderScan`.**
  - `Validate` returns `{canonical_path}` or `locationError`.
  - `Scan` blocks:
    ```go
    // Scan runs one dev-folder scan. A newer Scan cancels this one; the
    // renderer cancels by cancelling the call's promise (ctx).
    func (s *DevFolderScan) Scan(ctx context.Context, req ScanDevFolderRequest) (ScanDevFolderResult, error) {
    	return guard(s.Panics, "DevFolderScan.Scan", func() (ScanDevFolderResult, error) {
    		ctx, cancel := context.WithCancel(ctx)
    		defer cancel()
    		s.mu.Lock()
    		if s.cancelCurrent != nil {
    			s.cancelCurrent()
    		}
    		s.cancelCurrent = cancel
    		s.mu.Unlock()
    		// ...validate path, run s.Scan(ctx, path), persist via s.Locations, trigger sync...
    	})
    }
    ```
  - When `BISTRO_REPO_DIR` is set, return `ScanDevFolderResult{}` at once, as the old `{ok:true}` did.
  - A scan error → `locationError("scan_failed", …)`. A persistence error → `locationError("persistence_failed", …)`. `ctx.Err()` → let `guard` map it to `cancelled`.
  - Rename the injected scanner field to `ScanFunc` to avoid clashing with the method.
- [ ] **Step 4: Wire them.** Delete `registerRepoLocationHandlers`, the scan wait func, and its `Run` defer.
- [ ] **Step 5: Tests.** Port the existing ones, and delete the real-WS-server test in the old `dev_folder_scan_test.go` (~line 50). New behaviour tests:
  - **Superseded.** Start `Scan` A with a `ScanFunc` that blocks until its ctx is done. Then `Scan` B. A returns `CodeCancelled` and B completes.
  - **Cancelled.** Cancelling the caller's ctx returns `CodeCancelled`.
  - **Reason preserved.** `AddByURL` with `"not a url"` → `CodeInvalid`, Reason `invalid_url`.
- [ ] **Step 6: Verify.** `go vet ./... && go test -race ./...`
- [ ] **Step 7: Commit.** `git add -A && git commit -m "feat: port repo-location and dev-folder scan handlers to services"`

### Task 8: `Expertise`, `Classification`, `Editor`

**Files:**
- Create: `internal/services/{expertise,classification,editor}.go` plus tests ported from `handler/{expertise,classification_settings,editor}_test.go`
- Delete: those handler files and tests; `registerExpertiseHandlers` in `internal/engine/expertise.go`; the remaining registrations in `handlers.go`
- Modify: `set.go`, `engine/services.go`, `engine/app.go` (add `ApplyClassificationSettings`)

**Interfaces:**
- Produces:
  - `Expertise.Enable(ctx, RepoRef) error`
  - `Expertise.Disable(ctx, RepoRef) error`
  - `Expertise.Get(ctx, PRRef) (ExpertiseData, error)`
  - `Expertise.DismissNudge(ctx, RepoRef) error`
  - `Expertise.GetSettings(ctx) (ExpertiseSettings, error)`
  - `Expertise.SetSettings(ctx, ExpertiseSettings) (ExpertiseSettings, error)`
  - `Classification.SetSettings(ctx, ClassificationSettings) (ClassificationSettings, error)`
  - `Classification.SetByokKey(ctx, ByokKeyRequest) (ByokKeyStatus, error)`
  - `Classification.DeleteByokKey(ctx, ByokProviderRequest) (ByokKeyStatus, error)`
  - `Editor.CheckoutStatus(ctx, PRRef) (CheckoutStatus, error)`
  - `Editor.CheckoutHead(ctx, CheckoutHeadRequest) (CheckoutResult, error)`
  - engine: `func (a *App) ApplyClassificationSettings(s services.ClassificationSettings) error`
  - `Set.Expertise`, `Set.Classification`, `Set.Editor`

- [ ] **Step 1: Port `Expertise`.**
  - `Enable` maps `repo_not_local` / `git_unavailable` → `WithReason(CodeInvalid, code, msg)`.
  - `Get` keeps `status:"unavailable"` + `reason` + `files:[]` as data.
  - `SetSettings` returns the stored settings (the old `EXPERTISE_SETTINGS_UPDATED` payload). `ExpertiseSettings.Excludes` keeps its map type.
- [ ] **Step 2: Port `Classification`.**
  - `SetSettings` updates the shared `*ClassificationState` and returns the applied value. `ApplyClassificationSettings` calls the same code path (extract an unexported `applyClassification` used by both). It exists so the shell can push the settings file's `classification` section without going through bindings.
  - `SetByokKey` / `DeleteByokKey` keep `onKeyChanged()` and return `present`.
  - **Never log the key.** Check that every log line in the ported code omits `req.Key`.
- [ ] **Step 3: Port `Editor`.**
  - `CheckoutStatus` keeps `GitCheckoutInspector`.
  - `CheckoutHead` keeps `a.gitClient` and honours `ctx`. The old `ok:false` + `reason` → `WithReason(CodeInvalid, reason, …)`, with the reason strings unchanged.
  - Success returns `CheckoutResult{PRID, LocalPath}`.
- [ ] **Step 4: Wire them.** After this task, `handlers.go` should register nothing. Delete any now-empty `register*` functions but keep `handlers.go` if `Run` still calls `wsServer` setup; Task 9 deletes it. Run `ls internal/engine/handler`: only files Task 9 deletes may remain (shared helpers, `doc.go`). List them in your report.
- [ ] **Step 5: Tests.** Port the existing tests. Required coverage:
  - `Enable` on a non-local repo → Reason `repo_not_local`
  - BYOK set then delete flips `present`
  - checkout with a head mismatch → `CodeInvalid` with the old reason
  - `ApplyClassificationSettings` and `Classification.SetSettings` reach the same state: set via one, read the state, and assert equal
- [ ] **Step 6: Verify.** `go vet ./... && go test -race ./...`
- [ ] **Step 7: Commit.** `git add -A && git commit -m "feat: port expertise, classification and editor handlers to services"`
