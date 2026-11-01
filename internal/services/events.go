package services

import (
	"github.com/itterno/bistro/internal/shell"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	EventAuthRedirect       = "auth:redirect"
	EventPRsReadChanged     = "prs:read-changed"
	EventDraftsChanged      = "drafts:changed"
	EventSyncComplete       = "sync:complete"
	EventSyncError          = "sync:error"
	EventSyncStatus         = "sync:status"
	EventRateLimited        = "sync:rate-limited"
	EventPRMetadataChanged  = "pr:metadata-changed"
	EventCIStatusChanged    = "pr:ci-status-changed"
	EventPRApproved         = "pr:approved"
	EventCommentPosted      = "comment:posted"
	EventThreadStateChanged = "thread:state-changed"
	EventActionComplete     = "action:complete"
	EventExpertiseProgress  = "expertise:progress"
	EventExpertiseReady     = "expertise:ready"
	EventExpertiseError     = "expertise:error"
	EventExpertiseNudge     = "expertise:nudge"
	EventCommentsClassified = "comments:classified"
	EventByokAuthFailed     = "byok:auth-failed"
	EventByokUsage          = "byok:usage"
	EventSettingsChanged    = "settings:changed"
	EventMenuAction         = "menu:action"
)

// MenuAction identifies a native menu click; Sequence distinguishes repeated clicks.
type MenuAction struct {
	ActionID string `json:"action_id"`
	Sequence int    `json:"sequence"`
}

var MenuActionIDs = []string{"global.settings", "global.inbox", "global.search", "global.command", "global.help"}

// AuthRedirect is emitted only after the callback listener is bound.
type AuthRedirect struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
}

// PRsReadChanged identifies PRs whose local read state changed.
type PRsReadChanged struct {
	PRIDs []string `json:"pr_ids"`
}

// DraftsChanged identifies the PR whose local drafts changed.
type DraftsChanged struct {
	PRID string `json:"pr_id"`
}

const (
	SyncStateSignedOut   = "signed_out"
	SyncStateSyncing     = "syncing"
	SyncStateUpToDate    = "up_to_date"
	SyncStatePaused      = "paused"
	SyncStateRateLimited = "rate_limited"
	SyncStateError       = "error"
)

type SyncStatus struct {
	State        string `json:"state"`
	LastSyncedAt string `json:"last_synced_at,omitempty"` // RFC 3339
	ResumeAt     string `json:"resume_at,omitempty"`      // RFC 3339, rate_limited only
	ErrorCode    string `json:"error_code,omitempty"`
}

type SyncComplete struct {
	Scope  string `json:"scope,omitempty"`
	RepoID string `json:"repo_id,omitempty"`
}

type SyncError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

type RateLimited struct {
	ResetIn float64 `json:"reset_in"`
}

type PRMetadataChanged struct {
	PRID   string `json:"pr_id"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Field  string `json:"field"`
	Old    string `json:"old"`
	New    string `json:"new"`
}

type CIStatusChanged struct {
	PRID      string `json:"pr_id"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Number    int    `json:"number"`
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}

type PRApproved struct {
	PRID string `json:"pr_id"`
}

type CommentPosted struct {
	PRID      string `json:"pr_id"`
	ThreadID  string `json:"thread_id"`
	InReplyTo string `json:"in_reply_to"`
}

type ThreadStateChanged struct {
	ThreadID string `json:"thread_id"`
	PRID     string `json:"pr_id"`
	Resolved bool   `json:"resolved"`
}

type ActionComplete struct {
	PR      string `json:"pr"`
	Action  string `json:"action"`
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

type ExpertiseProgress struct {
	RepoID     string `json:"repo_id"`
	FilesDone  int    `json:"files_done"`
	FilesTotal int    `json:"files_total"`
}

type ExpertiseReady struct {
	RepoID       string `json:"repo_id"`
	IndexedSHA   string `json:"indexed_sha"`
	FilesTotal   int    `json:"files_total"`
	SkippedFiles int    `json:"skipped_files"`
}

type ExpertiseError struct {
	RepoID  string `json:"repo_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ExpertiseNudge struct {
	RepoID        string `json:"repo_id"`
	ReviewedCount int64  `json:"reviewed_count"`
}

type CommentsClassified struct {
	PRID string `json:"pr_id"`
}

type ByokAuthFailed struct{}

type ByokUsage struct {
	SessionTokens int64   `json:"session_tokens"`
	EstCost       float64 `json:"est_cost"`
}

// Direct registrations allow Wails' binding generator to discover payload types.
func init() {
	application.RegisterEvent[MenuAction](EventMenuAction)
	application.RegisterEvent[AuthRedirect](EventAuthRedirect)
	application.RegisterEvent[PRsReadChanged](EventPRsReadChanged)
	application.RegisterEvent[DraftsChanged](EventDraftsChanged)
	application.RegisterEvent[SyncComplete](EventSyncComplete)
	application.RegisterEvent[SyncError](EventSyncError)
	application.RegisterEvent[SyncStatus](EventSyncStatus)
	application.RegisterEvent[RateLimited](EventRateLimited)
	application.RegisterEvent[PRMetadataChanged](EventPRMetadataChanged)
	application.RegisterEvent[CIStatusChanged](EventCIStatusChanged)
	application.RegisterEvent[PRApproved](EventPRApproved)
	application.RegisterEvent[CommentPosted](EventCommentPosted)
	application.RegisterEvent[ThreadStateChanged](EventThreadStateChanged)
	application.RegisterEvent[ActionComplete](EventActionComplete)
	application.RegisterEvent[ExpertiseProgress](EventExpertiseProgress)
	application.RegisterEvent[ExpertiseReady](EventExpertiseReady)
	application.RegisterEvent[ExpertiseError](EventExpertiseError)
	application.RegisterEvent[ExpertiseNudge](EventExpertiseNudge)
	application.RegisterEvent[CommentsClassified](EventCommentsClassified)
	application.RegisterEvent[ByokAuthFailed](EventByokAuthFailed)
	application.RegisterEvent[ByokUsage](EventByokUsage)
	application.RegisterEvent[shell.Settings](EventSettingsChanged)
}
