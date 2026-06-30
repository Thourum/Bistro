# Bistro website plan

## Purpose

The website should explain Bistro without launch hype: a keyboard-first,
local-first desktop app for GitHub pull request review. The first public site is
for developers who want to understand what works today, run the project from
source, and decide whether Bistro fits their review workflow.

The site should not imply a Bistro cloud service, team infrastructure,
multi-provider support, packaged installers, or benchmarked speed claims before
those exist.

**Recommended stack:** Astro static site. Keep the build simple, content-led, and
easy to host on GitHub Pages, Netlify, or Vercel.

## Messaging baseline

### One-line description

Bistro is a keyboard-first, local-first desktop app for reviewing GitHub pull
requests from a fast local inbox.

### Longer description

Bistro syncs GitHub pull requests into a local SQLite cache, generates diffs on
the user's machine, and keeps common review actions close to the keyboard:
triage, open, navigate, comment, approve, and request changes.

### Messaging pillars

- **Keyboard-first review:** shortcuts, command palette workflows, and remappable
  bindings keep review actions on the keyboard.
- **Local-first architecture:** PR metadata, comments, settings, and generated
  diffs live locally. GitHub remains the sync source; the FOSS product should not
  require a Bistro-operated server in the review path.
- **Fast local inbox:** explain the mechanism: local cache, local diff generation,
  and local rendering. Add measured numbers only after benchmarks exist.
- **GitHub first:** launch copy should name GitHub PR review as the supported
  provider scope. GitLab, Gitea, and other providers belong on the roadmap.
- **Built for AI-era PR volume:** AI coding tools increase PR volume and
  force-push churn. Bistro's launch story is better triage and keyboarded review,
  not AI-generated review promises by default.

### Copy guardrails

- Do not use unmeasured speed multipliers, absolute latency claims, or vague
  speed adjectives.
- Avoid absolute mouse-free promises; use "Keep review actions on the keyboard"
  instead.
- Use "local-first" for the architecture. Use "offline queue" only for specific
  implemented action replay behavior.
- Be precise about privacy: tokens still go to GitHub and any user-configured
  providers. The claim is no required Bistro-operated server for the FOSS review
  path.
- Prefer screenshots, keyboard references, setup steps, and status tables over
  broad slogans.

## Launch site map

### 1. Home

**Purpose:** Give reviewers a clear product overview and a source-run CTA.

**Sections:**

- Hero: one-line description, current app screenshot, and "Run from source" CTA.
- Problem: browser review workflows make triage, diff navigation, and repeated
  review actions expensive as PR volume increases.
- Product proof: local PR inbox, local diffs, keyboard navigation, command
  palette, comment/approval actions, and sync status.
- Why local-first: local SQLite cache, local git/diff work, GitHub sync source,
  and no required Bistro cloud service.
- GitHub-first scope: what works for GitHub PRs now and where provider expansion
  sits on the roadmap.
- Known limits: source setup today, packaged installers later, early workflows
  that still need testing, and support boundaries.

**Draft hero copy:**

> Review GitHub pull requests from a local desktop inbox. Bistro keeps triage,
> diff navigation, comments, approvals, and request-changes flows close to the
> keyboard while GitHub remains the source of truth.

### 2. Docs / Getting started

**Purpose:** Help early users and contributors run Bistro without guessing.

**Sections:**

- Current status: source-run setup today; packaged installers when release
  packaging is ready.
- Prerequisites: supported OS notes, Go, pnpm, GitHub OAuth setup, and `.env`
  configuration.
- First run: launch Electron, authenticate with GitHub, sync repositories, open a
  PR, navigate the diff, and submit a review action.
- Troubleshooting: OAuth setup, sync engine startup, SQLite location, logs, and
  reset steps.
- Links: `README.md`, `DEVELOPMENT.md`, `docs/ROADMAP.md`, architecture overview,
  contribution guidance, `docs/FEEDBACK.md`, and
  `docs/LOCAL-FIRST-PRIVACY.md`.

### 3. Privacy / local-first

**Purpose:** Explain what local-first means without overstating privacy.

**Sections:**

- What stays local: SQLite cache, settings, local clones/diffs, and queued review
  actions.
- What leaves the machine: GitHub API calls, OAuth, review actions sent back to
  GitHub, and any user-configured future providers.
- Token handling: GitHub tokens in the OS keychain, not in SQLite.
- Local storage limits: private repository context can exist in the local cache,
  so users should use normal workstation protections and redact logs.
- Source of truth: derive copy from `docs/LOCAL-FIRST-PRIVACY.md` and
  `design/security-model.md`.

### 4. Keyboard reference

**Purpose:** Make keyboard-driven review concrete.

**Sections:**

- Navigation: inbox movement, open/close, PR list traversal, file and diff
  movement.
- Review actions: comment, approve, request changes, mark viewed, refresh/sync,
  and command palette.
- Search and filtering: inbox filters, repository selection, status views.
- Customization: remappable bindings when available.

**Source of truth:** Generate or manually sync this page from
`design/keyboard-bindings.md` until a binding extraction script exists. The page
must name its source so stale shortcuts are easy to spot.

### 5. Roadmap / status

**Purpose:** Set honest expectations for early adopters.

**Sections:**

- What works now: GitHub sync, local cache, local diffs, core inbox/diff/review
  flows.
- What is rough: packaging, platform coverage, edge-case sync behavior, or any
  workflow that still requires source setup.
- What is next: near-term roadmap items from `docs/ROADMAP.md`.
- What is not launch scope: multi-provider support, team/server features, or broad
  AI review automation unless separately implemented.

### 6. Updates

**Purpose:** Keep launch and release communication durable.

**Sections:**

- Release notes and material product updates.
- Short engineering notes about local-first design, keyboard workflows, sync, and
  diff generation.
- RSS feed for people tracking the project.

## Visual and content strategy

- **Plain and technical:** write for developers evaluating a tool, not a generic
  productivity audience.
- **Screenshot-led:** show the inbox, diff view, keyboard overlay/reference, sync
  status, and review action flow.
- **Status-aware:** use current-state labels such as "source setup", "roadmap",
  and "experimental" where appropriate.
- **Accessible by default:** light and dark theme support, semantic HTML, keyboard
  navigable pages, readable contrast, and useful alt text.
- **No unnecessary client JavaScript:** static content should remain static unless
  a specific interaction needs enhancement.

## Technical requirements

- Static output with minimal client JavaScript.
- Lighthouse targets: 95+ for accessibility, best practices, and SEO; performance
  target should be validated on the deployed site rather than promised in copy.
- Sitemap, RSS feed, per-page title/description, Open Graph image, and software
  application structured data.
- Privacy-conscious analytics only if the project has a clear reason to collect
  aggregate page data; document the choice.
- CDN-friendly assets with compressed images and cacheable static files.

## Asset checklist

- [ ] Current app screenshot for the hero.
- [ ] Inbox, diff, keyboard reference, and review-action screenshots.
- [ ] Logo SVG and favicon set.
- [ ] Open Graph image template.
- [ ] Light and dark theme color variables.
- [ ] Short demo clip only if it shows the actual keyboard review loop.

## Content checklist

- [ ] Homepage copy following the baseline above.
- [ ] Current status block: source-run setup, GitHub-only scope, packaged installer
      status, and known rough edges.
- [ ] Getting-started guide linked to canonical development docs.
- [ ] Keyboard reference page synced from the binding source.
- [ ] Roadmap/status page derived from `docs/ROADMAP.md`.
- [x] Privacy/security explainer derived from `design/security-model.md`.
- [x] Feedback section: bugs, docs fixes, feature requests, and usability feedback
      with required context such as OS, commit/version, provider, logs, and repro
      steps.
- [ ] Update post template for release notes and launch notes.

## Launch sequence

1. **Copy and information architecture:** finalize the five-page map, current
   status block, and homepage draft.
2. **Screenshots and proof:** capture current app screenshots and keyboard-flow
   evidence from the working product.
3. **Static site scaffold:** build the Astro site, shared layout, navigation,
   metadata, sitemap, and RSS.
4. **Content pass:** add Home, Docs, Keyboard reference, Roadmap, and Updates
   pages; link each page to its canonical source document.
5. **Release readiness:** run accessibility/performance checks, verify links,
   confirm install/source-run instructions, and publish only claims backed by the
   current app.

**Soft launch:** share with a small group of developers who can run from source
and file GitHub issues.

**Public launch:** announce on GitHub first, then share in developer communities
where support capacity exists. Route feedback back to GitHub issues or
discussions.

## Maintenance

- Update status, screenshots, and keyboard references when the product changes.
- Add release notes for material improvements.
- Keep website claims aligned with `README.md`, `DEVELOPMENT.md`,
  `docs/ROADMAP.md`, `design/keyboard-bindings.md`, and
  `design/security-model.md`.
- Review feedback monthly and convert recurring confusion into docs fixes or
  product issues.
