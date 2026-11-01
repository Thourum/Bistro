package services

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/itterno/bistro/internal/expertise"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/prdiff"
	"github.com/itterno/bistro/internal/store/repolocation"
)

type expertiseEngine interface {
	Enable(context.Context, string, string) error
	Disable(context.Context, string) error
	GitAvailable() bool
	GitUnavailableReason() string
}

type expertiseRepoStore interface {
	ExpertiseRepo(context.Context, string) (ExpertiseRepo, bool, error)
}

type expertiseDataStore interface {
	ExpertisePRRepo(context.Context, string) (string, bool, error)
	ExpertisePRFiles(context.Context, string) ([]string, error)
	FileExpertise(context.Context, string, string) ([]expertise.FileExpertiseRow, error)
	RepoState(context.Context, string) (expertise.ExpertiseRepoState, bool, error)
}

type expertiseSettingsStore interface {
	ExpertiseSettings(context.Context) (ExpertiseSettings, error)
	DismissExpertiseNudge(context.Context, string) error
	SetExpertiseSettings(context.Context, ExpertiseSettings) error
}

type repoLocationReader interface {
	Get(context.Context, string) (*repolocation.Location, error)
}

type prCheckoutInspector interface {
	InspectPRCheckout(string, string) (git.CheckoutInspection, error)
}

type prCheckoutChecker interface {
	CheckoutPRHead(context.Context, string, string, string) error
}

type prLister interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
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

type diffReader interface {
	GetUnified(prID string) (string, error)
	Get(prID string) ([]prdiff.File, error)
}

type diffViewedChecker interface {
	IsFileViewed(ctx context.Context, prID, filePath, login, contentHash string) (bool, error)
}

type prReader interface {
	GetPRCheckoutInfo(context.Context, string) (*store.PRCheckoutInfo, error)
}

type patchsetLister interface {
	prReader
	ListPatchsets(ctx context.Context, prID string) ([]store.Patchset, error)
}

type fileViewedMarker interface {
	MarkFileViewed(ctx context.Context, prID, filePath, login, contentHash string) error
}

type triageStore interface {
	prReader
	ListPatchsets(ctx context.Context, prID string) ([]store.Patchset, error)
	CurrentPatchset(ctx context.Context, prID string) (store.Patchset, bool, error)
	ListPatchsetFiles(ctx context.Context, prID string, number int) (map[string]string, error)
	GetLastReviewed(ctx context.Context, prID, login string) (store.LastReviewedRecord, bool, error)
}

type interdiffPatchsets interface {
	ListPatchsets(ctx context.Context, prID string) ([]store.Patchset, error)
	CurrentPatchset(ctx context.Context, prID string) (store.Patchset, bool, error)
}

type viewedBaselineStore interface {
	ListViewedFiles(ctx context.Context, prID, login string) ([]store.FileViewed, error)
	CurrentPatchset(ctx context.Context, prID string) (store.Patchset, bool, error)
	ListPatchsetFiles(ctx context.Context, prID string, number int) (map[string]string, error)
	RecordLastReviewed(ctx context.Context, event store.ReviewEvent) error
}

type readStateDB interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func contextError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func requirePR(ctx context.Context, reader prReader, prID string) error {
	pr, err := reader.GetPRCheckoutInfo(ctx, prID)
	if err != nil {
		return err
	}
	if pr == nil {
		return Errorf(CodeNotFound, "PR not found: %s", prID)
	}
	return nil
}
