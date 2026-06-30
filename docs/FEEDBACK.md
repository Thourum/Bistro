# Feedback and launch reports

Bistro is early and source-run first. Feedback is most useful when it is specific
enough for a maintainer to reproduce the problem or understand the review
workflow you want to improve.

Use GitHub issues or discussions as the durable feedback path. Avoid sending
private repository data, OAuth tokens, full logs with secrets, or screenshots that
show confidential code.

## What to file

### Bugs

File a bug when Bistro does something unexpected: fails to start, cannot sign in,
syncs the wrong state, renders a diff incorrectly, drops a keyboard action, or
cannot replay a review action.

Include:

- OS and architecture, for example `macOS 15 arm64` or `Windows 11 x64`.
- Bistro version or commit SHA.
- Whether you are running from source or a packaged build.
- Whether the issue happens before or after GitHub OAuth.
- Steps to reproduce.
- Expected behavior and actual behavior.
- Relevant terminal output from Electron or `bistro-sync`, with tokens and private
  repository details removed.

### Performance reports

File a performance report when a local view or action feels slow enough to break
review flow.

Include:

- PR size: file count, approximate changed lines, and whether the PR has large
  generated files.
- Machine basics: OS, CPU architecture, memory if relevant.
- The slow path: inbox load, search/filter, opening a PR, diff scrolling,
  comment submission, sync, or action replay.
- Whether the data was already synced locally or Bistro was still fetching from
  GitHub.

### Docs fixes

File a docs issue when setup, architecture, keyboard shortcuts, privacy wording,
or roadmap status is confusing or stale.

Include:

- Page or section name.
- What you tried to do.
- The sentence, command, screenshot, or missing detail that caused confusion.
- Suggested wording if you have it.

### Feature requests and workflow feedback

File a feature request when you can describe a real review workflow that Bistro
does not yet handle.

Include:

- The review workflow you are trying to improve.
- The provider involved. Bistro is GitHub-first today; other providers belong on
  the roadmap unless separately implemented.
- Whether the request fits a local-first desktop app with no required Bistro
  server in the open-source review path.
- Any keyboard shortcut, command-palette, or inbox behavior that would make the
  workflow faster.

## Triage expectations

Maintainers should triage launch feedback into four buckets:

1. **Product bugs** — reproducible defects in sync, local data, diffs, review
   actions, keyboard behavior, or packaging.
2. **Docs fixes** — stale commands, unclear setup, missing privacy details, or
   confusing roadmap/status language.
3. **Roadmap requests** — larger workflow changes, provider expansion, team
   features, or non-local architecture proposals.
4. **Support questions** — setup or environment issues that need clarification
   before they can become a bug.

Close the loop by linking merged fixes or roadmap decisions back to the original
issue or discussion.
