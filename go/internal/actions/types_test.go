package actions

import (
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
)

func TestActionRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     *ActionRequest
		wantErr bool
	}{
		{
			name: "valid approve action",
			req: &ActionRequest{
				PRID:   "github:owner/repo:42",
				Action: "approve",
				Body:   "Looks good!",
			},
			wantErr: false,
		},
		{
			name: "missing PRID",
			req: &ActionRequest{
				Action: "approve",
				Body:   "Looks good!",
			},
			wantErr: true,
		},
		{
			name: "invalid action type",
			req: &ActionRequest{
				PRID:   "github:owner/repo:42",
				Action: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestActionResultSuccess(t *testing.T) {
	result := &ActionResult{
		PRID:      "github:owner/repo:42",
		Action:    "approve",
		Success:   true,
		Timestamp: time.Now(),
	}

	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %s", result.Error)
	}
}

func TestActionResultFailure(t *testing.T) {
	result := &ActionResult{
		PRID:      "github:owner/repo:42",
		Action:    "approve",
		Success:   false,
		Error:     "rate limit exceeded",
		Timestamp: time.Now(),
	}

	if result.Success {
		t.Error("expected Success=false")
	}
	if result.Error != "rate limit exceeded" {
		t.Errorf("expected error message, got %s", result.Error)
	}
}

func TestAmbiguousResultWithTransientCauseIsNotRecoverable(t *testing.T) {
	// Retrying would repeat a mutation the provider may already have applied.
	result := &ActionResult{Error: ErrAmbiguousRemoteSuccess + "provider action may have executed; local effects suppressed: network connection reset"}
	if result.IsRecoverable() {
		t.Error("ambiguous result must not be recoverable even when its cause is transient")
	}
}

func TestResolveThreadCommandValidation(t *testing.T) {
	cases := []struct {
		name    string
		req     ActionRequest
		wantErr bool
	}{
		{
			name:    "valid resolve",
			req:     ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: "PRRT_abc"},
			wantErr: false,
		},
		{
			name:    "missing thread id (body)",
			req:     ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: ""},
			wantErr: true,
		},
		{
			name:    "missing pr id",
			req:     ActionRequest{PRID: "", Action: ResolveThreadCommand, Body: "PRRT_abc"},
			wantErr: true,
		},
		{
			name:    "valid unresolve",
			req:     ActionRequest{PRID: "pr1", Action: UnresolveThreadCommand, Body: "PRRT_abc"},
			wantErr: false,
		},
		{
			name:    "unresolve missing body",
			req:     ActionRequest{PRID: "pr1", Action: UnresolveThreadCommand, Body: ""},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestSubmitReviewValidation(t *testing.T) {
	comments := []provider.ReviewComment{{Path: "a.go", Side: "RIGHT", Line: 1, Body: "x"}}
	cases := []struct {
		name    string
		req     ActionRequest
		wantErr bool
	}{
		{"approve without body", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventApprove}, false},
		{"comment with drafts only", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventComment, CommitID: "c", ReviewComments: comments}, false},
		{"request changes needs body", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventRequestChanges}, true},
		{"request changes with body", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventRequestChanges, Body: "Needs work"}, false},
		{"unknown event", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: "MERGE"}, true},
		{"missing event", ActionRequest{PRID: "p", Action: SubmitReviewCommand}, true},
		{"comments need commit id", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventApprove, ReviewComments: comments}, true},
		{"comment with body", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventComment, Body: "Notes"}, false},
		{"comment before drafts are loaded", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventComment}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestReplyCommentCommandValidation(t *testing.T) {
	cases := []struct {
		name    string
		req     ActionRequest
		wantErr bool
	}{
		{
			name:    "valid reply",
			req:     ActionRequest{PRID: "pr1", Action: ReplyCommentCommand, Body: "reply", InReplyTo: "github-review-comment-1"},
			wantErr: false,
		},
		{
			name:    "missing body",
			req:     ActionRequest{PRID: "pr1", Action: ReplyCommentCommand, InReplyTo: "github-review-comment-1"},
			wantErr: true,
		},
		{
			name:    "missing in reply to",
			req:     ActionRequest{PRID: "pr1", Action: ReplyCommentCommand, Body: "reply"},
			wantErr: true,
		},
		{
			name:    "valid thread target",
			req:     ActionRequest{PRID: "pr1", Action: ReplyCommentCommand, Body: "reply", ThreadID: "PRRT_abc"},
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// TestIsRateLimit_AnchoredPatterns verifies that the anchored rate-limit
// patterns correctly classify HTTP 429 responses and not PR-number matches.
func TestIsRateLimit_AnchoredPatterns(t *testing.T) {
	cases := []struct {
		errMsg      string
		wantIsRL    bool
		description string
	}{
		// Must be rate-limit.
		{"status 429: too many requests", true, "status 429"},
		{"status: 429 rate limit exceeded", true, "status: 429"},
		{"HTTP 429 Too Many Requests", true, "HTTP 429 (case insensitive)"},
		{"429 Too Many Requests from GitHub", true, "429 too many"},
		{"429 rate limit hit", true, "429 rate prefix"},
		{"GitHub API rate limit exceeded", true, "rate limit text"},
		// Must NOT be rate-limit.
		{"failed to post comment on PR owner/repo#429: connection reset", false, "PR number #429 in message"},
		{"error on PR github:o/r:429: timeout", false, "PR number :429 in message"},
		{"post comment PR-429 timed out", false, "PR-429 identifier"},
		{"connection reset by peer", false, "plain transport error"},
		{"503 service unavailable", false, "5xx not rate limit"},
	}

	for _, tc := range cases {
		ar := &ActionResult{Success: false, Error: tc.errMsg}
		got := ar.IsRateLimit()
		if got != tc.wantIsRL {
			t.Errorf("[%s] IsRateLimit(%q) = %v, want %v", tc.description, tc.errMsg, got, tc.wantIsRL)
		}
	}
}

// TestIsRateLimit_GoGithubFormat covers ": 429" and verifies ": 500" is NOT rate-limit.
func TestIsRateLimit_GoGithubFormat(t *testing.T) {
	cases := []struct {
		errMsg   string
		wantIsRL bool
		desc     string
	}{
		{"POST https://api.github.com/...: 429  []", true, "go-github ': 429'"},
		{"POST https://api.github.com/...: 500  []", false, "go-github ': 500' is not rate-limit"},
		{"POST https://api.github.com/...: 503  []", false, "go-github ': 503' is not rate-limit"},
		{"owner/repo#429: connection reset", false, "PR#429 not rate-limit"},
		{"owner/repo#500: connection reset", false, "PR#500 not rate-limit"},
	}
	for _, tc := range cases {
		ar := &ActionResult{Success: false, Error: tc.errMsg}
		if got := ar.IsRateLimit(); got != tc.wantIsRL {
			t.Errorf("[%s] IsRateLimit(%q) = %v, want %v", tc.desc, tc.errMsg, got, tc.wantIsRL)
		}
	}
}
