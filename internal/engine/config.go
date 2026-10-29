package engine

import "github.com/itterno/bistro/internal/classify"

func (a *App) configureRules(dir string) {
	if dir != "" {
		if err := classify.SeedAndUpdate(dir); err != nil {
			a.log.Warn("rules dir unusable, classification falls back to embedded built-ins", "dir", dir, "err", err)
		}
	}
	a.classifier = classify.NewEngineProvider(dir, a.classificationState.Mode, a.log)
}

func (a *App) githubToken() string {
	token, err := a.tokenManager.GetGitHubToken()
	if err != nil {
		return ""
	}
	return token
}
