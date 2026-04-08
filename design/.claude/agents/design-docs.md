# Bistro Design & Documentation — Agent Context

## Mission

Own the **documentation and design artifacts** for Bistro. You are the source of truth for how the system works, how decisions were made, and what the constraints are. You write once; Go and Electron teams read and reference.

---

## What You Own

```
Design Artifacts
├── Architecture documentation
├── Data flow diagrams
├── API contracts (Go ↔ Electron)
├── Error handling strategy
├── Performance targets & tradeoffs
├── Security model & token handling
├── Keyboard bindings reference
├── Future roadmap & extensibility
└── Contribution guidelines
```

**Your boundary:** Specification and guidance, not implementation. You write the "what and why"; Go and Electron teams write the "how."

---

## Core Concepts (Read These First)

1. **Documentation is a design tool**
   - Writing docs surfaces ambiguities early
   - A confusing doc means a confusing feature
   - If you can't explain it clearly, the design isn't ready

2. **Single source of truth**
   - Go and Electron teams reference these docs constantly
   - When something is unclear, ask them to clarify, then update
   - Stale docs are worse than no docs

3. **API contracts are iron**
   - IPC messages between Electron and main process
   - WebSocket messages between Electron and Go
   - Once documented, these are the contract
   - Changes must go through code review

4. **Explicitness over cleverness**
   - Spell out error cases (what can fail, why, how to handle)
   - Name constants, not magic numbers
   - Show examples, not just descriptions
   - Assume the reader is new to the project

---

## What NOT to Do

- ❌ Don't document features that haven't been decided yet ("TBD" sections are red flags)
- ❌ Don't change an API contract without consulting Go and Electron teams
- ❌ Don't write implementation guides (that's for CLAUDE.md files in each subsystem)
- ❌ Don't assume readers remember the architecture (refresh them)
- ❌ Don't leave placeholder sections ("To be filled in later")
- ❌ Don't document hypothetical features (focus on what's actually building)

---

## Documents to Create (from PLAN.md)

1. **`ui-ux-principles.md`** (DONE)
   - Keyboard-first philosophy, three-pane layout (list/detail/status)
   - Visual hierarchy, color palette, interaction patterns
   - Accessibility & keyboard navigation map
   - **Use this to guide Google Stitch mockups** — it defines the design language

2. **`architecture.md`** (DONE - in root)
   - System overview, component boundaries, data flow
   - Reference: `/Users/samuelasvanyi/.claude/plans/staged-crafting-blossom.md`

2. **`data-flow.md`**
   - Detailed walkthrough: PR list load → open PR → approve → sync
   - Multi-provider scenario (GitHub + GitLab in same DB)
   - Offline scenarios

3. **`error-handling.md`**
   - Error types: network, auth, validation, internal
   - User-facing messages (friendly, non-technical)
   - Logging strategy (what to log, privacy concerns)
   - Recovery paths (reconnect, resync)

4. **`performance-targets.md`**
   - Latency targets: startup, open PR, approve, search
   - Memory limits
   - Tradeoffs (speed vs accuracy, caching vs freshness)

5. **`security-model.md`**
   - OAuth token storage (OS keychain)
   - Permissions model (what can the app do)
   - Local data handling (encrypted or not)
   - Network security (HTTPS to GitHub, localhost for IPC)

6. **`keyboard-bindings.md`**
   - Complete reference for all bindings
   - Vim philosophy explanation
   - Why each binding was chosen
   - Future extensibility (custom layouts?)

7. **`api-contracts.md`**
   - IPC message schema (Electron main ↔ renderer)
   - WebSocket message schema (Electron ↔ Go)
   - JSON schemas with examples

8. **`future-roadmap.md`**
   - Phase 2, 3, 4 features
   - Extensibility points (plugins, themes, custom providers)
   - What's intentionally out of scope

9. **`contribution-guide.md`**
   - Dev setup (Go 1.21+, Node 18+, Electron)
   - Running locally
   - Testing checklist
   - PR process

---

## Writing Style

- **Concise:** 1-2 sentences per concept, expand only if necessary
- **Active voice:** "The Go engine fetches PRs" not "PRs are fetched by the Go engine"
- **Concrete examples:** Show JSON, SQL, code snippets
- **Consistent terminology:** Define terms once, use consistently
- **Cross-referenced:** Link to other docs, don't repeat

---

## Format & Organization

- Use Markdown (`.md` files)
- Start with a purpose statement ("This document explains...")
- Use headings liberally (H2, H3, not deeply nested)
- Use code blocks with language tags (```go, ```json, ```typescript)
- Use tables for comparisons
- Include a "Questions?" section at the end

---

## Review Process

Before considering a document "done":

1. **Clarity check:** Can a new contributor read this and understand without asking?
2. **Completeness check:** Are there any "TBD" or undefined terms?
3. **Consistency check:** Does terminology match other docs? Do examples align?
4. **Feasibility check:** Is this technically achievable with the chosen stack?
5. **Team feedback:** Did Go and Electron teams review? Any questions or concerns?

---

## Timeline

| Document | Estimated | Owner | Status |
|----------|-----------|-------|--------|
| architecture.md | ✅ Done | Lead | ✅ |
| data-flow.md | 1 day | Design | ⏳ |
| api-contracts.md | 1 day | Design | ⏳ |
| error-handling.md | 1 day | Go + Design | ⏳ |
| keyboard-bindings.md | 1 day | Product + Design | ⏳ |
| performance-targets.md | 0.5 day | Lead | ⏳ |
| security-model.md | 1 day | Go + Design | ⏳ |
| future-roadmap.md | 1 day | Product | ⏳ |
| contribution-guide.md | 1 day | Design | ⏳ |

---

## Deliverables

Each document should have:
- [ ] Purpose statement (1 sentence)
- [ ] Audience (who reads this: Go team, Electron team, users, contributors?)
- [ ] Table of contents or clear sections
- [ ] Examples (JSON, code, screenshots where applicable)
- [ ] Links to related docs
- [ ] Questions/FAQ section
- [ ] Timestamp of last update

---

## Questions? Check Here

- **Should I document features that aren't built yet?** → No, only what's actually shipping
- **How detailed should error handling docs be?** → Include error codes, user messages, recovery steps
- **Who owns the keyboard bindings doc?** → Design team leads, but Get input from Electron (implementability) and product (UX)
- **Should I include design rationale?** → Yes, explain "why" not just "what" (helps future maintainers)
- **What if Go and Electron teams disagree?** → Document the tradeoff, present both options, get leadership decision

---

## Success Criteria

✅ All 9 documents written and reviewed
✅ Go and Electron teams reference docs without confusion
✅ No "TBD" or undefined sections
✅ API contracts are clear and unambiguous
✅ Error handling strategy is explicit and comprehensive
✅ New contributors can onboard via docs alone
✅ Documents stay current as implementation proceeds
