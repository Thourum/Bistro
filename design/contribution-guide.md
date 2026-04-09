# Bistro Contribution Guide

**Purpose:** Help developers set up Bistro for development and understand the contribution process.

**Audience:** Open-source contributors, team members onboarding.

---

## Quick Start

### Prerequisites

```bash
# Required
go version                  # Go 1.21 or later
node --version              # Node 18 or later (includes npm)
git --version               # Git 2.0+

# macOS
brew install go node

# Linux (Ubuntu)
sudo apt-get install golang-1.21 nodejs npm git

# Windows
# Download from golang.org, nodejs.org
```

### Development Setup (5 minutes)

```bash
# Clone the repository
git clone https://github.com/bistro-dev/bistro.git
cd bistro

# Install dependencies
npm install                 # Electron + frontend
go mod download             # Go backend

# Start development
npm run dev                 # Runs both Go and Electron in watch mode

# In another terminal (optional, if above doesn't work)
cd go && go run ./cmd/bistro-sync &
cd .. && npm start
```

**What to expect:**
- Electron window opens (http://localhost:3000)
- Go process listens on WebSocket port 8080
- Hot reload on file changes (Electron + TypeScript)
- Go recompiles on save

### First Test: Verify Setup

```bash
# In Electron window, press '?'
# Should show keyboard bindings

# In browser DevTools (Ctrl+Shift+I)
# Check Console for errors

# In terminal
# Should see logs from both Go and Electron
```

---

## Architecture Overview

```
bistro/
├── go/                      ← Go backend (data sync, GitHub API)
│   ├── cmd/bistro-sync/     ← Entry point
│   ├── internal/
│   │   ├── github/          ← GitHub API client
│   │   ├── store/           ← SQLite logic
│   │   └── engine/          ← Business logic
│   └── go.mod               ← Dependencies
│
├── electron/                ← Electron main process
│   ├── src/
│   │   ├── main.ts          ← Window setup, IPC handlers
│   │   ├── ipc.ts           ← Message schemas
│   │   └── store.ts         ← SQLite wrapper
│   └── package.json
│
├── electron/src/renderer/   ← React frontend
│   ├── components/          ← React components (PR list, detail, diff)
│   ├── pages/               ← Full-page views (Login, PRList, PRDetail, Settings)
│   ├── hooks/               ← Custom hooks
│   ├── store/               ← TanStack Store state management
│   ├── queries/             ← Data fetching helpers
│   ├── utils/               ← Utility functions
│   ├── App.tsx              ← Root component
│   └── main.tsx             ← React entry point
│
├── design/                  ← This documentation
│   ├── PLAN.md
│   ├── ui-ux-principles.md
│   ├── data-flow.md
│   ├── keyboard-bindings.md
│   ├── error-handling.md
│   ├── performance-targets.md
│   ├── security-model.md
│   ├── future-roadmap.md
│   └── contribution-guide.md (this file)
│
└── README.md
```

**Key reference:** See `../architecture.md` (root) for system design.

---

## Code Organization

### Go Backend (`/go`)

**Key packages:**

```go
github.com/bistro-dev/bistro/go/

// Main entry point
cmd/bistro-sync/main.go

// Provider abstraction + GitHub client
internal/provider/
  ├── github/         ← GitHub REST API implementation
  └── mock/           ← Test mock provider

// Data storage
internal/store/
  ├── migrations/     ← SQL schema migrations
  └── ...             ← SQLite queries and table operations

// Sync engine
internal/sync/         ← Main PR sync loop

// Action handlers (approve, comment, request changes)
internal/actions/

// WebSocket server
internal/ws/           ← WebSocket server and event broadcasting
internal/server/       ← HTTP/IPC server

// Event definitions
internal/events/

// Git operations (clone, diff)
internal/git/
```

### Electron Main (`/electron`)

```typescript
electron/src/

// IPC setup
├── main.ts          ← Creates window, handles system events
├── ipc.ts           ← Message schemas & handlers
└── store.ts         ← SQLite integration

// Shared types
├── types.ts         ← Type definitions (Shift+Click for docs)
```

### React Frontend (`/electron/src/renderer`)

```typescript
electron/src/renderer/

// Full-page views
├── pages/
│   ├── LoginPage.tsx        ← GitHub login
│   ├── PRListPage.tsx       ← PR list view
│   ├── PRDetailPage.tsx     ← PR detail + diff
│   └── SettingsPage.tsx     ← User preferences

// Reusable UI components
├── components/
│   ├── PRList.tsx
│   ├── PRListItem.tsx
│   ├── DiffViewer.tsx
│   ├── StatusSidebar.tsx
│   ├── CommandPalette.tsx
│   ├── HelpOverlay.tsx
│   ├── CommentInput.tsx
│   ├── CommentThread.tsx
│   └── StatusBar.tsx

// State management (TanStack Store)
├── store/

// Data fetching
├── queries/

// Custom hooks
├── hooks/

// Utilities
├── utils/

└── App.tsx                  ← Root component
```

---

## Running Tests

### Go Tests

```bash
cd go/

# Run all tests
go test ./...

# Run specific package
go test ./internal/github

# Run with coverage
go test -cover ./...

# Run one test
go test -run TestApprovePR ./internal/engine
```

**Test file naming:**
- Feature: `foo.go` → Test: `foo_test.go`
- Example: `approve.go` → `approve_test.go`

**Test style:**
```go
// Table-driven tests (preferred)
func TestApprovePR(t *testing.T) {
    cases := []struct {
        name    string
        prID    string
        wantErr bool
    }{
        {"valid PR", "123", false},
        {"empty ID", "", true},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            err := ApprovePR(tc.prID)
            if (err != nil) != tc.wantErr {
                t.Errorf("got error %v, want %v", err, tc.wantErr)
            }
        })
    }
}
```

### React / Electron Tests

```bash
# Frontend tests (Vitest + React Testing Library)
npm run test

# Run in watch mode
npm run test:watch

# E2E tests (Playwright)
npm run test:e2e
```

### Manual Testing Checklist

Before submitting PR:

```
[ ] App starts without errors
[ ] Can authenticate with GitHub
[ ] PR list loads (> 0 PRs)
[ ] Can open a PR and view diff
[ ] Can press 'a' to approve (with confirmation)
[ ] Can press 'c' to comment
[ ] Keyboard bindings work (j/k, n/p, ?)
[ ] No console errors (Ctrl+Shift+I)
[ ] No Go process crashes (watch terminal)
[ ] Offline mode works (unplug network, PR list still loads)
```

---

## Code Style

### Go

**Follow idiomatic Go:**

```go
// Good: clear, simple
func (e *Engine) FetchPR(ctx context.Context, id string) (*PR, error) {
    if id == "" {
        return nil, fmt.Errorf("pr_id required")
    }
    // ...
}

// Naming: short, descriptive
var pr *PR
var prList []*PR
var err error

// Errors: explicit, wrapped
if err != nil {
    return fmt.Errorf("fetch PR: %w", err)
}
```

**Tools:**

```bash
# Format code
go fmt ./...

# Lint
golangci-lint run ./...

# Vet (catch bugs)
go vet ./...
```

### TypeScript / React

**Follow Airbnb + React best practices:**

```typescript
// Good: clear typing
function ApprovePR(prID: string): Promise<Result<boolean, Error>> {
    return approvalService.approve(prID)
}

// Naming: camelCase for functions, PascalCase for components
const PRList = () => { ... }
const fetchPRs = async () => { ... }

// Avoid: try/catch for business logic (use Result types)
// Good: Result types everywhere
const result = await fetchPRs()
if (result.isErr()) {
    handleError(result.error)
}
```

**Tools:**

```bash
# Format
npm run format

# Lint
npm run lint

# Type check
npx tsc --noEmit
```

---

## Code Review Process

### Submitting a PR

1. **Create a branch**
   ```bash
   git checkout -b feature/my-feature
   # or
   git checkout -b fix/issue-123
   ```

2. **Write tests first** (TDD)
   ```bash
   # Write failing test
   npm run test:watch    # See it fail

   # Implement feature
   # See test pass

   # Commit
   git add .
   git commit -m "feat: add my feature"
   ```

3. **Run all checks**
   ```bash
   npm run lint
   npm run test
   npm run build
   go test ./...
   ```

4. **Push and create PR**
   ```bash
   git push -u origin feature/my-feature
   # Click link in output to create PR
   ```

5. **PR template** (auto-filled):
   ```markdown
   ## What does this PR do?
   Briefly describe the feature or fix.

   ## Why?
   Explain motivation or the bug being fixed.

   ## Testing
   - [ ] Added tests
   - [ ] Ran full test suite
   - [ ] Manual testing checklist (see above)

   ## Screenshots (if UI change)
   Attach before/after.
   ```

### Code Review Checklist

Reviewers check:

- [ ] Tests exist and pass
- [ ] Code follows style guide (Go idioms, TypeScript conventions)
- [ ] No hardcoded values (use constants)
- [ ] Error handling comprehensive
- [ ] No security issues (input validation, token handling)
- [ ] No performance regression
- [ ] Documentation updated (if needed)

**Feedback styles:**

```
MUST FIX (blocking):
"This could panic if err != nil"

SHOULD FIX (preferred):
"Consider using a Result type here instead of try/catch"

NITPICK (optional):
"Variable name could be more descriptive"
```

---

## Common Development Tasks

### Adding a New Keyboard Binding

**1. Define the binding (Go side: `internal/ipc/handlers.go`)**

```go
func (s *Server) handleKeypress(msg KeypressMessage) error {
    switch msg.Key {
    case "a":
        return s.handleApprove(msg.PRID)
    case "m":
        return s.handleToggleMute(msg.PRID)
    // Add new binding here
    case "x":
        return s.handleNewAction(msg.PRData)
    }
    return nil
}
```

**2. Implement handler logic (Go side)**

```go
func (s *Server) handleNewAction(data interface{}) error {
    // Logic
    return nil
}
```

**3. Update keyboard binding docs (`design/keyboard-bindings.md`)**

Add to appropriate section:

```markdown
| x | Do new action |
```

**4. Update help overlay (`electron/src/renderer/components/HelpOverlay.tsx`)**

```typescript
export const HELP_TEXT = {
    detailView: [
        ...
        { key: 'x', action: 'Do new action' },
    ]
}
```

**5. Test**

```bash
npm run dev
# Press 'x' in detail view
# Verify action works
```

### Adding a New Error Type

**1. Add error code (Go: `internal/engine/errors.go`)**

```go
const (
    ErrMyError ErrorCode = "MY_ERROR"
)
```

**2. Add handler (Electron: `electron/src/renderer/utils/errorHandler.ts`)**

```typescript
case 'MY_ERROR':
    showNotification('User-friendly message')
    break
```

**3. Add test**

```go
func TestMyError(t *testing.T) {
    // Test that error returns correct code
}
```

### Adding a Database Migration

**1. Create migration file (`go/internal/store/migrations/001_initial.sql`)**

```sql
CREATE TABLE IF NOT EXISTS new_table (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL
);
```

**2. Update migration loader (`go/internal/store/migration.go`)**

```go
func (s *Store) migrate() error {
    // Load and run migrations in order
}
```

**3. Update types (`go/internal/store/types.go`)**

```go
type NewTable struct {
    ID   string
    Name string
}
```

**4. Add accessor methods (`go/internal/store/new_table.go`)**

```go
func (s *Store) GetNewTable(ctx context.Context) (*NewTable, error) {
    // Query
}
```

---

## Debugging

### Enable Debug Logging

```bash
# Go backend
export LOG_LEVEL=debug
npm run dev

# Electron
Set DEBUG=* in terminal before running
npm run dev

# React
Console.log freely, use React DevTools (Cmd+Option+J)
```

### Go Debugging

```bash
# Using Delve debugger
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug a test
dlv test ./internal/engine -- -test.run TestApprovePR

# Or use VS Code with Delve extension
```

### Electron Debugging

```bash
# In main process, open DevTools
Ctrl+Shift+I (or Cmd+Option+I on macOS)

# View main process logs
Menu > View > Toggle Developer Tools (Main Process)
```

### Performance Profiling

```go
// Go CPU profile
import _ "net/http/pprof"

// Start pprof server
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()

// Then visit http://localhost:6060/debug/pprof/
```

---

## Deployment & Releases

### Versioning

**SemVer (Semantic Versioning):**
```
v1.2.3
└─ PATCH (bug fixes)
  └─ MINOR (new features)
    └─ MAJOR (breaking changes)
```

### Release Process (Core Team Only)

```bash
# Tag new version
git tag -a v1.0.0 -m "Release v1.0.0"

# Push tag
git push origin v1.0.0

# GitHub Actions builds and publishes release
# (automatic via CI/CD)
```

---

## Getting Help

### Questions?

**In code:**
```go
// Ask a question in comments
// TODO: Is this the right approach?
// (Add context so reviewers can help)
```

**In PR:**
- Add `?` or `question` label
- Reviewers will provide guidance

**In Discord/Discussions:**
- Join our community
- Ask in #development channel

### Common Issues

**Issue:** "Go process keeps crashing"
**Fix:** Check logs (`go run ./cmd/bistro-sync 2>&1 | grep Error`)

**Issue:** "Electron window blank"
**Fix:** Check DevTools (Ctrl+Shift+I), look for React errors

**Issue:** "Database locked error"
**Fix:** Close other Bistro instances, restart

---

## Questions?

**Q: Do I need to sign a CLA?**
A: No, we accept contributions as-is. Read LICENSE before submitting.

**Q: What if I want to work on a big feature?**
A: Open an issue first to discuss approach. DMs welcome.

**Q: How long before my PR is reviewed?**
A: Usually 24-48h. We're a small team, so patience appreciated.

**Q: Can I work on Phase 2 features?**
A: Please focus on Phase 1 bugs/improvements first. Phase 2 planned after v1.0.

---

**Last updated:** 2025-04-08

**Questions? Create an issue on GitHub or ask in Discord.**
