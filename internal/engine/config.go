package engine

import (
	"io"
	"time"

	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/git"
)

// SetReadyWriter overrides where the machine-readable ready signal is written.
// Defaults to os.Stdout when unset.
func (a *App) SetReadyWriter(w io.Writer) {
	a.readyWriter = w
}

// SetGitClient configures the git client used by the sync orchestrator.
// Must be called before Run.
func (a *App) SetGitClient(gitClient *git.Client) {
	a.gitClient = gitClient
	if gitClient != nil && a.repoLocks != nil {
		gitClient.SetLocks(a.repoLocks)
	}
}

// SetRulesDir seeds the classification rules directory with the embedded
// built-ins (presence-gated) and constructs the engine provider shared by the
// sync writer and the comments handler. An empty dir skips seeding entirely;
// classification then runs on the embedded built-ins. Call before Run.
// The mode getter reads the live classification state, so a mode change from
// SET_CLASSIFICATION_SETTINGS invalidates fingerprints lazily.
func (a *App) SetRulesDir(dir string) {
	if dir != "" {
		if err := classify.SeedAndUpdate(dir); err != nil {
			a.log.Warn("rules dir unusable, classification falls back to embedded built-ins",
				"dir", dir, "err", err)
		}
	}
	a.classifier = classify.NewEngineProvider(dir, a.classificationState.Mode, a.log)
}

// SetSyncInterval configures the interval between automatic sync cycles.
func (a *App) SetSyncInterval(d time.Duration) {
	a.syncInterval = d
}

// SetAutoSync enables or disables the automatic periodic sync ticker.
// Manual FORCE_SYNC always works regardless of this setting.
func (a *App) SetAutoSync(enabled bool) {
	a.autoSync = enabled
}

// SetGitHubClientID sets the GitHub OAuth app client ID.
func (a *App) SetGitHubClientID(clientID string) {
	a.githubClientID = clientID
}

// SetGitHubClientSecret sets the GitHub OAuth app client secret used by the auth code flow.
func (a *App) SetGitHubClientSecret(secret string) {
	a.githubClientSecret = secret
}

// SetAuthCallbackTimeout overrides the OAuth callback deadline; zero uses the default.
func (a *App) SetAuthCallbackTimeout(duration time.Duration) {
	a.authCallbackTimeout = duration
}

func (a *App) githubToken() string {
	token, err := a.tokenManager.GetGitHubToken()
	if err != nil {
		return ""
	}
	return token
}
