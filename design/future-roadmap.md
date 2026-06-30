# Bistro Future Roadmap

**Purpose:** Document planned features, extensibility points, and what's intentionally out of scope.

**Audience:** Product team, contributors planning Phase 2+, users asking "when will X be ready?"

---

## Version Strategy

**Phase 1 (MVP):** GitHub-only, core review workflow
**Phase 2:** Multi-provider, notifications
**Phase 3:** Advanced features, plugins
**Phase 4:** CLI, extensions

Each phase is shipped when features are complete and tested, not on a fixed calendar.

---

## Phase 1: MVP (Current)

**Status:** In development

**Scope:**
- GitHub PR review (no GitLab yet)
- Keyboard-first navigation
- Offline mode (read-only)
- Three-pane layout
- Vim bindings
- Local SQLite cache
- Dark mode

**Out of scope for Phase 1:**
- Notifications
- Custom themes
- Plugins
- CLI
- GitLab / Gitea

---

## Phase 2: Multi-Provider & Notifications

**Estimated:** 3-4 months after Phase 1 release

### New Providers

- **GitLab** (self-hosted + GitLab.com)
  - Merge requests equivalent to GitHub PRs
  - Same three-pane UI
  - Same keyboard bindings
  - Different API (GitLab GraphQL)

- **Gitea** (self-hosted)
  - Simpler API than GitHub
  - Community request (self-hosted Git)

**Implementation notes:**
```
Single DB, multiple providers:
├── prs table: add "provider" column
├── Each provider has separate auth tokens
├── UI: filter by provider or show all
└── Sync loop: parallel per provider
```

### Notifications

- **Native OS notifications** (macOS, Windows, Linux)
  - New PR assigned to you
  - PR approved / changes requested
  - CI failed on your PR
  - Settings: mute, enable/disable per type

- **In-app notification center**
  - List of recent events
  - Mark as read / dismiss
  - Filter by type

**Timeline:** Begin in Phase 2.2 (after multi-provider stable)

### Enhanced Search

- **Server-side FTS** (Phase 2.2)
  - If 1000+ PRs, search becomes slow
  - Optional backend (Typesense, Meilisearch) or server-side Go service
  - Out of scope for Phase 1 (local SQLite is enough)

### Action Queuing (Offline)

- **Queue for sync when offline**
  - User approves PR while offline
  - Stored in local `offline_actions` table
  - When online, retry with exponential backoff
  - Visual indicator: "Queued for sync"

**Current status (Phase 1):** User sees "offline, try when online" (no queue)

---

## Phase 3: Advanced Features

### Custom Themes & Styling

- **Theme system**
  - Default: dark (current)
  - Optional light theme
  - User-defined color schemes (Phase 3.2)

- **Custom keyboard layouts**
  - User can remap j/k → ↑/↓
  - User can add custom bindings
  - Stored in `~/.bistro/keybindings.json`

### Plugin System

- **Why plugins?**
  - Extensibility without changing core
  - Community-driven features
  - Use cases: custom formatters, integrations, analytics

- **Plugin types**
  ```
  1. Renderers (custom diff viewers)
  2. Notifiers (Slack, Discord webhooks)
  3. Filters (custom search)
  4. Providers (Forgejo, custom Git hosting)
  ```

- **Plugin architecture** (sketch)
  ```
  ~/.bistro/plugins/
  ├── my-slack-notifier/
  │   ├── manifest.json
  │   ├── plugin.js
  │   └── package.json
  └── custom-diff-viewer/
  ```

  Plugins are JavaScript (sandboxed via WebWorker or separate process)

### Bulk Actions

- **Select multiple PRs**
  - Approve all at once
  - Add labels / comments
  - Archive / hide
  - Keyboard: Shift+j/k to select range, Ctrl+a to select all

### AI-Assisted Review Suggestions (Phase 3.2)

- **Code review suggestions**
  - Highlight security issues
  - Suggest tests
  - Check for common bugs
  - Uses Claude API (optional, Phase 3.2)

- **Why optional?**
  - Requires network call to Claude API
  - User must have API key or credits
  - Turns on/off in settings

**Implementation:**
```go
// After diff loaded
if settings.AIReviewEnabled {
    suggestions, err := CallClaudeAPI(diff)
    // Highlight suggestions in diff
}
```

---

## Phase 4: CLI & Extensions

### CLI Tool

**Use case:** Approve PR from command line in CI/CD

```bash
$ bistro pr approve 123 --repo org/repo --github-token $TOKEN

Approving PR #123 in org/repo...
✓ Approved (2.3s)
```

**Implementation:** Go binary, no Electron (slim)

### VSCode Extension

**Use case:** Review PR without switching away from editor

**Features:**
- Sidebar panel showing assigned PRs
- Inline diff viewer (native VSCode)
- Comment directly from editor
- Single Ctrl+Shift+B to toggle sidebar

**Architecture:**
```
vscode-bistro/
├── extension.ts (VSCode API)
├── client/ (shared with desktop)
└── package.json
```

### GitHub Actions / Automation

- **Approve on CI pass**
  ```yaml
  - name: Auto-approve if tests pass
    uses: bistro-app/action-approve@v1
    if: success()
    with:
      github-token: ${{ secrets.GITHUB_TOKEN }}
  ```

### Browser Extension

**Out of scope currently**, but possible:
- Review PRs from github.com directly
- Sync with desktop app
- Keep keyboard bindings consistent

---

## Out of Scope (Permanently)

### What Bistro Won't Do

| Feature | Why |
|---------|-----|
| **Code push** | Out of scope (review-only) |
| **PR creation** | Out of scope (review-only) |
| **Issue tracking** | Different workflow, separate tool |
| **Project management** | Use GitHub Projects |
| **Communication** (Slack, Discord) | Notifications only, no team chat |
| **Repository management** | Use GitHub settings |
| **Admin functions** | Org admins use GitHub web UI |
| **Mobile app** | Desktop-first (may revisit Phase 4+) |

### Why These Boundaries?

Bistro's mission: **Fast, distraction-free code review**

Expanding beyond PR review would:
- Increase complexity
- Decrease focus
- Make app slower
- Dilute keyboard-first design
- Compete with other tools (GitHub Issues, Projects, etc.)

**Best practice:** Do one thing well, integrate with others.

---

## Extensibility Points

### URL Schemes

**Phase 2:**

```
bistro://pr/123?repo=org/repo&provider=github
→ Opens PR #123 in Bistro (if installed)

bistor://search?q=label:bug
→ Opens Bistro, runs search
```

### Webhooks

**Phase 3:**

```
User can configure webhooks to external services:
- When PR approved: POST to custom URL
- When comment posted: Trigger action
- When CI fails: Send notification
```

### Database Inspection

**Phase 2:**

```
$ bistro db export --format=json > my-prs.json
$ bistro db query "SELECT * FROM prs WHERE state='open'"
```

User can analyze their own data, build custom reports.

---

## Performance Roadmap

| Phase | Focus | Goal |
|-------|-------|------|
| 1 | Launch, fix bugs | Startup < 500ms, < 200MB |
| 2 | Multi-provider scale | Startup < 800ms, < 300MB with 500+ PRs |
| 3 | Plugins overhead | Keep baseline < 250MB |
| 4 | CLI performance | Instant (< 100ms) |

---

## Community & Ecosystem

### Open Source

- **License:** GPLv3-only
- **Repository:** Public on GitHub
- **Contributing:** Welcome with DCO sign-off (see `CONTRIBUTING.md`)

### Plugin Marketplace (Phase 3.2)

```
Registry of community plugins:
bistro.app/plugins/
├── [my-notifier by @user]
├── [custom-themes by @designer]
└── [ci-parser by @devops]
```

### Discord / Community

- Community server (Phase 3)
- Weekly builds channel
- Plugin showcase
- User feedback

---

## Long-Term Vision (Phase 4+)

### "Code Review Platform"

Bistro could evolve into:

1. **Multi-VCS Hub**
   - GitHub, GitLab, Gitea, Forge, etc.
   - Single UI for all Git hosting

2. **Team Review Workflow**
   - Assign reviews to team members
   - Track review metrics
   - SLAs (e.g., "approve within 24h")

3. **Enterprise Features**
   - Admin controls
   - Audit logs
   - Custom integrations
   - Single sign-on (LDAP, SAML)

4. **AI-Native**
   - Review suggestions (Claude, GPT-4)
   - Auto-generate test cases
   - Commit message lint

**Timeline:** 2026-2027 (speculative)

---

## What's Next?

**Immediate priorities (Phase 1 finish line):**
1. Stabilize desktop app (macOS, Linux, Windows)
2. Full keyboard navigation (all features)
3. Error handling & recovery
4. E2E tests (critical flows)
5. Performance profiling

**After Phase 1 release:**
1. Community feedback
2. Bug fixes
3. Plan Phase 2 (GitLab, notifications)

---

## Questions?

**Q: When will GitLab support ship?**
A: Phase 2, estimated 3-4 months after Phase 1 release. No fixed date yet.

**Q: Can I use Bistro offline?**
A: Yes, Phase 1 supports offline read-only. Phase 2 will add offline action queue.

**Q: Will there be a CLI version?**
A: Yes, Phase 4. Currently desktop-only.

**Q: Can I install plugins?**
A: Phase 3 (earliest). Phase 1-2 is core features only.

**Q: What about mobile?**
A: Out of scope for Phase 1-3. Desktop-first strategy. Possible Phase 4+.

**Q: Will you ever support Bitbucket / Azure DevOps?**
A: Possible Phase 3+, but depends on community demand and our capacity.

---

**Last updated:** 2025-04-08

**Next roadmap review:** After Phase 1 release (TBD)
