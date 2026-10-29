package services

import "github.com/wailsapp/wails/v3/pkg/application"

// EventAuthRedirect asks the renderer to open the OAuth authorization URL.
const EventAuthRedirect = "auth:redirect"

// AuthRedirect is emitted only after the callback listener is bound.
type AuthRedirect struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
}

const EventPRsReadChanged = "prs:read-changed"

// PRsReadChanged identifies PRs whose local read state changed.
type PRsReadChanged struct {
	PRIDs []string `json:"pr_ids"`
}

const EventDraftsChanged = "drafts:changed"

// DraftsChanged identifies the PR whose local drafts changed.
type DraftsChanged struct {
	PRID string `json:"pr_id"`
}

// Direct registrations allow Wails' binding generator to discover payload types.
func init() {
	application.RegisterEvent[DraftsChanged](EventDraftsChanged)
	application.RegisterEvent[AuthRedirect](EventAuthRedirect)
	application.RegisterEvent[PRsReadChanged](EventPRsReadChanged)
}
