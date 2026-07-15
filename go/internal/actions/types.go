package actions

import (
	"fmt"
	"strings"
	"time"
)

// ActionCommand defines the command type
type ActionCommand string

const (
	ApproveCommand         ActionCommand = "approve"
	RequestChangesCommand  ActionCommand = "request_changes"
	PostCommentCommand     ActionCommand = "post_comment"
	ReplyCommentCommand    ActionCommand = "reply_comment"
	ResolveThreadCommand   ActionCommand = "resolve_thread"
	UnresolveThreadCommand ActionCommand = "unresolve_thread"
)

// isNonIdempotent reports whether a given action can produce duplicate
// server-side mutations if executed more than once (e.g. creates a new DB
// record on GitHub rather than updating an existing one).
//
//   - post_comment     → each call creates a new comment thread.
//   - reply_comment    → each call creates a new review-thread reply.
//   - request_changes  → each call creates a new review submission.
//   - approve          → uses CreateReview; each call creates a new review record.
//   - resolve_thread, unresolve_thread are idempotent: retrying them
//     when the previous attempt is unknown is safe.
func (a ActionCommand) isNonIdempotent() bool {
	switch a {
	case PostCommentCommand, ReplyCommentCommand, RequestChangesCommand, ApproveCommand:
		return true
	}
	return false
}

// ActionRequest represents a user-requested PR action
type ActionRequest struct {
	PRID   string        // "github:owner/repo:number"
	Action ActionCommand // approve, request_changes, post_comment, resolve_thread, unresolve_thread
	Body   string        // approval comment, change message, or thread node ID for resolve/unresolve
	Line   int           // for post_comment: which line (0 = general comment)
	// InReplyTo is the existing review-comment ID for reply_comment. GitHub accepts
	// the cached domain ID (github-review-comment-N) or raw numeric database ID.
	InReplyTo string
	// ThreadID is the review-thread node ID for reply_comment requests that target
	// a thread instead of a concrete comment. The sync orchestrator resolves it to
	// InReplyTo before provider execution or queueing.
	ThreadID string
}

// ResolveThreadPayload is JSON-serialised into ActionQueue.payload for
// resolve_thread / unresolve_thread actions.
type ResolveThreadPayload struct {
	ThreadID string `json:"thread_id"`
	PRID     string `json:"pr_id"`
}

// ReplyCommentPayload is JSON-serialised into ActionQueue.payload for
// reply_comment actions so replay preserves both user text and target comment.
type ReplyCommentPayload struct {
	Body      string `json:"body"`
	InReplyTo string `json:"in_reply_to"`
	ThreadID  string `json:"thread_id,omitempty"`
}

// Validate checks if ActionRequest has all required fields
func (ar *ActionRequest) Validate() error {
	if ar.PRID == "" {
		return fmt.Errorf("PRID is required")
	}

	switch ar.Action {
	case ApproveCommand:
		// Body optional for approve
		return nil
	case RequestChangesCommand:
		if ar.Body == "" {
			return fmt.Errorf("body is required for request_changes")
		}
		return nil
	case PostCommentCommand:
		if ar.Body == "" {
			return fmt.Errorf("body is required for post_comment")
		}
		return nil
	case ReplyCommentCommand:
		if ar.Body == "" {
			return fmt.Errorf("body is required for reply_comment")
		}
		if ar.InReplyTo == "" && ar.ThreadID == "" {
			return fmt.Errorf("inReplyTo or threadID is required for reply_comment")
		}
		return nil
	case ResolveThreadCommand, UnresolveThreadCommand:
		if ar.Body == "" {
			return fmt.Errorf("body (thread node ID) is required for %s", ar.Action)
		}
		return nil
	default:
		return fmt.Errorf("invalid action: %s", ar.Action)
	}
}

// ActionResult represents the result of an action execution
type ActionResult struct {
	PRID      string // PR ID
	Action    ActionCommand
	Success   bool
	Error     string // error message if !Success
	Timestamp time.Time
	Retries   int // number of retries before success/failure
}

// ErrAmbiguousRemoteSuccess is the sentinel prefix used when a provider call
// may have already mutated GitHub state but context/liveness cancellation
// occurred before local side-effects ran. Queue rows carrying this prefix must
// NOT be retried — doing so risks duplicate mutations for non-idempotent
// actions (post_comment, request_changes). They should be marked permanently
// failed so an operator can reconcile manually if needed.
const ErrAmbiguousRemoteSuccess = "ambiguous remote success: "

// IsAmbiguousRemoteSuccess reports whether the result error indicates that the
// provider action may have already executed on GitHub. Callers (e.g. replay)
// must treat these as terminal rather than retrying.
func (ar *ActionResult) IsAmbiguousRemoteSuccess() bool {
	return strings.HasPrefix(ar.Error, ErrAmbiguousRemoteSuccess)
}

// permanentErrorKeywords are substrings that indicate a permanent failure
// which should not be retried.
var permanentErrorKeywords = []string{
	"status 401", "status: 401", "http 401", "401 unauthorized", "unauthorized",
	"status 403", "status: 403", "http 403", "403 forbidden", "forbidden",
	"status 404", "status: 404", "http 404", "404 not found", "not found",
	"status 400", "status: 400", "http 400", "400 bad request", "bad request",
	"status 422", "status: 422", "http 422", "422 unprocessable", "unprocessable",
	"validation", "invalid",
}

// transientErrorKeywords are substrings that indicate a transient failure
// which is worth retrying.
var transientErrorKeywords = []string{
	// Rate-limit: explicit HTTP-status patterns avoid matching PR numbers like
	// owner/repo#429 in composed provider error messages.
	"status 429", "status: 429", "http 429", "429 too many", "429 rate", "rate limit",
	// ": 429" matches go-github ErrorResponse format (e.g. "POST ...: 429  []").
	// Safe: "#429: ..." has "429" followed by ":", not preceded by ": ".
	": 429",
	// Server errors: anchored patterns plus ": NNN" for go-github format.
	"status 500", "status: 500", "http 500", "500 internal", ": 500",
	"status 502", "status: 502", "http 502", "502 bad gateway", ": 502",
	"status 503", "status: 503", "http 503", "503 service", "service unavailable", ": 503",
	"status 504", "status: 504", "http 504", "504 gateway", ": 504",
	// Transport failures.
	"timeout", "timed out",
	"temporary", "connection reset", "connection refused",
	"network",
}

// IsRecoverable determines if a failed action should be retried.
// Only transient errors (rate limit, timeout, 5xx, network) are recoverable.
// Permanent errors (auth, not found, validation) are not retried.
func (ar *ActionResult) IsRecoverable() bool {
	if ar.Success || ar.Error == "" {
		return false
	}

	lower := strings.ToLower(ar.Error)

	// Explicit permanent errors take priority over transient matches.
	for _, kw := range permanentErrorKeywords {
		if strings.Contains(lower, kw) {
			return false
		}
	}

	for _, kw := range transientErrorKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	// Unknown errors: do not retry to avoid wasting API quota.
	return false
}

// rateLimitKeywords detect explicit HTTP 429 / rate-limit responses.
// These use anchored patterns (e.g. "status 429", ": 429") rather than bare
// "429" to avoid false-positives when the error message contains a PR number
// like "owner/repo#429" or "PR #429".
// ": 429" matches go-github ErrorResponse format ("POST ...: 429  []").
var rateLimitKeywords = []string{
	"status 429", "status: 429", "http 429", "429 too many", "429 rate", ": 429", "rate limit",
}

// IsRateLimit reports whether the failure is an explicit rate-limit response
// (HTTP 429 / "rate limit"). This is distinct from other transient transport
// errors (timeout, 5xx, connection reset) because a 429 proves the provider
// rejected the request before any mutation occurred — the action is safe to
// re-queue and retry after the reset window, unlike an ambiguous transport
// failure where the mutation may already have been applied.
func (ar *ActionResult) IsRateLimit() bool {
	if ar.Success || ar.Error == "" {
		return false
	}
	lower := strings.ToLower(ar.Error)
	for _, kw := range rateLimitKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
