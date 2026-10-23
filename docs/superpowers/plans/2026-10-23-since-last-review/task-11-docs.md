# Task 11: Docs — overview, roadmap, shortcuts, release note

**Files:**
- Modify: `docs/go-project-overview.md`, `docs/ROADMAP.md`, `docs/keyboard-shortcuts.md`, `website/src/pages/docs/keybindings.mdx`, `docs/ALPHA.md`, `docs/STAGE-2-TEAM.md`
- Modify: `CLAUDE.md` only if a convention changed (none planned; check)

**Interfaces:** none. Describe what Tasks 1–10 shipped; read the code if anything here disagrees with it — the code wins.

Facts to state (all from the spec and its implementation notes):
- Diffs are `merge-base(base, head)..head`; shallow clones deepen once, then fall back to the base tip with a log line.
- A patchset is recorded when the head or the merge base changes, not when only the base tip moves. Each patchset stores `merge_base_sha` and per-file `patch_id`s in `patchset_files` (`has_fingerprints` marks patchsets recorded with them).
- `patch_id` = SHA-256 of the file's patch without `index` lines and with `@@` line numbers stripped; it is also the viewed hash.
- The since-last-review baseline is the newest of: your latest GitHub review (any client), a Bistro approve/request-changes/comment, or the mark-viewed that left no file unviewed.
- PR heads are pinned as `refs/bistro/<safeID>/pull/<N>/heads/<sha>` in the repo the sync used; cache clones are refreshed in place so pins survive force-pushes.
- WS: `REQUEST_TRIAGE` → `TRIAGE_READY`, `REQUEST_INTERDIFF` → `INTERDIFF_READY`; `REQUEST_RANGE_DIFF` is gone. No `git range-diff` / `os/exec` path remains.

- [ ] **Step 1: `docs/go-project-overview.md`**
  - Line ~132 tree: replace `rangediff.go  # Range-diff handlers` with `triage.go` (since-last-review file triage) and `interdiff.go` (per-file interdiff); add `git/patchid.go`, `git/mergebase.go`, `git/retention.go`, `git/interdiff.go` to the `git/` listing in the same style.
  - Line ~229: replace "range-diff" in the handler domain list with "triage, interdiff".
  - In the sync / git section, add one short paragraph with the merge-base, patchset, and head-pin facts above.

- [ ] **Step 2: `docs/ROADMAP.md`**
  - Status blocks near lines 22, 35–43, 212–230, 242–244: replace the range-diff statements with the triage + interdiff design (link the spec `docs/superpowers/specs/2026-10-23-since-last-review-rebases-design.md`). Remove decision 2 about the bounded `os/exec` exception, or mark it superseded by the spec.
  - `### 2.1` heading: drop "range-diff partial"; the pair picker now drives triage + interdiff. Update its acceptance bullets (line ~275) to: diffing a pair triages files by patch ID and offers per-file interdiff; unavailable heads say so.
  - `### 2.2` (lines ~303–320): acceptance bullets become triage (changed/new/removed/unchanged, filter on `s`, status-bar summary, explicit message when no comparable review or no fingerprints) and `i` interdiff with the rebased label. Mark the "`s` falls back after force-push" release blocker cleared.
  - Line ~833 checklist item "Wire a local repo path into the range-diff handler…": check it off as superseded by triage/interdiff.

- [ ] **Step 3: Shortcuts**
  - `docs/keyboard-shortcuts.md` line 84: `| \`s\` | Since last review: show only files changed since your last review |`, and add `| \`i\` | Interdiff: the current file since your last review |` below it. Fix line 15's prose if it still says "range diff".
  - `website/src/pages/docs/keybindings.mdx` line 72: the same two rows in that table's wording style.

- [ ] **Step 4: Release note in `docs/ALPHA.md`**

Under `## Data and resets`, add:

```markdown
- Since this release, viewed marks are keyed on a rebase-stable patch fingerprint. Marks made before the upgrade reset once; mark files viewed again.
```

- [ ] **Step 5: `docs/STAGE-2-TEAM.md` S2.2**

Add after the S2.2 heading: `> **Superseded for FOSS (2026-10-23):** FOSS no longer shells out to \`git range-diff\`; since-last-review uses per-file patch-ID triage and go-git interdiffs (see \`docs/superpowers/specs/2026-10-23-since-last-review-rebases-design.md\`). A pure-Go range-diff remains a possible Team feature.` Leave the rest as the historical Team plan.

- [ ] **Step 6: Check for stragglers**

Run: `grep -rn -i "range.diff\|rangediff" docs website/src README.md CLAUDE.md | grep -v "docs/superpowers\|STAGE-2-TEAM"`
Expected: no matches, or only lines that explicitly say range-diff was removed/superseded.

- [ ] **Step 7: Commit**

```bash
git add docs website/src/pages/docs/keybindings.mdx
git commit -m "docs: describe since-last-review triage, interdiff, and the viewed-mark reset"
```
