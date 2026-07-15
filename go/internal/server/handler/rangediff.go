package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// rangeDiffRunner is the interface for running a range-diff.
// Satisfied by *git.RangeDiffer.
type rangeDiffRunner interface {
	Available() bool
	FallbackReason() string
	RunRangeDiff(ctx context.Context, repoPath, baseStart, baseEnd, headStart, headEnd string) (git.RangeDiffResult, error)
}

// lastReviewedReader returns the full patchset identity the user last reviewed.
// Satisfied by *store.DB (GetLastReviewedPatchset).
type lastReviewedReader interface {
	GetLastReviewedPatchset(ctx context.Context, prID, login string) (store.LastReviewedRecord, bool, error)
}

// RangeDiffHandlerDeps bundles dependencies for the REQUEST_RANGE_DIFF handler.
// All fields except Runner are optional; their absence causes graceful degradation.
type RangeDiffHandlerDeps struct {
	Runner       rangeDiffRunner
	Patchsets    patchsetLister     // to resolve patchset numbers → SHAs
	LastReviewed lastReviewedReader // for since_last_review path
	CurrentUser  currentUserReader  // to resolve login for since_last_review
	PlainDiff    diffReader         // fallback: stored per-file patches from DiffStore
}

// NewRangeDiffHandler returns a WS handler for REQUEST_RANGE_DIFF.
//
// # Request
//
//	{
//	  "pr_id":             "github:owner/repo:N",
//	  "from_patchset":     1,     // patchset number to diff FROM; 0 = auto
//	  "to_patchset":       2,     // patchset number to diff TO;   0 = latest
//	  "since_last_review": false  // if true: derive from_patchset from last-reviewed patchset
//	}
//
// Auto-resolution:
//   - to_patchset==0   → latest observed patchset
//   - from_patchset==0 → to_patchset - 1 (clamped to 1)
//   - since_last_review==true → read last-reviewed patchset identity (number if known,
//     else SHA-pair match); use as from_patchset. Ambiguous legacy records degrade.
//
// # Response type: RANGE_DIFF_READY
//
//	{
//	  "pr_id":           "...",
//	  "from_patchset":   1,
//	  "to_patchset":     2,
//	  "from_sha":        "<40-char hex>",
//	  "to_sha":          "<40-char hex>",
//	  "raw":             "<diff text or empty>",
//	  "degraded":        true,
//	  "fallback_reason": "<human-readable reason>"
//	}
//
// Fallback raw semantics (Blocker 3):
//   - Identical pair (from==to): raw="" — nothing to diff.
//   - Latest sequential pair (from=latest-1, to=latest): raw=stored plain diff, clearly labeled.
//   - All other pairs: raw="" — the stored diff covers a different range and would be misleading.
//
// This handler never returns a WS handler error in the degraded path.
func NewRangeDiffHandler(rd rangeDiffRunner, log *slog.Logger) ws.MessageHandler {
	return NewRangeDiffHandlerWithDeps(RangeDiffHandlerDeps{Runner: rd}, log)
}

// NewRangeDiffHandlerWithDeps creates the handler with full dependencies.
func NewRangeDiffHandlerWithDeps(deps RangeDiffHandlerDeps, log *slog.Logger) ws.MessageHandler {
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req struct {
			PRID            string `json:"pr_id"`
			FromPatchset    int    `json:"from_patchset"`
			ToPatchset      int    `json:"to_patchset"`
			SinceLastReview bool   `json:"since_last_review"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse REQUEST_RANGE_DIFF: %w", err)
		}
		if req.PRID == "" {
			return nil, fmt.Errorf("REQUEST_RANGE_DIFF requires pr_id")
		}

		// ── Server-side patchset resolution ──────────────────────────────────
		var patchsets []store.Patchset
		if deps.Patchsets != nil {
			var err error
			patchsets, err = deps.Patchsets.ListPatchsets(ctx, req.PRID)
			if err != nil {
				logger.Error("list patchsets failed", "pr_id", req.PRID, "error", err)
				return emptyDegraded(msg.ID, req.PRID, 0, 0, "", "",
					fmt.Sprintf("could not load patchsets: %v", err)), nil
			}
		}

		if len(patchsets) == 0 {
			return emptyDegraded(msg.ID, req.PRID, 0, 0, "", "",
				"no patchsets recorded yet for this PR"), nil
		}

		// Resolve to_patchset: 0 → latest.
		to := req.ToPatchset
		if to == 0 || to > patchsets[len(patchsets)-1].Number {
			to = patchsets[len(patchsets)-1].Number
		}

		// Resolve from_patchset.
		from := req.FromPatchset
		if req.SinceLastReview && deps.LastReviewed != nil && deps.CurrentUser != nil {
			login, ok, _ := deps.CurrentUser.GetCurrentUser(ctx, "github")
			if !ok || login == "" {
				return emptyDegraded(msg.ID, req.PRID, 0, to, "", "",
					"since_last_review: no authenticated user"), nil
			}
			rec, found, err := deps.LastReviewed.GetLastReviewedPatchset(ctx, req.PRID, login)
			if err != nil {
				return emptyDegraded(msg.ID, req.PRID, 0, to, "", "",
					fmt.Sprintf("since_last_review: GetLastReviewedPatchset failed: %v", err)), nil
			}
			if !found || rec.SHA == "" {
				return emptyDegraded(msg.ID, req.PRID, 0, to, "", "",
					"since_last_review: no previous review recorded"), nil
			}

			// Resolve from_patchset using the stored patchset identity.
			if rec.PatchsetNum > 0 {
				// Authoritative: patchset number known — direct lookup, no ambiguity.
				from = rec.PatchsetNum
			} else {
				// Legacy record (patchset_number=NULL): match by (head_sha, base_sha).
				// If base_sha is unknown we can only match by head_sha, which may be
				// ambiguous when multiple patchsets share the same head (e.g. rebase
				// changed base without changing head). Detect and degrade.
				matches := findPatchsetsByHeadAndBase(patchsets, rec.SHA, rec.BaseSHA)
				switch len(matches) {
				case 0:
					// SHA no longer in patchsets (e.g. force-push rewritten history).
					from = patchsets[0].Number
					logger.Warn("since_last_review: legacy SHA not found in patchsets, using first",
						"pr_id", req.PRID, "login", login, "last_sha", rec.SHA)
				case 1:
					from = matches[0].Number
				default:
					// Ambiguous: multiple patchsets with the same head_sha —
					// cannot determine which one the user reviewed without patchset_number.
					return emptyDegraded(msg.ID, req.PRID, 0, to, "", "",
						fmt.Sprintf(
							"since_last_review: ambiguous — %d patchsets share head_sha %s"+
								" (legacy record without patchset_number); re-review to record exact identity",
							len(matches), rec.SHA[:min7(rec.SHA)])), nil
				}
			}
		}
		if from == 0 {
			from = to - 1
			if from < 1 {
				from = 1
			}
		}

		// Map numbers to patchset rows.
		fromPS := patchsetByNumber(patchsets, from)
		toPS := patchsetByNumber(patchsets, to)
		if fromPS == nil || toPS == nil {
			return emptyDegraded(msg.ID, req.PRID, from, to, "", "",
				fmt.Sprintf("patchset not found: from=%d to=%d (have %d patchsets)",
					from, to, len(patchsets))), nil
		}

		fromSHA := fromPS.HeadSHA
		toSHA := toPS.HeadSHA

		// ── Identical pair → no content ───────────────────────────────────────
		if from == to {
			return emptyDegraded(msg.ID, req.PRID, from, to, fromSHA, toSHA,
				"selected patchsets are identical — nothing to diff"), nil
		}

		logger.Debug("range-diff resolved",
			"pr_id", req.PRID, "from", from, "to", to,
			"from_sha", fromSHA[:min7(fromSHA)], "to_sha", toSHA[:min7(toSHA)])

		// ── git range-diff (not available: no local clone at query time) ─────
		if deps.Runner != nil && deps.Runner.Available() {
			logger.Debug("range-diff capability available but no local clone at query time; degrading",
				"pr_id", req.PRID)
		}

		// ── Pair-specific fallback (Blocker 3) ────────────────────────────────
		// The stored plain diff (pr_diffs) covers the latest sync snapshot.
		// Only serve it as a fallback when the requested pair IS the current latest
		// sequential pair (from = latest-1, to = latest) — any other pair would
		// return stale or out-of-range content and mislead the UI.
		latest := patchsets[len(patchsets)-1].Number
		if deps.PlainDiff != nil && to == latest && from == latest-1 {
			unified, err := deps.PlainDiff.GetUnifiedDiff(req.PRID)
			if err == nil && unified != "" {
				return labeledDegraded(msg.ID, req.PRID, from, to, fromSHA, toSHA,
					"using stored plain diff for current pair"+
						" (range-diff requires local repo clone not available at query time)",
					unified), nil
			}
		}

		// Non-current, non-sequential, or single-patchset pair: return empty raw
		// with an informative reason. Do not serve a misleading current diff.
		reason := fmt.Sprintf(
			"pair-specific diff unavailable for P%d→P%d"+
				" (stored diff only covers latest pair P%d→P%d;"+
				" range-diff requires local repo clone)",
			from, to, latest-1, latest)
		if latest == 1 {
			reason = "only one patchset recorded; range-diff requires at least two"
		}
		return emptyDegraded(msg.ID, req.PRID, from, to, fromSHA, toSHA, reason), nil
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

// patchsetByNumber finds a patchset by its number in an ordered slice.
func patchsetByNumber(patchsets []store.Patchset, number int) *store.Patchset {
	for i := range patchsets {
		if patchsets[i].Number == number {
			return &patchsets[i]
		}
	}
	return nil
}

// findPatchsetsByHeadAndBase returns patchsets matching both head_sha and base_sha.
// When baseSHA is empty (legacy), falls back to head-sha-only matching.
func findPatchsetsByHeadAndBase(patchsets []store.Patchset, headSHA, baseSHA string) []store.Patchset {
	var out []store.Patchset
	for _, ps := range patchsets {
		if ps.HeadSHA != headSHA {
			continue
		}
		if baseSHA == "" || ps.BaseSHA == baseSHA {
			out = append(out, ps)
		}
	}
	return out
}

func min7(s string) int {
	if len(s) < 7 {
		return len(s)
	}
	return 7
}

// emptyDegraded builds a RANGE_DIFF_READY response with degraded=true and raw="".
// Use for identical pairs, non-current pairs, or error conditions where returning
// the stored diff would be misleading.
func emptyDegraded(msgID, prID string, fromPS, toPS int, fromSHA, toSHA, reason string) interface{} {
	return buildRangeDiffReady(msgID, prID, fromPS, toPS, fromSHA, toSHA, "", true, reason)
}

// labeledDegraded builds a RANGE_DIFF_READY response with degraded=true and
// a non-empty raw from the stored plain diff — only for the current sequential pair.
func labeledDegraded(msgID, prID string, fromPS, toPS int, fromSHA, toSHA, reason, raw string) interface{} {
	return buildRangeDiffReady(msgID, prID, fromPS, toPS, fromSHA, toSHA, raw, true, reason)
}

func buildRangeDiffReady(msgID, prID string, fromPS, toPS int, fromSHA, toSHA, raw string, degraded bool, reason string) interface{} {
	return map[string]interface{}{
		"id":   msgID,
		"type": "RANGE_DIFF_READY",
		"data": map[string]interface{}{
			"pr_id":           prID,
			"from_patchset":   fromPS,
			"to_patchset":     toPS,
			"from_sha":        fromSHA,
			"to_sha":          toSHA,
			"raw":             raw,
			"degraded":        degraded,
			"fallback_reason": reason,
		},
	}
}
