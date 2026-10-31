# Tasks 16–18: Port the renderer onto the bindings

Read first:
- [README](README.md), especially decisions 2, 6, 7 and 10;
- [inventory-renderer.md](inventory-renderer.md): the call-site table is the work list;
- [inventory-engine.md](inventory-engine.md): new method names and response shapes;
- the generated `frontend/bindings/github.com/itterno/bistro/internal/services/`.

All paths are under `frontend/src/`.

## Port recipe (every call site)

1. **Replace the call.** Swap `wsClient.request('X', data)` / `wsClient.send('X', data)` for `call(() => Svc.Method(req))`, or `callWrite(...)` for mutations, using `call`/`callWrite` from `services/engine/call`.
   - `send` + push-handler pairs become a request whose `.match(ok, err)` does what the push handler did (the "hybrids" in the inventory).
   - Keep timeouts that the old `REQUEST_TIMEOUTS` set: `PR.Sync` 135 000, `ReviewQueue.Get` and `Repos.List` 35 000, `Editor.CheckoutHead` 60 000. `Auth.Start` and `DevFolderScan.Scan` use `timeoutMs: null`.
2. **Delete the parsing.** Drop the `parseResponse(schema, data)` step and the local zod schema; the result is already typed.
   - Old `data.error` / `ok:false` branches become `err` branches.
   - Where the UI branched on a code string (repo-location codes, expertise codes, auth reasons, viewed reasons), use `faultReason(fault)`.
3. **Keep fault tags only if they matter.** Drop wrapper `mapErr(cause => new Fault(cause).withTag('NETWORK_ERROR'))` lines unless a test or the UI reads that tag; in that case keep it and say why in the commit.
4. **Switch the types.** When a domain's call sites move, replace its zod schema and hand-written interface in `types/index.ts` with a type re-export of the generated model, under the **old name**: `export type { PRRow as PR } from '@bindings/services';`. Fix the type errors that follow at the consumers.
   - Generated nullable fields are `T | null` where the old types had `T | undefined` or `?`. Fix consumers with `?? fallback`, not casts.
5. **Swap the test mocks.** Tests that mocked `wsClient` now mock the bindings:
   ```ts
   const List = vi.hoisted(() => vi.fn());
   vi.mock('@bindings/services', () => ({ Drafts: { List, Save: vi.fn(), Delete: vi.fn() } }));
   List.mockReturnValue(cancellable(Promise.resolve({ pr_id: 'pr-1', drafts: [draft] })));
   ```
   Assert on behaviour (store state, returned Result), not on "was called" alone.
6. **Verify:** `cd frontend && pnpm type-check && pnpm lint && pnpm test`.

### Task 16: Auth, sync, PR list, repos, onboarding, and the bootstrap swap

**Files:**
- Create: `services/repos.ts`, `hooks/useEngine.ts`
- Modify:
  - `App.tsx` (`Bootstrap`: `useWebSocketConnection()` → `useEngine()`)
  - `services/signIn.ts` (+ test), `pages/settings/ConnectionsSection.tsx`, `utils/openUrl.ts`
  - `hooks/useWebSocketPRList.ts` → rename to `hooks/usePRList.ts` and fix its imports; `hooks/usePRHydration.ts`
  - `components/CommandPalette.tsx` (FORCE_SYNC and REQUEST_REPO_LIST only), `components/RepoPicker.tsx`, `components/PRListTable.tsx`
  - `services/onboardingSync.ts` (+ test; review queue, repo list and watch only), `pages/useOnboardingSync.test.ts`
  - `store/ui.ts` (`syncStatus`, `isWsOpen`, auth fields), `components/AppFooter.tsx`, `components/SyncStatusHoverCard.tsx`, `pages/OnboardingFooter.tsx`, `hooks/useKeyActionContext.ts`, `hooks/useSignInScreenData.ts`
  - `types/index.ts` (`PR`, `RepoInfo`, `ReviewQueue`), `store/settings.ts` (`initSettings` only), `components/RootShell.tsx` (drop `initEngineStatus()`)

**Interfaces:**
- Consumes: `call`, `callWrite`, `faultReason`, `subscribeEngineEvents` (Task 15), plus `Auth`, `PR`, `Sync`, `Repos` and `ReviewQueue` from `@bindings/services`.
- Produces:
  - `function useEngine(): void`, which subscribes to events and runs startup once;
  - `function loadRepos(options?: { cacheOnly?: boolean }): ResultAsync<RepoInfo[], Fault>`, which writes `repos`/`reposLoading` and toasts on error unless onboarding;
  - `function setRepoWatched(repoId: string, watched: boolean): ResultAsync<void, Fault>`;
  - `signIn.ts`: `startSignIn(provider)` and `cancelSignIn()`, keeping their current names if they already exist.

- [ ] **Step 1: `hooks/useEngine.ts`.** It replaces `useWebSocketConnection`.
  ```ts
  /** Subscribes to engine push events and runs the startup checks the WS onOpen used to run. */
  export function useEngine(): void {
    useEffect(() => {
      const unsubscribe = subscribeEngineEvents();
      appStore.setState((s) => ({ ...s, syncStatus: 'connected' }));
      void call(() => Auth.Check()).match(handleAuthStatus, handleAuthCheckFailed);
      runStartupDevFolderScan();
      return unsubscribe;
    }, []);
  }
  ```
  - `handleAuthCheckFailed(fault)` logs the fault and sets `authChecked: true`, `isAuthenticated: false`, `syncStatus: 'error'` and `syncError: fault.message`. The `authChecked` gate (`ConnectingScreen`) therefore always finishes, and the router lands on the login page, which already shows `syncError` and offers sign-in. A successful sign-in later runs `Auth.Check` again through the normal success path. Test it in `services/signIn.test.ts`: a rejected `Auth.Check` leaves `authChecked` true and `syncError` set.
  - `handleAuthStatus` is the old `handleAuthCheckResult` logic, moved to `services/signIn.ts` and typed as the generated `AuthStatus`.
  - Until Task 18, `runStartupDevFolderScan` calls `call(() => DevFolderScan.Scan({ path, reason: 'startup' }), { timeoutMs: null })` when `settingsStore.state.repositories.devFolderPath` is set. Task 18 moves it into `services/onboardingSync.ts`. Write it there now if that file already has the scan helpers.
- [ ] **Step 2: Sign-in.**
  - `startSignIn` calls `call(() => Auth.Start({ provider }), { timeoutMs: null, onStart: (p) => { pending = p; } })`. On ok it runs the old `AUTH_SUCCESS` logic. On err it runs the old `AUTH_FAILED` logic, with `faultReason`/tag replacing the old error strings: `CANCELLED` → the cancelled path, which keeps `authInterrupted` semantics only if a test still needs them; otherwise delete `authInterrupted`.
  - `cancelSignIn` calls `pending?.cancel()`.
  - The browser opens through the `auth:redirect` handler (Task 15), which uses `utils/openUrl.ts`. **Port `openUrl.ts` to `callWrite(() => Shell.OpenExternal(url))` in this task** (moved from Task 18), because Electron is already gone and sign-in can't be verified otherwise.
  - Delete `authStartRequestId` from `store/ui.ts`.
  - `ConnectionsSection` calls `callWrite(() => Auth.Disconnect({ provider }))` and runs the old `AUTH_DISCONNECT_RESULT` logic (`handleAuthStatus`) on ok.
- [ ] **Step 3: Connection state.**
  - In `store/ui.ts`, drop the `syncStatus` members `'connecting' | 'disconnected'`; the initial value becomes `'connected'`. Delete `isWsOpen` and every reader: treat it as true, delete `false` branches, and delete OFFLINE states that only `isWsOpen` drove. `isOffline` keeps its `navigator.onLine` meaning if anything still sets it.
  - Update the `deriveStatus` and `SYNC_STATUS` maps.
  - Run `rg "isWsOpen|'connecting'|'disconnected'" src` and expect no hits outside `services/ws/`.
- [ ] **Step 4: PR list and sync.**
  - `usePRList` pages through `PR.List` until `has_more` is false.
  - `usePRHydration` calls `call(() => PR.Sync({ pr_id }), { timeoutMs: 135_000 })`.
  - `CommandPalette` FORCE_SYNC becomes `callWrite(() => Sync.Force({ repo_id: '' }))`. On ok, apply the old `handleSyncTriggered` logic (`queued` → `'syncing'`); this replaces the optimistic set.
- [ ] **Step 5: Repos.**
  - `services/repos.ts` holds `loadRepos` and `setRepoWatched`, carrying over the old `handleRepoListReady` / `handleWatchRepoResult` side effects.
  - `CommandPalette`, `RepoPicker` and `PRListTable` call `loadRepos()`.
  - `onboardingSync` calls `loadRepos({ cacheOnly: true })` and `setRepoWatched`.
  - `ReviewQueue.Get` replaces REQUEST_REVIEW_QUEUE (`timeoutMs: 35_000`).
- [ ] **Step 6: Types.** Switch `PR`, `RepoInfo` and `ReviewQueue` per recipe step 4, and delete `prSchema`, `prListReadySchema`, `repoInfoSchema`, `repoListResponseSchema` and `reviewQueueSchema`.
- [ ] **Step 7: Tests.** Swap the mocks in `services/signIn.test.ts`, `services/onboardingSync.test.ts` and `pages/useOnboardingSync.test.ts`. Required behaviour:
  - a cancelled `Auth.Start` (rejects with `CancelError`) leaves `authLoading` false;
  - `loadRepos` writes `repos` on ok and sets `reposLoading` false on err.
- [ ] **Step 7b: Startup-critical shell pieces** (moved from Task 18 so this task's live check works).
  - `store/settings.ts` `initSettings` reads through `call(() => Shell.GetSettings())`, keeping its current exported signature. Writes stay as they are until Task 18.
  - Delete the `initEngineStatus()` call in `components/RootShell.tsx`. Any route gate that waits on `engineStatus` treats the engine as ready. Task 18 deletes the rest of the engine-status code.
  - `rg -n "window\.electron" src/main.tsx src/App.tsx src/components/RootShell.tsx src/store/settings.ts src/router.tsx` lists the remaining uses. None of them may sit on the startup path; port any that do.
- [ ] **Step 8: Verify.** Run the recipe checks, then `wails3 task dev` (manual): the app opens, the PR list loads after sign-in, and the footer shows the sync state. Report what you saw.
- [ ] **Step 9: Commit.** `git add -A && git commit -m "feat: move auth, sync, PR list and repo calls onto Wails bindings"`

### Task 17: Diff, review, comments, drafts, threads

**Files:**
- Modify:
  - `pages/usePRDetailData.ts` (diff, comments; delete REQUEST_CI_CHECKS and `ciChecksResponseSchema`), `pages/useDiffPageData.ts` (+ test)
  - `hooks/useInterdiff.ts`, `hooks/useTriage.ts` (+ test), `hooks/usePatchsets.ts`, `hooks/useReviewThreads.ts`, `hooks/useThreadActions.ts`
  - `services/prActions.ts` (+ test), `services/reviewDrafts.ts` (+ test), `services/readState.ts`, `services/dismissals.ts`
  - `types/index.ts` (`Comment`, `DiffFile`, `Patchset`, `TriageResult`, `TriageFileStatus`, `InterdiffResult`, `ReviewThread`, `ReviewDraft`, `FileViewedAck`, `PRsReadAck`; delete `CICheck` if nothing else uses it)

**Interfaces:**
- Consumes: `Diff`, `Patchsets`, `Triage`, `Interdiff`, `Viewed`, `ReadState`, `Comments`, `Dismiss`, `Threads` and `Drafts` from `@bindings/services`.
- Produces: the same exported function names and signatures these modules have today, except that results are typed from bindings.

- [ ] **Step 1: Port the read paths.** `Diff.Get`, `Comments.List`, `Patchsets.List`, `Triage.Get` (0/0 = since last review), `Interdiff.Get` and `Threads.List`, per the recipe. `Interdiff` `available:false` + `reason` is still data.
- [ ] **Step 2: Port the writes.**
  - `prActions.ts` → `callWrite(() => Threads.SubmitAction({...}))`. A Go `action_failed` reason arrives as an err; keep the existing user-facing error text path.
  - `useThreadActions` → the same method with `resolve_thread` / `unresolve_thread`.
  - `reviewDrafts.ts` → `Drafts.List` (read), `callWrite(Drafts.Save)` and `callWrite(Drafts.Delete)`.
  - `readState.ts`: `MarkRead`'s fault still maps to `null`. `MarkFile`'s old `ok:false` + `reason` is now `faultReason(fault)`; keep the existing UI branch on the stale reason.
  - `dismissals.ts` → `callWrite(Dismiss.Dismiss/Undismiss)`.
- [ ] **Step 3: Types.** Re-export the generated models under the old names, delete the corresponding zod schemas (inventory list), and fix the consumers.
- [ ] **Step 4: Tests.** Swap the mocks in `pages/useDiffPageData.test.tsx`, `hooks/useTriage.test.ts`, `services/prActions.test.ts` and `services/reviewDrafts.test.ts`. Keep their existing behavioural assertions. One required case: a Go rejection with `{code:'internal', reason:'action_failed', message:'boom'}` on `SubmitAction` surfaces `boom` through the same path the old `success:false` did.
- [ ] **Step 5: Verify.** Run the recipe checks. Manual `wails3 task dev`: open a PR, open a diff, mark a file viewed, write a draft (it reappears after reload), and resolve a thread.
- [ ] **Step 6: Commit.** `git add -A && git commit -m "feat: move diff, review, draft and thread calls onto Wails bindings"`

### Task 18: Repo location, expertise, classification, editor, Shell, settings; delete the WS layer

**Files:**
- Modify:
  - `components/AddRepoDialog.tsx`, `components/CacheUsageCard.tsx`, `components/RepoRecoveryDialog.tsx`, `pages/settings/RepositoriesSection.tsx`, `services/onboardingSync.ts`, `pages/onboardingResults.ts`
  - `hooks/useExpertise.ts`, `components/ExpertiseNudge.tsx`, `components/CommandPalette.tsx` (expertise), `pages/settings/ExpertiseSection.tsx`
  - `pages/settings/ClassificationProvider.tsx`, `bench/keys.ts`, `bench/index.ts`, `services/editor.ts`
  - `utils/keybindingActions.ts`, `utils/keybindings.ts`, `utils/keymaps.ts`, `utils/benchSafety.ts`
  - `store/settings.ts`, `main.tsx`, `sentry.ts`, `components/ErrorBoundary.tsx`, `components/RootShell.tsx`, `components/ConnectingScreen.tsx`, `pages/LoginPage.tsx`
  - `frontend/package.json` (`@sentry/electron` → `@sentry/browser`), `types/index.ts`
- Delete:
  - `services/ws/` (all), `hooks/useWebSocketConnection.ts`, `types/wsSchemas.ts` (+ test)
  - `parseResponse` in `utils/error.ts` (keep `formatErrorMessage` and `ERROR_MESSAGES`, dropping `WEBSOCKET_DISCONNECTED`)
  - `services/engine.ts`, `components/EngineCrashedScreen.tsx`, `hooks/useEngineScreenData.ts`
  - `types/window.d.ts` (the `electron` typing), `types/electron/`, `shared/benchSafety.ts` (the WS allow-list)

**Interfaces:**
- Consumes: `RepoLocation`, `DevFolderScan`, `Expertise`, `Classification`, `Editor` and `Shell` from `@bindings/services`, plus the generated `Settings` model.
- Produces:
  - `store/settings.ts`, with the same exported API: `settingsStore`, `initSettings()`, `updateSettings(patch): ResultAsync<…>`, `resetSettings()`, `openSettingsFile()`;
  - `appStore.appVersion` and `appStore.platform`, set once at bootstrap from `Shell.AppInfo`.

- [ ] **Step 1: Repo location and scan.**
  - Port the call sites per the inventory, and delete the hand-written guards (`parseAddRepoResponse`, `parseCacheStats`, `parseCacheActionResponse`, `parseActionResponse`, `parseCounts`).
  - Code-specific UI copy now switches on `faultReason(fault)`: the reasons equal the old `code` strings.
  - `DevFolderScan.Scan` has no timeout. "Stop" cancels **its own** promise, kept via `onStart`. This replaces `SCAN_CANCEL` and `onSent`.
  - Move `runStartupDevFolderScan` here if Task 16 left it in `useEngine`.
- [ ] **Step 2: Expertise and classification.**
  - Enable, Disable and DismissNudge are `callWrite`; on err they run the old `handleExpertiseError` logic, using `faultReason` for the code.
  - Get and GetSettings are reads, and their results write `expertiseSettings` directly.
  - `SetByokKey` is `callWrite`; never log the key.
  - `bench/keys.ts` uses `call(() => Classification.SetSettings({mode:'rules', dismissal_scope:'local', provider:'anthropic', model:'', endpoint:''}))` and checks the result fields.
- [ ] **Step 3: Editor and Shell.**
  - Replace each `window.electron` use with its Shell row in the inventory: `Shell.OpenInEditor`, `Shell.OpenExternal` (http/https), `Shell.PickFolder` (`''` → `null`), `Shell.Quit`, and `Shell.AppInfo` (version and platform at bootstrap). `OpenInEditor` and `OpenExternal` use `callWrite`, because they reach outside the app. `PickFolder`, `Quit` and `AppInfo` use `call`, since `bench/index.ts` must still be able to quit.
  - `Editor.CheckoutStatus` is a read. `Editor.CheckoutHead` is `callWrite` with `timeoutMs: 60_000`, and its reason is `faultReason`.
  - Delete `openInEditorResponseSchema`.
- [ ] **Step 4: Engine-status removal** (decision 7).
  - Delete `services/engine.ts`, `EngineCrashedScreen.tsx`, `useEngineScreenData.ts`, `engineStatus` in `store/ui.ts`, and the log-tail panel in `ConnectingScreen.tsx` (which stays for the `authChecked` gate).
  - Remove the `connecting.restart` and `connecting.report` actions from `ACTION_CATALOG`, `KEYMAPS` (all three styles) and the palette commands.
  - `connecting.openLogs` calls `call(() => Shell.OpenLogFolder())`.
  - Run `rg "engineStatus|restartEngine|reportEngineCrash" src` and expect no hits.
- [ ] **Step 5: Settings store.**
  - `initSettings` already reads `Shell.GetSettings` (Task 16).
  - `updateSettings(patch)` merges the patch into the current state section by section, then calls `callWrite(() => Shell.SetSettings(merged))`, then sets the store from the **returned** value (Go may keep old leaves).
  - `resetSettings` → `Shell.ResetSettings`. `openSettingsFile` → `Shell.OpenSettingsFile`.
  - `settings:changed` (Task 15) keeps other edits in sync.
  - The `AppSettings` type becomes a re-export of the generated `Settings` model.
  - Keep the localStorage fallback and `legacySettingsSchema` only if the browser-only dev path (`pnpm dev` without Wails) still needs them; otherwise delete them and say so in the commit.
  - Check that `UI_FONT_CHOICES`, `CODE_FONT_CHOICES` and `fontChoiceId` survive. They were in `types/electron/settings.ts`; move them to `utils/fontChoices.ts`.
- [ ] **Step 6: Sentry and bench.**
  - Swap to `@sentry/browser`. In `sentry.ts`, `init({ dsn: import.meta.env.VITE_SENTRY_DSN, release: \`bistro@${import.meta.env.VITE_APP_VERSION}\`, sendDefaultPii: false, tracesSampleRate: 0, beforeSend })`, where `beforeSend` returns null unless `settingsStore.state.privacy.telemetryEnabled` (decision 12). Keep the `ui.click` breadcrumb drop.
  - Add `VITE_SENTRY_DSN` to the vite `define`, reading from `SENTRY_DSN` at build time.
  - `main.tsx` and `bench/` read `bench_keys` from the URL only.
- [ ] **Step 7: Delete the WS layer** (Files/Delete list). Then all of these must print nothing:
  ```bash
  rg "wsClient|services/ws|wsSchemas|parseResponse|window\.electron|WSInboundMessage|resolveWsResponse" frontend/src frontend/tests
  rg "from 'zod'" frontend/src -l   # every hit must be external input: router, bench, useDismissals, call.ts, settings legacy (if kept)
  ```
- [ ] **Step 8: Tests.** Swap any remaining `wsClient` mocks. Required behaviour:
  - `updateSettings` sets the store from Go's returned value, not the patch: mock `SetSettings` to return a different theme and assert it.
  - `AddRepoDialog`'s reason mapping is covered only if its logic is in a util or hook. Components are verified manually (project rule).
- [ ] **Step 9: Verify.**
  - Run the recipe checks, plus `pnpm build`, `wails3 task generate && git diff --exit-code frontend/bindings`, and `go test -race ./...`.
  - Manual `wails3 task dev`: add a repo by URL, cancel a dev-folder scan, open a file in `$EDITOR`, change the theme in Settings (both the window and `settings.json` update), and edit `settings.json` by hand (the UI follows within ~2 s).
- [ ] **Step 10: Commit.** `git add -A && git commit -m "feat: finish renderer port to Wails bindings and delete the WebSocket client"`
