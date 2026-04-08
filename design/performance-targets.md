# Bistro Performance Targets

**Purpose:** Define latency budgets, memory limits, and performance tradeoffs.

**Audience:** Go team, Electron team, QA for benchmarking.

---

## Latency Targets (User-Perceived)

### App Startup

| Metric | Target | Acceptable | Unacceptable |
|--------|--------|-----------|-------------|
| Click app → First PR list visible | < 500ms | 500-800ms | > 800ms |
| Click app → PRs fully synced | < 2s | 2-3s | > 3s |

**Breakdown:**
- Electron process start: 200ms
- SQLite query (last 50 PRs): 50ms
- React render: 100ms
- Go process wakeup: 150ms (parallel)
- Subtotal: ~500ms for list render

**Sync (parallel, doesn't block UI):**
- Go fetches from GitHub: 1-2s (network dependent)
- SQLite upsert: 100ms
- WebSocket notification: 50ms

### PR List Operations

| Operation | Target |
|-----------|--------|
| Scroll (j/k) | < 16ms (60fps) |
| Search / Filter | < 50ms (local FTS5) |
| Open PR → Diff visible | < 100ms (cached) or < 2s (new) |

### Detail View

| Operation | Target |
|-----------|--------|
| Scroll diff (hjkl) | < 16ms (60fps) |
| Jump to next file (n) | < 100ms |
| Approve PR | < 3s (round-trip) |
| Post comment | < 500ms (local), 2-3s (server confirm) |
| Load fresh comments | < 1s (from cache) or 2-3s (from API) |

### Background Sync

| Operation | Target |
|-----------|--------|
| Fetch PRs from GitHub | 1-2s (depends on repo count & network) |
| Update SQLite | 100ms per 50 PRs |
| Send WebSocket notification | 50ms |
| **Total (no UI blocking)** | < 3s |

---

## Memory Limits

### Baseline (App Idle)

| Scenario | Target |
|----------|--------|
| Electron main + renderer | < 100MB |
| Go process | < 50MB |
| SQLite DB | < 50MB (default) |
| Total (idle) | < 200MB |

### With PRs Open

| Scenario | PRs | Target |
|----------|-----|--------|
| PR list in memory | 50 | 10MB (state only) |
| Diff in memory | 1 large diff | 5MB (Electron) |
| SQLite cache | 500 PRs | 50MB |
| **Total (50 PRs open, 1 diff)** | - | < 250MB |
| **Total (100 PRs open, 5 diffs)** | - | < 500MB |

### Garbage Collection

- Go: GC pause < 50ms, run every 10 minutes
- Electron: React component cleanup, no memory leaks after 8 hours uptime

---

## CPU Usage

| Scenario | Target |
|----------|--------|
| Idle (no sync) | < 1% |
| Background sync | < 10% for 2s |
| Rendering 50 PR rows | < 5% during scroll |
| Diff render (1000 lines) | < 15% |

---

## Network Usage

### Initial Sync (After Restart)

| Action | Data | Target |
|--------|------|--------|
| Fetch 50 PRs metadata | ~50KB (JSON) | 200-500ms |
| Fetch 5 diffs (avg 50KB each) | 250KB | 1-2s |
| Fetch comments | ~20KB | 500ms |
| **Total** | ~320KB | 2-3s |

### Continuous Sync (Every 60s)

| Action | Data | Target |
|--------|------|--------|
| Check for PR updates | 5KB request | 100-200ms |
| WebSocket keep-alive | 1KB ping | 50ms |
| Monthly bandwidth (30 syncs/hour) | ~150KB/day | < 5MB/month |

---

## Database Performance

### SQLite Queries

| Query | Target |
|-------|--------|
| `SELECT * FROM prs WHERE state='open'` | < 50ms |
| `SELECT * FROM comments WHERE pr_id=?` | < 20ms |
| `SELECT * FROM diffs WHERE pr_id=?` | < 10ms |
| FTS5 search on PR title/author | < 50ms |

### Write Performance

| Operation | Target |
|-----------|--------|
| Upsert 50 PRs (batch) | < 100ms |
| Insert comment | < 20ms |
| Update approval status | < 10ms |

---

## Tradeoffs

### Speed vs. Accuracy

**Tradeoff 1: Cache staleness**
```
Accurate but slow (1s):
  Always fetch fresh PRs from GitHub before showing list

Fast but stale (100ms):
  Show cached PRs, fetch in background

Decision: Show cached first, update async
  Reason: Better UX, acceptable staleness (max 60s)
```

**Tradeoff 2: Diff rendering**
```
Accurate (compute on demand):
  `git diff base...head` every time

Fast (cache):
  Store diff for 7 days, reuse

Decision: Cache with validation
  Reason: 95% of PRs don't change diffs
  Fallback: Recompute if head commit changes
```

### Caching vs. Freshness

| Data | Cache | Freshness | Rationale |
|------|-------|-----------|-----------|
| PR list | 60s | User can manually refresh (⟲) | Sync happens in background |
| Diff | 7 days | Invalidate if head commit changes | Most diffs are stable |
| Comments | 5 min | Fetch fresh when detail opens | Important for discussion |
| Approval status | 3s | From approval action | User just approved, show immediately |

### Features to Defer (For Performance)

**Phase 1 (focus on speed):**
- ✓ PR list
- ✓ Diff view
- ✓ Approve
- ✓ Comment
- ✓ Keyboard navigation
- ✓ Offline mode

**Phase 2 (add features, maintain speed):**
- [ ] Search across 1000+ PRs (might need server-side FTS)
- [ ] Bulk actions (approve multiple PRs)
- [ ] Custom themes (rendering overhead)
- [ ] PR suggestions (AI, would slow down)
- [ ] Notification center

---

## Benchmarking Methodology

### Startup Time

```bash
# Measure from app launch to first PR list render
time open -a Bistro
```

### Scroll Performance

```
Count dropped frames during:
- Scrolling PR list (j/k 50 times)
- Scrolling diff (hjkl 100 times)

Target: 0 dropped frames (60fps smooth)
```

### Memory Leak Detection

```
- Open app
- Open 10 PRs, scroll each
- Close all (garbage collect)
- Check memory: should return to baseline
- Repeat 5x: memory shouldn't grow
```

### Network Profile

```
- Throttle to 3G (1 Mbps down, 400 Kbps up)
- Measure sync time
- Should still < 5s for 50 PRs
```

---

## Monitoring & Alerting (Phase 2)

### Metrics to Track

```
- App startup time (p50, p95)
- Scroll frame rate (avg FPS, dropped frames)
- Approval round-trip time
- Memory usage over 8-hour session
- Sync time per repo count
- Error rate by type
```

### Alerting Thresholds

```
- Startup > 2s: investigate
- Scroll FPS < 50: performance regression
- Approval > 5s: likely network issue
- Memory growth > 50MB/hour: leak suspected
- Sync > 5s: GitHub API slow or network issue
```

---

## Load Testing Scenarios

### Scenario 1: Large Monorepo

```
Setup:
- 500 open PRs
- 100 watched repos
- 50 recent diffs in memory

Test:
- Scroll through PR list (no lag)
- Open random PR (< 100ms if cached)
- Memory usage (< 500MB)
```

### Scenario 2: Slow Network

```
Setup:
- Throttle to 2G (100 Kbps)
- 50 open PRs

Test:
- Sync time (should still work, just slower)
- Error handling (retry logic works)
- App remains responsive
```

### Scenario 3: Long Session

```
Setup:
- Keep app open 8 hours
- 1 sync every minute
- User opens/closes 50 PRs

Test:
- Memory stable (no leaks)
- CPU idle when not active
- Final memory ≈ initial memory
```

---

## Questions?

**Q: Why cache diffs for 7 days instead of invalidating daily?**
A: 95% of diffs don't change after PR creation. 7-day window balances freshness with performance.

**Q: What if startup takes 1.5s instead of 500ms?**
A: Acceptable (500-800ms range). Not ideal, but user still sees PR list quickly.

**Q: Should we pre-fetch diffs in background?**
A: Phase 2. Current: only fetch diff when user opens PR.

**Q: How do we measure performance on Windows vs. macOS?**
A: Same benchmarks on each platform. Different hardware profiles may vary, but targets are universal.

**Q: What about mobile / tablet support?**
A: Out of scope (Phase 1 is desktop only).

---

**Last updated:** 2025-04-08
