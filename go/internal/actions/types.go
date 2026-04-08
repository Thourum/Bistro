package actions

import (
	"fmt"
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

// IsRecoverable determines if an error should be retried
func (ar *ActionResult) IsRecoverable() bool {
	// Rate limits, timeouts, temporary network errors are recoverable
	// Authorization, not found, invalid requests are not
	if !ar.Success && ar.Error != "" {
		// Simple heuristic: assume transient unless explicitly not
		// Phase 5+ can refine with error code mapping
		return true
	}
	return false
}
