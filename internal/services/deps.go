package services

import (
	"context"
	"database/sql"
	"log/slog"
)

type prLister interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

type currentUserReader interface {
	Get(ctx context.Context, provider string) (string, bool, error)
}

type touchesYouResolver interface {
	TouchesYouForPRs(ctx context.Context, prIDs []string) (map[string]bool, error)
}

// PRSyncer is the on-demand hydration subset of the live orchestrator.
type PRSyncer interface {
	SyncPR(ctx context.Context, prID string) error
}

func serviceLogger(log *slog.Logger) *slog.Logger {
	if log != nil {
		return log
	}
	return slog.Default()
}
