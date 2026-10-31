package engine

import (
	"context"

	"github.com/itterno/bistro/internal/store/repolocation"
)

func (a *App) configureRepoLocationStore(repoLocationStore *repolocation.Store) {
	if a.gitClient == nil {
		return
	}
	a.gitClient.SetLocalResolver(newRepoLocationResolver(repoLocationStore, a.log))
}

func (a *App) cloneRepoLocationRepo(ctx context.Context, repoURL, destination string, bare bool) error {
	return a.gitClient.CloneRepoToPath(ctx, repoURL, destination, bare)
}
