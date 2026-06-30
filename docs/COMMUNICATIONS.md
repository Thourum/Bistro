# Bistro communication strategy and documentation plan

This plan keeps Bistro's public communication grounded for a FOSS-first launch.
Bistro should be explained as a keyboard-first, local-first desktop client for
GitHub pull request review. It should not imply team infrastructure, cloud sync,
or speed claims that the product has not measured.

## Positioning

### One-line description

Bistro is a keyboard-first, local-first desktop app for reviewing GitHub pull
requests from a fast local inbox.

### Longer description

Bistro syncs GitHub pull requests into a local SQLite cache, generates diffs on
the user's machine, and keeps common review actions close to the keyboard:
triage, open, navigate, comment, approve, and request changes.

### Messaging pillars

- **Keyboard-first review.** The product is built around shortcuts, command
  palette workflows, and remappable bindings. Avoid implying that mouse use is
  impossible or shameful; say that review can stay keyboard-driven.
- **Local-first data model.** PR metadata, comments, settings, and generated
  diffs live locally. GitHub is the sync source; Bistro should not introduce a
  required server for the FOSS product.
- **Fast local inbox.** The inbox and diff views should feel fast because data is
  cached and rendered locally. Use measured performance targets when available;
  avoid unverified claims like "10x faster" or "instant".
- **GitHub first.** The first public story is GitHub PR review. Mention GitLab or
  other providers only as roadmap work, not launch capability.
- **Built for AI-era PR volume.** AI coding tools increase review throughput and
  notification pressure. Bistro's answer is better triage and keyboarded review,
  not AI-generated review promises by default.

### Voice and wording rules

- Prefer plain, technical language over launch hype.
- Say what exists, what is local, and what is planned.
- Do not overstate privacy: tokens still go to GitHub and any user-configured
  providers; Bistro's claim is that the FOSS app has no Bistro-operated server in
  the review path.
- Use "local-first" when describing architecture and data ownership; use
  "offline queue" only for implemented action replay behavior.
- Use screenshots, keyboard references, and concrete workflows as proof instead
  of slogans.

## Audience map

| Audience | Primary question | Communication job |
| --- | --- | --- |
| Individual reviewers | Can this make my daily PR review faster without changing GitHub? | Show inbox triage, diff navigation, shortcuts, and local cache behavior. |
| Open-source contributors | Can I run, inspect, and improve this project? | Keep setup, architecture, contribution, and issue labels accurate. |
| Security-conscious developers | Where do code, tokens, and review data go? | Explain local storage, OS keychain, GitHub OAuth, and absence of Bistro cloud. |
| Early adopters | What works now and what is still rough? | Keep roadmap/status honest; separate source-run setup from packaged releases. |

## Documentation plan

### Canonical docs

- `README.md` — public product overview, source install, basic run flow, links to
  development and roadmap docs.
- `DEVELOPMENT.md` — authoritative setup, configuration, build, and verification
  instructions for contributors.
- `AGENTS.md` — internal agent and architecture conventions; keep aligned with
  development docs when commands or structure change.
- `docs/ROADMAP.md` — public feature status and roadmap boundaries.
- `design/keyboard-bindings.md` — user-facing source for keyboard reference
  content until a generated website page exists.
- `design/security-model.md` — source material for public privacy/security copy.
- `docs/FEEDBACK.md` — public launch feedback path and triage expectations.
- `docs/LOCAL-FIRST-PRIVACY.md` — public local-first privacy explainer.

### Near-term documentation work

1. **Contributor onboarding cleanup.** `design/contribution-guide.md` now points
   contributors to the canonical product, setup, architecture, roadmap, and
   keyboard references, and uses the current pnpm/source-run workflow.
2. **Website implementation.** `website/PLAN.md` now uses the grounded launch
   messaging in this document. The next website step is implementing the static
   site around Home, Docs, Keyboard reference, Roadmap, and Updates.
3. **Public status page.** Add a compact "what works now" section or page before
   launch so early users know which workflows are ready, which require source
   setup, and which are roadmap items.
4. **Feedback loop docs.** `docs/FEEDBACK.md` and the GitHub issue templates now
   give users a launch feedback path for bugs, usability issues, docs fixes, and
   feature requests, with prompts for OS, app version/commit, provider, logs, and
   reproduction steps.
5. **Security/privacy explainer.** `docs/LOCAL-FIRST-PRIVACY.md` condenses the
   security model into public docs: GitHub OAuth, OS keychain storage, local
   SQLite cache, local git/diff work, and optional provider calls only when
   configured.

## Website copy strategy

### Homepage narrative

1. **Hero:** Bistro is a keyboard-first local desktop app for GitHub PR review.
2. **Problem:** AI-assisted development increases PR volume and force-push churn;
   browser review workflows still make triage, diff navigation, and repeated
   actions expensive.
3. **Product proof:** Local inbox, local diffs, command palette, keyboard review
   actions, and visible sync status.
4. **Trust proof:** Local-first architecture, GitHub OAuth, OS keychain, open
   source code, and honest roadmap.
5. **CTA:** Run from source now; packaged installers when v1.0 packaging is ready.

### Recommended launch-page sections

- Hero with one screenshot and source-run CTA.
- "Why local" section: local cache, local diff generation, no required Bistro
  server for FOSS.
- "Keyboard review loop" section: triage, open, navigate diff, comment, approve.
- "GitHub first" section: current provider scope and roadmap link.
- "For contributors" section: setup link, architecture link, good first issues.
- "Known limits" section: packaged installers pending, roadmap items, support
  boundaries.

### Copy guardrails

- Replace "10x faster" with measured numbers once benchmarks exist.
- Replace "blazingly fast" with the mechanism: local cache and local diff
  generation.
- Replace "Never touch your mouse" with "Keep review actions on the keyboard".
- Avoid "offline-first" as a blanket phrase unless the doc names which actions
  are available offline and how replay works.

## Launch communication plan

### Before launch

- Freeze a short public status statement: supported OSes, GitHub-only scope,
  source-run requirements, and known rough edges.
- Capture fresh screenshots from the current app state.
- Prepare issue templates for bugs, docs fixes, and feedback.
- Keep `docs/FEEDBACK.md` and the issue templates aligned with the launch support
  capacity and current run mode.
- Publish a contributor path: setup, architecture, first issue, verification
  commands, and review expectations.

### Launch week

- Announce on GitHub first with a release note or launch discussion.
- Share a short demo clip or screenshot thread focused on the keyboard review
  loop, not broad productivity claims.
- Post to developer communities only where feedback is welcome and support
  capacity exists.
- Route all feedback back to GitHub issues/discussions so it becomes durable.

### After launch

- Triage feedback weekly into docs fixes, product bugs, and roadmap requests.
- Maintain a public changelog/update post for material improvements.
- Update screenshots and keyboard references when UI or bindings change.

## Contributor onboarding and feedback loop

### Onboarding path

1. Read `README.md` for the product overview.
2. Follow `DEVELOPMENT.md` to run the app.
3. Read `docs/go-project-overview.md` for the Go sync engine and WebSocket flow.
4. Use `docs/ROADMAP.md` to understand current project priorities.
5. Pick an issue labeled for docs, onboarding, UI polish, or bounded backend work.

### Feedback intake

- **Bug reports:** require OS, commit/version, GitHub provider scope, steps,
  expected/actual behavior, and relevant logs.
- **Performance reports:** require PR size, file count, local machine basics, and
  which view/action felt slow.
- **Docs reports:** ask what page was confusing, what the reader tried, and what
  fixed wording they expected.
- **Feature requests:** ask for the review workflow being improved and whether it
  fits local-first GitHub PR review.

## Immediate content gaps found

- Contributor onboarding docs now match the current pnpm/source-run workflow;
  keep them current when commands, runtime versions, or repo structure change.
- Website implementation has not started yet; `website/PLAN.md` is the current
  grounded content plan.
- Public docs now include a concise feedback/reporting path in `docs/FEEDBACK.md`.
- Public docs now include a short privacy/security explainer in
  `docs/LOCAL-FIRST-PRIVACY.md`, derived from the detailed security model.
- Keyboard reference content exists; the website plan now names
  `design/keyboard-bindings.md` as the source of truth until a generated page
  exists.

## Follow-up issue candidates

1. Implement the static website from `website/PLAN.md`.
2. Keep feedback templates current as launch support capacity changes.
3. Expand the public local-first security/privacy explainer when packaged builds
   or optional providers are added.
