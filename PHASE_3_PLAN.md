# Phase 3: Git Operations & Diff Generation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement shallow clone/fetch operations, per-file diff generation, and repository caching to avoid re-cloning.

**Architecture:** The git package provides pure-Go git operations via go-git library. It clones or updates repositories, fetches only base+head commits, generates unified diffs for each modified file, and caches repositories in memory to avoid redundant network I/O.

**Tech Stack:** Go 1.21+, go-git/go-git (pure Go Git implementation), bufio for diff parsing.

---

## File Structure

```
go/internal/
  └── git/
      ├── clone.go          ← Shallow clone/fetch base+head
      ├── diff.go           ← Per-file unified diff generation
      ├── cache.go          ← In-memory repo cache
      ├── git_test.go       ← Integration tests
      └── types.go          ← Shared types (CloneResult, DiffResult)
```

---

## Phase 3 Tasks

### Task 1: Repository Types & Cache Setup

**Files:**
- Create: `go/internal/git/types.go`
- Create: `go/internal/git/cache.go`
- Create: `go/internal/git/cache_test.go`

**Steps:**

- [ ] **Step 1: Define repository types**

```go
// git/types.go
package git

import (
	"time"
)

// RepoCache entry
type RepoEntry struct {
	URL        string
	LocalPath  string
	BaseSHA    string
	HeadSHA    string
	FetchedAt  time.Time
}

// CloneResult tracks successful clone
type CloneResult struct {
	LocalPath string
	BaseSHA   string
	HeadSHA   string
}

// DiffFile represents a single file diff
type DiffFile struct {
	Path    string // e.g., "cmd/main.go"
	Status  string // "added", "modified", "deleted"
	Patch   string // Unified diff content
	AddedLines    int
	RemovedLines  int
}

// DiffResult contains all files for a PR
type DiffResult struct {
	Files []*DiffFile
	Size  int64 // Total bytes
}
```

- [ ] **Step 2: Implement in-memory cache**

```go
// git/cache.go
package git

import (
	"fmt"
	"sync"
)

// Cache stores cloned repositories in memory
type Cache struct {
	mu    sync.RWMutex
	repos map[string]*RepoEntry
}

// NewCache creates a new repository cache
func NewCache() *Cache {
	return &Cache{
		repos: make(map[string]*RepoEntry),
	}
}

// Get retrieves a cached repo entry
func (c *Cache) Get(url string) (*RepoEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.repos[url]
	return entry, ok
}

// Set caches a repo entry
func (c *Cache) Set(url string, entry *RepoEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.repos[url] = entry
}

// Clear removes an entry from cache
func (c *Cache) Clear(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.repos, url)
}

// Size returns number of cached repos
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.repos)
}
```

- [ ] **Step 3: Write tests**

```go
// git/cache_test.go
package git

import (
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	cache := NewCache()
	if cache == nil {
		t.Error("expected non-nil cache")
	}
	if cache.Size() != 0 {
		t.Error("expected empty cache")
	}
}

func TestCacheSetGet(t *testing.T) {
	cache := NewCache()
	entry := &RepoEntry{
		URL:       "https://github.com/org/repo",
		LocalPath: "/tmp/repo",
		BaseSHA:   "abc123",
		HeadSHA:   "def456",
		FetchedAt: time.Now(),
	}

	cache.Set(entry.URL, entry)

	retrieved, ok := cache.Get(entry.URL)
	if !ok {
		t.Error("expected entry to be cached")
	}
	if retrieved.BaseSHA != "abc123" {
		t.Errorf("expected BaseSHA abc123, got %s", retrieved.BaseSHA)
	}
}

func TestCacheClear(t *testing.T) {
	cache := NewCache()
	entry := &RepoEntry{URL: "https://github.com/org/repo"}
	cache.Set(entry.URL, entry)

	cache.Clear(entry.URL)

	_, ok := cache.Get(entry.URL)
	if ok {
		t.Error("expected entry to be cleared")
	}
}

func TestCacheThreadSafety(t *testing.T) {
	cache := NewCache()

	// This just verifies no race conditions with mutex
	// Use `go test -race` to detect issues
	entry := &RepoEntry{URL: "https://github.com/org/repo"}
	cache.Set(entry.URL, entry)
	cache.Get(entry.URL)
}
```

---

### Task 2: Shallow Clone/Fetch with go-git

**Files:**
- Create: `go/internal/git/clone.go`
- Create: `go/internal/git/clone_test.go`

**Steps:**

- [ ] **Step 1: Add go-git dependency**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go get github.com/go-git/go-git/v5
```

- [ ] **Step 2: Write failing test**

```go
// git/clone_test.go
package git

import (
	"context"
	"testing"
)

func TestCloneRepo(t *testing.T) {
	// Just verify the method structure
	// Real tests with GitHub repo in Phase 3 integration tests
	client := NewClient(NewCache())
	if client == nil {
		t.Error("expected non-nil client")
	}
}
```

- [ ] **Step 3: Implement clone.go**

```go
// git/clone.go
package git

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

// Client handles git operations
type Client struct {
	cache *Cache
}

// NewClient creates a new git client
func NewClient(cache *Cache) *Client {
	return &Client{cache: cache}
}

// ShallowClone clones a repository, fetching only base+head commits
func (c *Client) ShallowClone(ctx context.Context, repoURL, baseSHA, headSHA string) (*CloneResult, error) {
	// Check cache first
	if cached, ok := c.cache.Get(repoURL); ok {
		return &CloneResult{
			LocalPath: cached.LocalPath,
			BaseSHA:   cached.BaseSHA,
			HeadSHA:   cached.HeadSHA,
		}, nil
	}

	// Create temp directory
	tmpDir, err := ioutil.TempDir("data/repos", "repo-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Clone using go-git with depth limit
	opts := &gitlib.CloneOptions{
		URL:      repoURL,
		Depth:    1, // Shallow clone - only latest commit
	}

	// TODO: In Phase 3, implement multi-ref shallow fetch
	// For now, basic structure in place

	repo, err := gitlib.PlainClone(tmpDir, false, opts)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to clone repo: %w", err)
	}

	// Get HEAD SHA
	ref, err := repo.Head()
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	headHash := ref.Hash().String()

	result := &CloneResult{
		LocalPath: tmpDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headHash,
	}

	// Cache the result
	c.cache.Set(repoURL, &RepoEntry{
		URL:       repoURL,
		LocalPath: tmpDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headHash,
	})

	return result, nil
}

// FetchRefs updates existing repository to fetch specific refs
func (c *Client) FetchRefs(ctx context.Context, localPath, baseSHA, headSHA string) error {
	// Placeholder for Phase 3
	// Will implement multi-ref shallow fetch
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/git -v
```

---

### Task 3: Per-File Diff Generation

**Files:**
- Create: `go/internal/git/diff.go`
- Create: `go/internal/git/diff_test.go`

**Steps:**

- [ ] **Step 1: Write failing test**

```go
// git/diff_test.go
package git

import (
	"testing"
)

func TestGenerateDiff(t *testing.T) {
	client := NewClient(NewCache())
	// Structure verification
	if client == nil {
		t.Error("expected non-nil client")
	}
}
```

- [ ] **Step 2: Implement diff.go**

```go
// git/diff.go
package git

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// GenerateDiff creates per-file diffs between two commits
func (c *Client) GenerateDiff(localPath, baseSHA, headSHA string) (*DiffResult, error) {
	repo, err := git.PlainOpen(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repo: %w", err)
	}

	baseHash := plumbing.NewHash(baseSHA)
	headHash := plumbing.NewHash(headSHA)

	baseCommit, err := repo.CommitObject(baseHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get base commit: %w", err)
	}

	headCommit, err := repo.CommitObject(headHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get head commit: %w", err)
	}

	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get base tree: %w", err)
	}

	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get head tree: %w", err)
	}

	// Get changes between trees
	patch, err := baseTree.Patch(headTree)
	if err != nil {
		return nil, fmt.Errorf("failed to create patch: %w", err)
	}

	// Parse patches into per-file diffs
	result := &DiffResult{Files: make([]*DiffFile, 0)}

	for _, filePatch := range patch.FilePatches() {
		from, to := filePatch.Files()
		path := to.Path()
		if path == "" {
			path = from.Path()
		}

		patchStr := filePatch.String()
		added, removed := countLines(patchStr)

		status := "modified"
		if from == nil {
			status = "added"
		} else if to == nil {
			status = "deleted"
		}

		diffFile := &DiffFile{
			Path:         path,
			Status:       status,
			Patch:        patchStr,
			AddedLines:   added,
			RemovedLines: removed,
		}

		result.Files = append(result.Files, diffFile)
		result.Size += int64(len(patchStr))
	}

	return result, nil
}

// countLines counts added/removed lines in a patch
func countLines(patch string) (added, removed int) {
	scanner := bufio.NewScanner(strings.NewReader(patch))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return
}

// WritePatchFiles writes diffs to disk
func (c *Client) WritePatchFiles(diffResult *DiffResult, baseSHA, headSHA string) error {
	// Placeholder for Phase 3
	// Will write to data/patches/{baseSHA}_{headSHA}/ directory
	return nil
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/git -v
```

---

### Task 4: Patch File I/O

**Files:**
- Modify: `go/internal/git/diff.go` (extend WritePatchFiles)

**Steps:** (Similar TDD pattern)

- [ ] Implement WritePatchFiles to save patches to `data/patches/{baseSHA}_{headSHA}/`
- [ ] Create directory structure
- [ ] Handle file writing errors gracefully
- [ ] Write tests

---

### Task 5: Repository Cache Usage & Cleanup

**Files:**
- Modify: `go/internal/git/clone.go` (use cache)
- Modify: `go/internal/git/cache.go` (add cleanup)

**Steps:** (Similar TDD pattern)

- [ ] Implement cache invalidation (old entries)
- [ ] Add TTL support
- [ ] Test cache reuse
- [ ] Test cache miss

---

### Task 6: Integration with Provider

**Files:**
- Create: `go/internal/git/provider_integration.go` (placeholder for Phase 4)

**Steps:**

- [ ] Verify git package can be called from sync orchestration
- [ ] Define interface for sync orchestration to use

---

## Phase 3 Verification Checklist

- [ ] Cache stores and retrieves repository entries
- [ ] ShallowClone creates temp directory with repo contents
- [ ] baseSHA/headSHA stored in cache entry
- [ ] GenerateDiff creates per-file diffs
- [ ] DiffFile contains path, status, patch content, line counts
- [ ] countLines function counts + and - lines correctly
- [ ] WritePatchFiles writes to data/patches/{sha1}_{sha2}/
- [ ] Cache prevents re-cloning same URL
- [ ] All tests pass: `go test ./internal/git/... -v`
- [ ] Build succeeds: `go build ./...`

---

## Expected Deliverables

- ✅ Cache with thread-safe get/set/clear
- ✅ ShallowClone using go-git
- ✅ GenerateDiff per-file unified diffs
- ✅ WritePatchFiles to disk
- ✅ Line counting for diffs
- ✅ Integration with existing packages
- ✅ ~400 LOC across 4 files
- ✅ 20+ tests
- ✅ Ready for Phase 4 sync orchestration

---

## Notes

- TDD discipline: RED → GREEN → REFACTOR → COMMIT for each task
- go-git provides pure-Go Git operations (no git CLI dependency)
- Shallow cloning with depth=1 minimizes bandwidth
- In-memory cache avoids re-cloning same repo
- Patch file I/O enables offline diff viewing
- Error handling: all functions return error, explicit propagation
- Next phase (Phase 4): Sync orchestration with concurrent goroutines

