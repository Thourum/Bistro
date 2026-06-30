# Contributing to Bistro

Bistro is a keyboard-first, local-first desktop app for GitHub pull request
review. Contributions should preserve that shape: review work happens quickly,
from the keyboard, with PR data, diffs, and credentials staying on the user's
machine.

## Start here

1. Read the product overview in [README.md](README.md).
2. Set up the app with [DEVELOPMENT.md](DEVELOPMENT.md).
3. Read the contributor onboarding guide in
   [design/contribution-guide.md](design/contribution-guide.md).
4. Check [docs/ROADMAP.md](docs/ROADMAP.md) before starting larger work.

## Contribution rules

- Keep Bistro local-first. Do not add a required hosted service for the
  open-source app.
- Preserve keyboard-first review. Every core review action needs a keyboard path
  and updated shortcut documentation.
- Keep GitHub tokens and local repository data on the user's machine. Secrets
  belong in the OS keychain or ignored local env files, never in committed docs
  or code.
- Use specific, testable language in docs. Explain mechanisms such as local
  cache, local diffs, and offline queueing instead of broad speed claims.

## Developer Certificate of Origin

Bistro uses the Developer Certificate of Origin (DCO) instead of a Contributor
License Agreement. Every commit must include a `Signed-off-by` line certifying
that you have the right to contribute the work under Bistro's license.

Add the sign-off automatically with:

```bash
git commit -s
```

or append it manually to the commit message:

```text
Signed-off-by: Your Name <you@example.com>
```

The name and email must match an identity you are allowed to use for the
contribution. The certification text is in [DCO.txt](DCO.txt).

## Before opening a pull request

Choose the smallest verification that matches your change:

```bash
# Go engine changes
cd go && go vet ./... && go test -race ./...

# Electron UI changes
cd electron && pnpm type-check && pnpm lint && pnpm test

# Docs-only changes
git diff --check
```

For UI or review-flow changes, also run the app and verify the relevant keyboard
path manually: inbox navigation, opening a PR, moving through files/hunks,
command palette, help overlay, and the action you changed.

## License

By contributing, you agree that your contribution is licensed under the
[GNU General Public License v3.0 only](LICENSE), the same license as Bistro.
