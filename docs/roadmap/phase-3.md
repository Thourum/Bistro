# Phase 3 — AI Comment Classification

[Roadmap](../ROADMAP.md)

## Phase 3 — AI Comment Classification (the wedge) — **status: implemented 2026-07-12 (3.7 deferred to Team plan)**

> **Implemented 2026-07-12** (PR #22, branch `P3-Comment-Classification`) —
> sub-features 3.1–3.6 + 3.8; **3.7 deferred to the Team plan** (see below).
> Authoritative docs:
> - Spec: `docs/superpowers/specs/2026-07-12-comment-classification-design.md`
> - Plan: `docs/superpowers/plans/2026-07-12-comment-classification.md`
> - User guide: `website/src/pages/docs/comment-classification.mdx`
>
> (History: a 2026-07-11 audit found zero 3.x code; the 2026-06-19 foundation
> spec/plan were recovered from git history and superseded by the docs above.)

This is what makes Bistro the answer to the AI-PR era. Implemented as a rules engine + heuristics, with optional BYOK to push specific tasks to a user-supplied LLM.

**Design principle.** Every classification is deterministic, inspectable, and explainable. The user can always click a comment and see *why* it was tagged as "nit" — the matching rule, the regex, the heuristic.

> **As-built decisions** (where the implementation diverges from the original
> draft below — the draft is left intact for history; the spec is authoritative):
> - **Single engine.** Heuristics (3.4) are not a separate code path — they ship as a
>   built-in YAML (`zz-fallback.yml`) loaded last. The rule schema gained `weight` for
>   weighted heuristic scoring; vendor rules set severity directly.
> - **Resolution is last-match-wins**, not "highest confidence wins." Files load
>   alphabetically; `coderabbit.company.yml` overrides `coderabbit.yml` by loading later.
>   No confidence score in v1 (deferred until conflicts are observed in practice).
> - **No hot-reload / fsnotify.** A coarse `sha256` fingerprint over the rules dir
>   (+ classification mode) is stored per comment; comments re-classify lazily on
>   PR-open when the fingerprint is stale.
> - **Rule distribution = bundled + app auto-update.** Built-ins are `go:embed`-ed,
>   seeded to `~/.config/bistro/rules/` on first launch, then manifest-gated-updated
>   on app upgrade (`.seeded-builtins` tracks what shipped: present built-ins are
>   overwritten, new built-ins arrive, user deletions stay deleted, user files are
>   never touched). No runtime fetch.
> - **Per-PR counter lives in the sidebar** (3.2), not a per-PR header. The overview
>   reorders comments by severity and auto-collapses nits; the diff view shows inline
>   severity markers. Built-in vendors shipped: CodeRabbit, Greptile, fallback
>   heuristics (remaining vendors added as comment samples are collected).
> - **Corpus gate:** 50-comment fixture with a ≥70% heuristic-accuracy CI gate;
>   shipped built-ins score 90%.
> - **BYOK per-task toggles collapsed to one mode setting** (`rules` /
>   `rules+llm` / `llm-all`; `server` is a hidden Team-plan stub) — severity+category
>   are the only LLM jobs in v1, so per-job toggles had nothing to toggle. Official
>   vendor SDKs (`openai-go`, `anthropic-sdk-go`) replace hand-rolled HTTP.

### 3.1 Schema additions — **done (migrations 027–029)**

```sql
ALTER TABLE comments ADD COLUMN author_type TEXT;       -- 'human' | 'bot' | 'unknown'
ALTER TABLE comments ADD COLUMN bot_vendor TEXT;        -- 'coderabbit' | 'greptile' | 'copilot' | …
ALTER TABLE comments ADD COLUMN severity TEXT;          -- 'critical' | 'warning' | 'suggestion' | 'nit' | 'info'
ALTER TABLE comments ADD COLUMN category TEXT;          -- 'security' | 'correctness' | 'performance' | 'style' | 'readability' | 'docs'
ALTER TABLE comments ADD COLUMN classification_source TEXT; -- 'rule:<name>' | 'byok:<provider>' | 'manual'
ALTER TABLE comments ADD COLUMN dedup_group_id TEXT;    -- comments in the same group are duplicates of each other
```

### 3.2 Bot identification — **done**

> As-built: CodeRabbit + Greptile ship as built-in rule files; the remaining
> vendors (Copilot, Qodo, Cursor BugBot, Claude Code, Codex, Sourcery) are
> added as one YAML each when real comment samples are collected. Unknown
> `[bot]` logins classify as `bot`/`unknown`. The counter lives in the sidebar.

**Acceptance.**

- Curated vendor list shipped as data (YAML), updated independently of the binary.
- Top vendors covered at v1.0: CodeRabbit, Greptile, Copilot Code Review, Qodo, Cursor BugBot, Claude Code, Codex, Sourcery.
- Unknown bots (author login ends in `[bot]` or matches heuristics) flagged as `bot:unknown`.
- Per-PR header counter: `Human: 4 · CodeRabbit: 23 · Greptile: 8 · Other bots: 2`.

### 3.3 Severity classification (rules engine) — **done**

> As-built: no confidence score (last-match-wins + weighted tally instead;
> deferred until real conflicts appear), and "hot-reload" is lazy — rule
> edits apply on next PR open via the fingerprint, not via a file watcher.

A YAML-driven rules engine that's hot-reloadable, shippable as data, and extensible by users.

```yaml
# bistro/rules/builtin/coderabbit.yml
name: CodeRabbit
match:
  author: ["coderabbitai[bot]", "coderabbit-ai[bot]"]

classifiers:
  - name: emoji_severity
    pattern: '^(?P<emoji>🔒|⚠️|🛠️|💡)'
    severity_map:
      "🔒": critical
      "⚠️": warning
      "🛠️": suggestion
      "💡": nit

  - name: nitpick_section
    pattern: '<details>.*?Nitpick'
    severity: nit

  - name: explicit_severity_field
    pattern: '\*\*Severity:\*\*\s+(?P<level>\w+)'
    severity_field: level
```

**Acceptance.**

- Rules can match on author, body regex, file path, or combinations.
- Each rule produces severity + category + a confidence score.
- Multiple rules can match; highest-confidence wins, with deterministic tie-breaking.
- A "show classification reason" tooltip shows which rule fired.
- Users can author their own rules under `~/.config/bistro/rules/` and they hot-reload.

### 3.4 Severity heuristics for unlabeled comments — **done (90% on corpus, gate ≥70%)**

Used when a comment doesn't match a vendor-specific rule (humans, unknown bots, bots without severity hints).

**Heuristic library.**

- **Nit signals.** "consider," "you could," "minor," "nit:", "nitpick:", "stylistic," "preference," "personal taste," "would be cleaner" → severity `nit`.
- **Critical signals.** "SQL injection," "XSS," "credential," "secret," "password," "auth bypass," "memory leak," "race condition" + path matches `*/auth/*`, `*/security/*`, `*/payments/*` → severity `critical`.
- **Style signals.** Path matches `*.md`, `*.rst`, comment is purely about formatting → category `style`.
- Heuristics are weighted; final severity is the max of all triggered weights.

**Acceptance.**

- 70%+ accuracy on a held-out set of real review comments (we'll build the test corpus from public PRs in popular OSS repos before v1.0).
- Every heuristic is documented with examples in `docs/classification.md`.

### 3.5 Bulk comment operations — **done**

**User story.**

> As a senior reviewer triaging a PR with 47 comments, I want a single keystroke to dismiss all nits, all comments from one bot, or all comments below a severity threshold, so that I can get to the 3 comments that actually matter.

**Acceptance.**

- `d n` — dismiss all nits (+info) in this PR. *(shipped)*
- `d b` — dismiss all bot comments (per-vendor picker deferred; selection helper already supports it). *(shipped)*
- `f c` / `f w` / `f s` / `f n` — filter to critical / warning / suggestion / nit.
- `f h` — show only human comments.
- All bulk operations are reversible via `u` (undo) within the session.
- Dismissals from FOSS are local-only; they don't write back to the provider unless the user explicitly enables that in settings (per-account opt-in).

### 3.6 Multi-bot deduplication — **done**

**Why it matters.** When a team has CodeRabbit + Greptile + Copilot all running, they routinely flag the same line with overlapping comments. Today the user reads three near-identical paragraphs.

**Acceptance.**

- Comments on the same `(file_path, line_range)` with text similarity > 0.7 (Jaccard or trigram) get a shared `dedup_group_id`.
- The UI renders one merged finding with provenance: "Flagged by CodeRabbit + Greptile + Copilot (3 sources)."
- Each source is expandable individually.
- The most severe rating in the group becomes the group's severity.

### 3.7 PR spam / slop detection — **deferred to Team plan (2026-07-12)**

> **Deferred (2026-07-12).** Two of the five signals need data FOSS doesn't
> sync: commit messages (no commits table) and contributor account age (author
> account created-date isn't fetched). Rather than adding sync surface for a
> single feature, slop scoring moves to the Team plan where it can be computed
> server-side with complete data. The acceptance below is kept for that scope.

**Why it matters.** OSS maintainers in 2025 reported being overwhelmed by AI-generated PRs that look plausible but are slop. curl shut down its bug bounty for this reason.

**Acceptance.**

- A PR-level "slop score" (0–100) computed from:
  - Generic phrases in description ("This PR improves the code quality," "I have made the following changes")
  - Description-to-diff length mismatch
  - Generated-file overlap
  - First-time contributor + account-age < 30 days
  - Commit message pattern (single-line generic, "Update foo.py", etc.)
- Slop score is shown as a sidebar indicator on each PR; doesn't auto-hide anything.
- Filter `f !` — hide PRs with slop score > 70.

### 3.8 BYOK (Bring Your Own Key) AI — **done**

**Acceptance.**

- Settings page: "AI Classification (Optional)" section.
- Provider dropdown: Anthropic, OpenAI, OpenRouter, Ollama (any OpenAI-compatible endpoint).
- API key stored in OS keychain.
- ~~Per-task toggle~~ As-built: one mode setting (rules / rules+llm / llm-all) — severity+category are the only LLM jobs in v1.
- All LLM calls go from the local Go process directly to the provider's API. No traffic through Bistro infrastructure (FOSS doesn't have any).
- Token-usage display in the UI: "This session: 14,230 tokens, est. $0.18."
- Aggressive cache: same comment text → same classification, never re-classify.
- Off by default. Opt-in only. Privacy-conscious users get full functionality with regex-only.

---

