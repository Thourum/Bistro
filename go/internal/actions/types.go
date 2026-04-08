package actions

import (
	"fmt"
	"strings"
	"time"
)

// ActionCommand defines the command type
type ActionCommand string

const (
	ApproveCommand        ActionCommand = "approve"
	RequestChangesCommand ActionCommand = "request_changes"
	PostCommentCommand    ActionCommand = "post_comment"
)

// ActionRequest represents a user-requested PR action
type ActionRequest struct {
	PRID   string        // "github:owner/repo:number"
	Action ActionCommand // approve, request_changes, post_comment
	Body   string        // approval comment or change request message
	Line   int           // for post_comment: which line (0 = general comment)
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
			return fmt.Errorf("Body is required for request_changes")
		}
		return nil
	case PostCommentCommand:
		if ar.Body == "" {
			return fmt.Errorf("Body is required for post_comment")
		}
		return nil
	default:
		return fmt.Errorf("invalid action: %s", ar.Action)
	}
}

// ActionResult represents the result of an action execution
type ActionResult struct {
	PRID      string        // PR ID
	Action    ActionCommand
	Success   bool
	Error     string    // error message if !Success
	Timestamp time.Time
	Retries   int // number of retries before success/failure
}

// permanentErrorKeywords are substrings that indicate a permanent failure
// which should not be retried.
var permanentErrorKeywords = []string{
	"401", "unauthorized", "403", "forbidden",
	"404", "not found",
	"400", "422", "bad request", "unprocessable",
	"validation", "invalid",
}

// transientErrorKeywords are substrings that indicate a transient failure
// which is worth retrying.
var transientErrorKeywords = []string{
	"429", "rate limit",
	"500", "502", "503", "service unavailable",
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
