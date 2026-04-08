# Bistro Go Sync Engine — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a WebSocket-based backend sync engine that fetches PR data from GitHub, generates diffs via shallow git clones, persists to SQLite, and streams sync events back to Electron.

**Architecture:** The engine runs as a sidecar process spawned by Electron on app startup. It listens on a WebSocket server, receives sync commands, orchestrates concurrent goroutines to fetch PR data from GitHub API, generates diffs via go-git, writes normalized data to SQLite, and streams progress events back to the UI. The engine exits cleanly when the parent Electron process exits.

**Tech Stack:** Go 1.21+, go-git (pure Go Git operations), go-octokit (GitHub API), zalando/go-keyring (OS keychain), gorilla/websocket, github.com/mattn/go-sqlite3 (CGO SQLite driver), slog (structured logging).

---

## File Structure

```
go/
  go.mod
  go.sum
  cmd/bistro-sync/
    main.go                    ← Entry point, spawn WS server, wait for Electron
  internal/
    provider/
      interface.go             ← Provider interface definition
      models.go                ← Normalized PR, Comment, CommitInfo types
      github/
        auth.go                ← OAuth device flow, token management
        prs.go                 ← ListPRs, GetPR implementations
        comments.go            ← GetComments, normalize to domain types
        ci.go                  ← GetCIStatus (check runs normalization)
        actions.go             ← Approve, RequestChanges, PostComment
    git/
      clone.go                 ← Shallow clone/fetch base+head
      diff.go                  ← Per-file unified diff generation
      cache.go                 ← In-memory repo cache (avoid re-clones)
    store/
      db.go                    ← SQLite connection pool, lifecycle
      migrations.go            ← Schema setup (schema.sql embedded)
      prs.go                   ← Insert/update PR rows
      comments.go              ← Insert/update comment rows
      files.go                 ← Insert/update file metadata + patch paths
    keychain/
      keychain.go              ← Wrapper around zalando/go-keyring
      token.go                 ← Store/retrieve OAuth tokens by provider
    ws/
      server.go                ← WebSocket lifecycle, port exposure
      handlers.go              ← Command handlers: sync, authenticate, approve, etc.
      events.go                ← Broadcast event functions (progress, errors)
    logger/
      logger.go                ← slog wrapper, no sensitive data logging
  tests/
    provider_mock.go           ← Mock provider for unit tests
    fixtures.go                ← Test data, canned GitHub responses
  data/
    patches/                   ← Generated patches: {base_sha}_{head_sha}/...
    sqlite.db                  ← SQLite database (created at runtime)
```

---

## Phase 1: Core Infrastructure

### Task 1: Project Scaffold & Go Module Init

**Files:**
- Create: `go/go.mod`
- Create: `go/.gitignore`
- Create: `go/cmd/bistro-sync/main.go` (empty entry point)

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go mod init github.com/samuelasvanyi/bistro
```

Expected output: Creates `go.mod` with module declaration.

- [ ] **Step 2: Create .gitignore**

```bash
cat > /Users/samuelasvanyi/dev/bistro/go/.gitignore << 'EOF'
# Binaries
bistro-sync
*.exe
*.dll
*.dylib

# Test binaries
*.test

# Go cache
.go/

# IDE
.idea/
.vscode/
*.swp
*.swo

# Database & patches
data/sqlite.db
data/patches/

# OS
.DS_Store
Thumbs.db

# Logs
*.log
EOF
```

- [ ] **Step 3: Create empty main.go**

```bash
mkdir -p /Users/samuelasvanyi/dev/bistro/go/cmd/bistro-sync
cat > /Users/samuelasvanyi/dev/bistro/go/cmd/bistro-sync/main.go << 'EOF'
package main

import (
	"fmt"
)

func main() {
	fmt.Println("bistro-sync: starting")
}
EOF
```

- [ ] **Step 4: Test build**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go build -o bistro-sync ./cmd/bistro-sync
```

Expected: Binary created, no errors.

- [ ] **Step 5: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
git add go.mod .gitignore cmd/
git commit -m "chore: initialize Go module and project scaffold"
```

---

### Task 2: Logger Setup

**Files:**
- Create: `go/internal/logger/logger.go`
- Create: `go/internal/logger/logger_test.go`

- [ ] **Step 1: Write failing test for logger**

```bash
mkdir -p /Users/samuelasvanyi/dev/bistro/go/internal/logger
cat > /Users/samuelasvanyi/dev/bistro/go/internal/logger/logger_test.go << 'EOF'
package logger

import (
	"testing"
)

func TestNewLogger(t *testing.T) {
	log := New()
	if log == nil {
		t.Error("expected logger, got nil")
	}
}

func TestLogInfo(t *testing.T) {
	log := New()
	log.Info("test message", "key", "value")
	// Just verify it doesn't panic
}

func TestLogError(t *testing.T) {
	log := New()
	log.Error("test error", "err", "test error message")
	// Just verify it doesn't panic
}
EOF
```

- [ ] **Step 2: Run test (should fail)**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go test ./internal/logger -v
```

Expected: FAIL - logger package undefined.

- [ ] **Step 3: Implement logger wrapper**

```bash
cat > /Users/samuelasvanyi/dev/bistro/go/internal/logger/logger.go << 'EOF'
package logger

import (
	"log/slog"
	"os"
)

type Logger struct {
	*slog.Logger
}

func New() *Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	return &Logger{slog.New(handler)}
}

// Info logs an info-level message
func (l *Logger) Info(msg string, args ...interface{}) {
	l.Logger.Info(msg, args...)
}

// Error logs an error-level message
func (l *Logger) Error(msg string, args ...interface{}) {
	l.Logger.Error(msg, args...)
}

// Debug logs a debug-level message
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.Logger.Debug(msg, args...)
}
EOF
```

- [ ] **Step 4: Run tests (should pass)**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go test ./internal/logger -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
git add internal/logger/
git commit -m "feat: add logger wrapper using slog"
```

---

### Task 3: SQLite Database Setup

**Files:**
- Create: `go/internal/store/db.go`
- Create: `go/internal/store/migrations.go`
- Create: `go/internal/store/db_test.go`

- [ ] **Step 1: Add SQLite dependency**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go get github.com/mattn/go-sqlite3
```

- [ ] **Step 2: Write test for database connection**

```bash
cat > /Users/samuelasvanyi/dev/bistro/go/internal/store/db_test.go << 'EOF'
package store

import (
	"os"
	"testing"
)

func TestOpenDB(t *testing.T) {
	// Use in-memory SQLite for testing
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Verify connection works
	err = db.Ping()
	if err != nil {
		t.Fatalf("failed to ping db: %v", err)
	}
}

func TestMigrations(t *testing.T) {
	db, _ := OpenDB(":memory:")
	defer db.Close()

	err := Migrate(db)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify tables exist
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='prs'").Scan(&count)
	if err != nil || count == 0 {
		t.Error("prs table not created")
	}
}
EOF
```

- [ ] **Step 3: Run test (should fail)**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go test ./internal/store -v
```

Expected: FAIL - functions not defined.

- [ ] **Step 4: Implement database connection**

```bash
cat > /Users/samuelasvanyi/dev/bistro/go/internal/store/db.go << 'EOF'
package store

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

func OpenDB(dsn string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// Verify connection
	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}

	return &DB{sqlDB}, nil
}

func (db *DB) Ping() error {
	return db.DB.Ping()
}

func (db *DB) Close() error {
	return db.DB.Close()
}
EOF
```

- [ ] **Step 5: Implement migrations**

```bash
cat > /Users/samuelasvanyi/dev/bistro/go/internal/store/migrations.go << 'EOF'
package store

const schema = `
CREATE TABLE IF NOT EXISTS prs (
	id TEXT PRIMARY KEY,
	provider TEXT NOT NULL,
	owner TEXT NOT NULL,
	repo TEXT NOT NULL,
	number INTEGER NOT NULL,
	title TEXT NOT NULL,
	body TEXT,
	author TEXT NOT NULL,
	state TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	merged_at DATETIME,
	base_branch TEXT NOT NULL,
	head_branch TEXT NOT NULL,
	base_sha TEXT NOT NULL,
	head_sha TEXT NOT NULL,
	comment_count INTEGER DEFAULT 0,
	ci_status TEXT,
	draft INTEGER DEFAULT 0,
	UNIQUE(provider, owner, repo, number)
);

CREATE TABLE IF NOT EXISTS comments (
	id TEXT PRIMARY KEY,
	pr_id TEXT NOT NULL,
	author TEXT NOT NULL,
	body TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	line_number INTEGER,
	file_path TEXT,
	FOREIGN KEY(pr_id) REFERENCES prs(id),
	UNIQUE(id)
);

CREATE TABLE IF NOT EXISTS files (
	id TEXT PRIMARY KEY,
	pr_id TEXT NOT NULL,
	file_path TEXT NOT NULL,
	status TEXT NOT NULL,
	additions INTEGER DEFAULT 0,
	deletions INTEGER DEFAULT 0,
	patch_path TEXT,
	FOREIGN KEY(pr_id) REFERENCES prs(id),
	UNIQUE(pr_id, file_path)
);

CREATE INDEX IF NOT EXISTS idx_prs_updated_at ON prs(updated_at);
CREATE INDEX IF NOT EXISTS idx_comments_pr_id ON comments(pr_id);
CREATE INDEX IF NOT EXISTS idx_files_pr_id ON files(pr_id);
`

func Migrate(db *DB) error {
	_, err := db.Exec(schema)
	return err
}
EOF
```

- [ ] **Step 6: Run tests (should pass)**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go test ./internal/store -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
git add internal/store/ go.mod go.sum
git commit -m "feat: add SQLite database setup and migrations"
```

---

### Task 4: WebSocket Server Scaffold

**Files:**
- Create: `go/internal/ws/server.go`
- Create: `go/internal/ws/server_test.go`

- [ ] **Step 1: Add WebSocket dependency**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go get github.com/gorilla/websocket
```

- [ ] **Step 2: Write test for WebSocket server startup**

```bash
mkdir -p /Users/samuelasvanyi/dev/bistro/go/internal/ws
cat > /Users/samuelasvanyi/dev/bistro/go/internal/ws/server_test.go << 'EOF'
package ws

import (
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	server := NewServer()
	if server == nil {
		t.Error("expected server, got nil")
	}
}

func TestStartServer(t *testing.T) {
	server := NewServer()
	go func() {
		err := server.Start(":0") // Random port
		if err != nil && err.Error() != "server stopped" {
			t.Errorf("unexpected error: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	err := server.Stop()
	if err != nil {
		t.Errorf("failed to stop server: %v", err)
	}
}
EOF
```

- [ ] **Step 3: Run test (should fail)**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go test ./internal/ws -v
```

Expected: FAIL - functions not defined.

- [ ] **Step 4: Implement WebSocket server**

```bash
cat > /Users/samuelasvanyi/dev/bistro/go/internal/ws/server.go << 'EOF'
package ws

import (
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type Server struct {
	listener   net.Listener
	upgrader   websocket.Upgrader
	mu         sync.Mutex
	stopChan   chan struct{}
	httpServer *http.Server
}

func NewServer() *Server {
	return &Server{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins (localhost only in production)
			},
		},
		stopChan: make(chan struct{}),
	}
}

func (s *Server) Start(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)

	s.httpServer = &http.Server{Handler: mux}

	go func() {
		s.httpServer.Serve(s.listener)
	}()

	return nil
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Keep connection open
	for {
		select {
		case <-s.stopChan:
			return
		default:
			var msg map[string]interface{}
			err := conn.ReadJSON(&msg)
			if err != nil {
				return
			}
		}
	}
}
EOF
```

- [ ] **Step 5: Run tests (should pass)**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go test ./internal/ws -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
git add internal/ws/ go.mod go.sum
git commit -m "feat: add WebSocket server scaffold"
```

---

### Task 5: OS Keychain Integration

**Files:**
- Create: `go/internal/keychain/keychain.go`
- Create: `go/internal/keychain/token.go`
- Create: `go/internal/keychain/keychain_test.go`

- [ ] **Step 1: Add keychain dependency**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go get github.com/zalando/go-keyring
```

- [ ] **Step 2: Write test for keychain storage**

```bash
mkdir -p /Users/samuelasvanyi/dev/bistro/go/internal/keychain
cat > /Users/samuelasvanyi/dev/bistro/go/internal/keychain/keychain_test.go << 'EOF'
package keychain

import (
	"testing"
)

func TestStoreAndRetrieveToken(t *testing.T) {
	service := "bistro-test"
	provider := "github"
	token := "test-token-value"

	// Store
	err := StoreToken(service, provider, token)
	if err != nil {
		t.Fatalf("failed to store token: %v", err)
	}

	// Retrieve
	retrieved, err := GetToken(service, provider)
	if err != nil {
		t.Fatalf("failed to retrieve token: %v", err)
	}

	if retrieved != token {
		t.Errorf("expected %s, got %s", token, retrieved)
	}

	// Cleanup
	DeleteToken(service, provider)
}

func TestDeleteToken(t *testing.T) {
	service := "bistro-test"
	provider := "github"
	token := "test-token-value"

	StoreToken(service, provider, token)
	err := DeleteToken(service, provider)
	if err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}

	_, err = GetToken(service, provider)
	if err == nil {
		t.Error("expected error retrieving deleted token")
	}
}
EOF
```

- [ ] **Step 3: Run test (should fail)**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go test ./internal/keychain -v
```

Expected: FAIL - functions not defined.

- [ ] **Step 4: Implement keychain wrapper**

```bash
cat > /Users/samuelasvanyi/dev/bistro/go/internal/keychain/keychain.go << 'EOF'
package keychain

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

// StoreToken stores an OAuth token in the OS keychain
func StoreToken(service, provider, token string) error {
	key := fmt.Sprintf("%s:%s", service, provider)
	return keyring.Set(service, key, token)
}

// GetToken retrieves an OAuth token from the OS keychain
func GetToken(service, provider string) (string, error) {
	key := fmt.Sprintf("%s:%s", service, provider)
	return keyring.Get(service, key)
}

// DeleteToken removes an OAuth token from the OS keychain
func DeleteToken(service, provider string) error {
	key := fmt.Sprintf("%s:%s", service, provider)
	return keyring.Delete(service, key)
}
EOF
```

- [ ] **Step 5: Create token management wrapper**

```bash
cat > /Users/samuelasvanyi/dev/bistro/go/internal/keychain/token.go << 'EOF'
package keychain

const BistroService = "bistro-sync"

// TokenManager provides a simplified interface for token storage
type TokenManager struct {
	service string
}

func NewTokenManager() *TokenManager {
	return &TokenManager{service: BistroService}
}

func (tm *TokenManager) StoreGitHubToken(token string) error {
	return StoreToken(tm.service, "github", token)
}

func (tm *TokenManager) GetGitHubToken() (string, error) {
	return GetToken(tm.service, "github")
}

func (tm *TokenManager) DeleteGitHubToken() error {
	return DeleteToken(tm.service, "github")
}
EOF
```

- [ ] **Step 6: Run tests (may skip on non-macOS if keyring unavailable)**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go test ./internal/keychain -v -short
```

Expected: PASS (may require OS keyring available).

- [ ] **Step 7: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
git add internal/keychain/ go.mod go.sum
git commit -m "feat: add OS keychain integration for token storage"
```

---

### Task 6: Main Entry Point with Graceful Shutdown

**Files:**
- Modify: `go/cmd/bistro-sync/main.go`
- Create: `go/internal/server/app.go`

- [ ] **Step 1: Create app orchestrator**

```bash
mkdir -p /Users/samuelasvanyi/dev/bistro/go/internal/server
cat > /Users/samuelasvanyi/dev/bistro/go/internal/server/app.go << 'EOF'
package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/samuelasvanyi/bistro/internal/logger"
	"github.com/samuelasvanyi/bistro/internal/store"
	"github.com/samuelasvanyi/bistro/internal/ws"
)

type App struct {
	log      *logger.Logger
	db       *store.DB
	wsServer *ws.Server
}

func New() *App {
	return &App{
		log:      logger.New(),
		wsServer: ws.NewServer(),
	}
}

func (a *App) Run(ctx context.Context) error {
	// Initialize database
	var err error
	a.db, err = store.OpenDB("data/sqlite.db")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer a.db.Close()

	// Run migrations
	if err := store.Migrate(a.db); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	a.log.Info("database initialized")

	// Start WebSocket server on random port
	if err := a.wsServer.Start(":0"); err != nil {
		return fmt.Errorf("failed to start ws server: %w", err)
	}

	a.log.Info("websocket server started")

	// Wait for signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	<-sigChan
	a.log.Info("shutdown signal received")

	return a.wsServer.Stop()
}
EOF
```

- [ ] **Step 2: Update main.go**

```bash
cat > /Users/samuelasvanyi/dev/bistro/go/cmd/bistro-sync/main.go << 'EOF'
package main

import (
	"context"
	"log"

	"github.com/samuelasvanyi/bistro/internal/server"
)

func main() {
	app := server.New()
	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("app error: %v", err)
	}
}
EOF
```

- [ ] **Step 3: Create data directory**

```bash
mkdir -p /Users/samuelasvanyi/dev/bistro/go/data/patches
```

- [ ] **Step 4: Test build**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
go build -o bistro-sync ./cmd/bistro-sync
```

Expected: Binary created, no errors.

- [ ] **Step 5: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/go
git add cmd/ internal/server/ data/
git commit -m "feat: add main entry point with graceful shutdown"
```

---

## Phase 1 Verification Checklist

- [ ] `go mod init` succeeded
- [ ] Binary builds without errors: `go build -o bistro-sync ./cmd/bistro-sync`
- [ ] SQLite database created in `data/sqlite.db` on first run
- [ ] Logger outputs info/error messages to stderr
- [ ] WebSocket server starts on random port
- [ ] SIGTERM/SIGINT handled gracefully (no hanging)
- [ ] All Phase 1 tests pass: `go test ./...`

---

## Next Phases (Planning Reference)

**Phase 2:** Provider Interface & GitHub Adapter (fetch PR data)
**Phase 3:** Git Operations & Diff Generation (shallow clone + patches)
**Phase 4:** Sync Orchestration (concurrent goroutines + SQLite writes)
**Phase 5:** Actions & PR Modifications (approve, comment)

---

## Notes

- Each task produces working, testable code independently
- Tests use in-memory SQLite where possible to avoid disk I/O
- Commits are frequent and descriptive
- Use `@superpowers:test-driven-development` skill for TDD workflow during implementation
- Use `@everything-claude-code:go-review` for code review after major phases
- Use `@everything-claude-code:go-build` if build errors occur

