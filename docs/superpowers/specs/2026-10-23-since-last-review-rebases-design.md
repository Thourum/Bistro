# Since Last Review, Reliable After Rebases and Force-Pushes — Design

**Date:** 2026-10-23
**Status:** Approved (user-reviewed design conversation)
**Roadmap:** Review-features priority #2 (20 tags, mostly 2025–26, HN and
lobste.rs); clears the "`s` falls back after force-push" release blocker.
Includes the merge-base fix, which the keyboard-review spec depends on.

## Summary

`s` should answer "what do I need to look at again?" even after the author
rebases or force-pushes. Instead of a `git range-diff` text view, Bistro
**triages files**: each file is marked changed / new / removed / unchanged
relative to the patchset you last reviewed, by comparing per-file patch
fingerprints that ignore line-number shifts. Inside a changed file, `i` shows an
**interdiff** (file at your reviewed head → file now). Diffs move to the
correct base (merge-base), which also fixes left-side line numbers for
line comments.

## Current state (2026-10-23)

- `go/internal/server/handler/rangediff.go` never invokes the `git range-diff`
  runner: no repo path reaches the handler, so every request degrades. The
  latest pair returns the stored plain diff labeled "degraded"; other pairs
  return nothing.
- Patchsets are recorded on any head **or base-tip** change
  (`sync/writer_details.go` `recordPatchsetIfChanged`), so the base branch
  advancing creates spurious patchsets.
- The last-reviewed patchset is recorded only on Bistro approve /
  request-changes (`actions/executor.go` `recordLastReviewedSHA`).
- Viewed state is keyed on `sha256(patch)` (`handler/diff.go`
  `PatchContentHash`), so a rebase that only shifts `@@` line numbers
  un-views the file.
- Bistro diffs `pr.BaseSHA..HeadSHA` (`sync/fetcher.go` `generateDiff`), i.e.
  base-branch **tip** to head, so upstream changes leak into the PR diff and
  old-side line numbers disagree with GitHub.
- Old heads are not retained: after a force-push the previous head is
  unreachable from any ref Bistro fetches.

## Design

### 1. Merge-base diffs

- `generateDiff` diffs `merge-base(BaseSHA, HeadSHA)..HeadSHA`. Computed with
  go-git in the clone/local repo the sync already uses. If the merge base can't
  be computed (shallow history), deepen/broaden the fetch like the existing
  required-commit fallback; if that still fails, fall back to `BaseSHA` and log
  it.
- `patchsets` gains `merge_base_sha` (migration). A new patchset is recorded
  when **head or merge base** changes — not when only the base tip moves.
- The keyboard-review spec's left-side line comments depend on this section.

### 2. Per-file fingerprints

- New table `patchset_files(pr_id, patchset_number, path, patch_id,
  PRIMARY KEY(pr_id, patchset_number, path))`, written in the same transaction
  as the patchset row.
- `patch_id` = SHA-256 of the file's patch with `@@ … @@` hunk-header line
  numbers stripped (the idea behind `git patch-id`). One pure Go function,
  shared with §3's viewed hash.
- Viewed state switches to `patch_id`, so viewed marks survive line-shift-only
  rebases. **One-time effect:** existing viewed marks don't match the new hash
  and reset once on upgrade. Note it in the release notes.

### 3. Baseline: most recent of three events

`last_reviewed` gains `reviewed_at` and `source` (`github_review` |
`bistro_action` | `all_viewed`). On each event, upsert only if newer.

1. **GitHub review:** during sync, the current user's latest submitted review
   (`commit_id`, `submitted_at`). Counts reviews made in the browser or `gh`.
2. **Bistro action:** approve / request changes / submit review (today's path).
3. **All viewed:** the `MARK_FILE_VIEWED` that leaves zero unviewed files.

The baseline maps to a patchset by `head_sha` (+ `merge_base_sha` when known).
If it doesn't map to a recorded patchset, `s` shows "no comparable prior
review" instead of a misleading diff.

### 4. Triage view (`s`)

- `REQUEST_DIFF` (or a sibling request) returns, per file, a status relative to
  the baseline patchset: `changed` | `new` | `removed` | `unchanged`, computed
  from `patchset_files`. Files present only in the baseline patchset are listed
  as `removed`.
- `s` toggles a filter to changed / new / removed files. File bodies stay the
  normal current PR diff. Sidebar rows get a badge.
- Status bar: `since P3 (your review, 2d ago): 4 of 37 files changed`.
- If either patchset lacks fingerprints (recorded before this feature), `s`
  says so instead of guessing.

### 5. Per-file interdiff (`i`)

- `i` on a changed file toggles the body to the file at the baseline head vs.
  the file at the current head (a new WS request; Go generates it with go-git).
- If the file's content at the two merge bases differs, show the label
  `rebased: includes upstream changes`. No upstream subtraction (out of scope).
- **Head retention:** when a patchset is recorded, pin its head as
  `refs/bistro/<safeID>/ps/<n>` in whichever repo the sync used (cache clone or
  user repo; local repos already only receive `refs/bistro/*`). Force-pushes
  and gc no longer lose old heads.
- If a head commit is missing (pre-feature patchsets, or a deleted cache
  clone), interdiff reports unavailable. Triage still works.

### 6. Delete the range-diff path

Remove: the `git range-diff` runner (`go/internal/git/rangediff.go`, its probe
in `server/app.go`), the stored-plain-diff fallback in
`handler/rangediff.go`, `RangeDiffView.tsx`, `DegradedRangeDiffIndicator.tsx`,
and `useRangeDiff.ts`. The existing patchset-pair picker (`P2 → P5`) reuses
triage + interdiff with an explicit baseline. Update
`docs/go-project-overview.md` and `docs/ROADMAP.md` (the release blocker).

## Out of scope

- Subtracting upstream changes from interdiffs.
- Per-commit review and stacked PRs (lower-priority roadmap items).

## Testing (Go; no React component tests)

- `patch_id` ignores `@@` line-number shifts and changes on content changes.
- Baseline upsert keeps the newest event across the three sources.
- Force-push then sync: the previous head stays reachable via the pinned
  `refs/bistro/<safeID>/ps/<n>` ref and the interdiff request succeeds.
- Merge-base diff: a PR whose base branch advanced shows only the PR's changes.

## Implementation notes (added while planning, 2026-10-23)

- **Pin ref name.** `refs/bistro/<safeID>/ps/<n>` has no PR number, so two PRs
  of one repo would collide, and `n` is assigned by the writer after the repo
  lock is released. Heads are pinned at fetch time instead, as
  `refs/bistro/<safeID>/pull/<N>/heads/<sha>`: every head the sync diffed stays
  reachable, which is a superset of every recorded patchset head.
- **Cache clones survive force-pushes.** Today a missing commit re-creates the
  cache clone from scratch, which would drop pins. The sync now fetches into the
  existing cache clone first and only re-clones when that fails.
- **Fingerprints use the full patch.** `patch_id` is computed before the
  per-file size budget truncates the stored patch (binary files hash their blob
  IDs), and is stored in `pr_diffs.patch_id` so the viewed hash matches it.
- **`patchsets.has_fingerprints`** distinguishes "zero files" from "recorded
  before this feature".
- **Current patchset** is the latest patchset whose head equals the PR head
  (the base tip no longer identifies a patchset).
