package expertise

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const NudgeThreshold int64 = 10

type Account struct {
	Provider string
	Login    string
}

func ShouldNudge(ctx context.Context, store *Store, repoID string, account Account) (bool, error) {
	if store == nil {
		return false, errors.New("expertise store is nil")
	}

	isLocal, err := store.IsLocalRepo(ctx, repoID)
	if err != nil {
		return false, err
	}
	if !isLocal {
		return false, nil
	}

	state, hasState, err := nudgeRepoState(ctx, store, repoID)
	if err != nil {
		return false, err
	}
	if hasState && (state.Enabled || state.NudgeDismissedAt.Valid) {
		return false, nil
	}

	reviewedCount, err := store.ReviewedPRCount(ctx, repoID, account.Provider, account.Login, account.Login)
	if err != nil {
		return false, err
	}
	return reviewedCount >= NudgeThreshold, nil
}

func nudgeRepoState(ctx context.Context, store *Store, repoID string) (ExpertiseRepoState, bool, error) {
	state, err := store.RepoState(ctx, repoID)
	if errors.Is(err, sql.ErrNoRows) {
		return ExpertiseRepoState{}, false, nil
	}
	if err != nil {
		return ExpertiseRepoState{}, false, fmt.Errorf("get expertise nudge repo state %s: %w", repoID, err)
	}
	return state, true, nil
}
