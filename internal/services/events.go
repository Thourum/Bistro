package services

import "github.com/wailsapp/wails/v3/pkg/application"

// EventAuthRedirect asks the renderer to open the OAuth authorization URL.
const EventAuthRedirect = "auth:redirect"

// AuthRedirect is emitted only after the callback listener is bound.
type AuthRedirect struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
}

// Direct registrations allow Wails' binding generator to discover payload types.
func init() {
	application.RegisterEvent[AuthRedirect](EventAuthRedirect)
}
