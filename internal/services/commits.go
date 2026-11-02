package services

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/logging"
)

// Commits provides local, commit-by-commit PR review and account-scoped viewed marks.
type Commits struct {
	Patchsets   interdiffPatchsets
	Range       func(diff.CommitRangeRequest) (diff.CommitRange, error)
	Commit      func(diff.CommitRangeRequest, string) (diff.RangeCommit, error)
	MergeBase   func(repoPath, baseSHA, headSHA string) (string, error)
	RepoForPR   func(context.Context, string) (string, string, bool, error)
	Viewed      commitViewedStore
	CurrentUser currentUserReader
	Panics      PanicReporter
	Log         *slog.Logger
}

type CommitSummary struct {
	SHA        string    `json:"sha"`
	ParentSHAs []string  `json:"parent_shas"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	AuthorName string    `json:"author_name"`
	AuthoredAt time.Time `json:"authored_at"`
	IsMerge    bool      `json:"is_merge"`
	Additions  int       `json:"additions"`
	Deletions  int       `json:"deletions"`
	FileCount  int       `json:"file_count"`
	Viewed     bool      `json:"viewed"`
}

type CommitsResponse struct {
	PRID      string          `json:"pr_id"`
	Available bool            `json:"available"`
	Reason    string          `json:"reason"`
	Truncated bool            `json:"truncated"`
	Commits   []CommitSummary `json:"commits"`
}

type CommitRequest struct {
	PRID string `json:"pr_id"`
	SHA  string `json:"sha"`
}

type CommitDiffResponse struct {
	PRID      string     `json:"pr_id"`
	SHA       string     `json:"sha"`
	Available bool       `json:"available"`
	Reason    string     `json:"reason"`
	Diff      string     `json:"diff"`
	Files     []DiffFile `json:"files"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
}

type SetCommitViewedRequest struct {
	PRID   string `json:"pr_id"`
	SHA    string `json:"sha"`
	Viewed bool   `json:"viewed"`
}

type CommitViewed struct {
	PRID   string `json:"pr_id"`
	SHA    string `json:"sha"`
	Viewed bool   `json:"viewed"`
}

const ReasonCommitNotInPR = "commit_not_in_pr"

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type commitViewedStore interface {
	List(ctx context.Context, prID, login string) (map[string]int, error)
	Mark(ctx context.Context, prID, login, patchKey string, occurrences int) error
	Unmark(ctx context.Context, prID, login, patchKey string) error
}

// List returns the oldest bounded PR range with diff statistics and viewed state.
func (s *Commits) List(ctx context.Context, req PRRef) (CommitsResponse, error) {
	return guard(s.Panics, "Commits.List", func() (CommitsResponse, error) {
		ctx := logging.WithLogger(ctx, serviceLogger(s.Log))
		defer logCallTiming(ctx, "Commits.List", time.Now())
		prID := NormalizeID(req.PRID)
		if prID == "" {
			return CommitsResponse{}, Errorf(CodeInvalid, "pr_id is required")
		}
		resp := CommitsResponse{PRID: prID, Commits: []CommitSummary{}}
		rangeReq, reason, err := s.rangeRequest(ctx, prID)
		if err != nil || reason != "" {
			resp.Reason = reason
			return resp, err
		}
		rangeReq.WithDiffs = true
		commits, err := s.Range(rangeReq)
		if err != nil {
			resp.Reason, err = rangeFailure(ctx, err)
			return resp, err
		}
		login, err := s.login(ctx)
		if err != nil {
			return CommitsResponse{}, err
		}
		stored := map[string]int{}
		if login != "" {
			if stored, err = s.Viewed.List(ctx, prID, login); err != nil {
				return CommitsResponse{}, err
			}
		}
		keys := commitViewKeys(commits.Commits)
		resp.Available, resp.Truncated = true, commits.Truncated
		for i, commit := range commits.Commits {
			summary := CommitSummary{SHA: commit.SHA, ParentSHAs: commit.ParentSHAs, Subject: commit.Subject, Body: commit.Body, AuthorName: commit.AuthorName, AuthoredAt: commit.AuthoredAt, IsMerge: commit.IsMerge}
			if commit.Diff != nil {
				summary.Additions, summary.Deletions, summary.FileCount = commit.Diff.Additions, commit.Diff.Deletions, len(commit.Diff.Files)
			}
			occurrences, found := stored[keys[i].Key]
			summary.Viewed = found && occurrences == keys[i].Occurrences
			resp.Commits = append(resp.Commits, summary)
		}
		return resp, nil
	})
}

// Get returns a read-only diff only for a commit in the current PR range.
func (s *Commits) Get(ctx context.Context, req CommitRequest) (CommitDiffResponse, error) {
	return guard(s.Panics, "Commits.Get", func() (CommitDiffResponse, error) {
		ctx := logging.WithLogger(ctx, serviceLogger(s.Log))
		defer logCallTiming(ctx, "Commits.Get", time.Now())
		prID, sha, err := validateCommitRequest(req)
		if err != nil {
			return CommitDiffResponse{}, err
		}
		resp := CommitDiffResponse{PRID: prID, SHA: sha, Files: []DiffFile{}}
		rangeReq, reason, err := s.rangeRequest(ctx, prID)
		if err != nil || reason != "" {
			resp.Reason = reason
			return resp, err
		}
		commit, err := s.Commit(rangeReq, sha)
		if errors.Is(err, diff.ErrCommitNotInRange) {
			return resp, WithReason(CodeInvalid, ReasonCommitNotInPR, "this commit is no longer part of the PR")
		}
		if err != nil {
			resp.Reason, err = rangeFailure(ctx, err)
			return resp, err
		}
		resp.Available = true
		if commit.Diff != nil {
			resp.Diff, resp.Additions, resp.Deletions = commit.Diff.Unified, commit.Diff.Additions, commit.Diff.Deletions
			for _, file := range commit.Diff.Files {
				resp.Files = append(resp.Files, DiffFile{
					Path: file.Path, Status: file.Status, OldPath: file.OldPath,
					IsBinary: file.Binary, IsTruncated: file.Truncated, DiffStatus: string(file.DiffStatus),
					PatchBytes: file.PatchBytes, HunkCount: file.HunkCount,
					Additions: file.AddedLines, Deletions: file.RemovedLines,
				})
			}
		}
		return resp, nil
	})
}

// SetViewed derives the key from the current range; renderer-supplied keys are never trusted.
func (s *Commits) SetViewed(ctx context.Context, req SetCommitViewedRequest) (CommitViewed, error) {
	return guard(s.Panics, "Commits.SetViewed", func() (CommitViewed, error) {
		ctx := logging.WithLogger(ctx, serviceLogger(s.Log))
		defer logCallTiming(ctx, "Commits.SetViewed", time.Now())
		prID, sha, err := validateCommitRequest(CommitRequest{PRID: req.PRID, SHA: req.SHA})
		if err != nil {
			return CommitViewed{}, err
		}
		login, err := s.login(ctx)
		if err != nil {
			return CommitViewed{}, err
		}
		if login == "" {
			return CommitViewed{}, Errorf(CodeUnauthenticated, "sign in to mark commits viewed")
		}
		rangeReq, reason, err := s.rangeRequest(ctx, prID)
		if err != nil {
			return CommitViewed{}, err
		}
		if reason != "" {
			return CommitViewed{}, Errorf(CodeInvalid, "%s", reason)
		}
		rangeReq.WithDiffs = true
		commits, err := s.Range(rangeReq)
		if err != nil {
			if cerr := contextError(err); cerr != nil {
				return CommitViewed{}, cerr
			}
			if errors.Is(err, diff.ErrHeadUnavailable) {
				reason, _ := rangeFailure(ctx, err)
				return CommitViewed{}, Errorf(CodeInvalid, "%s", reason)
			}
			logging.FromContext(ctx).Warn("commits: range failed", "error", err)
			return CommitViewed{}, Errorf(CodeInternal, "could not list commits")
		}
		for i, commit := range commits.Commits {
			if commit.SHA != sha {
				continue
			}
			key := commitViewKeys(commits.Commits)[i]
			if req.Viewed {
				err = s.Viewed.Mark(ctx, prID, login, key.Key, key.Occurrences)
			} else {
				err = s.Viewed.Unmark(ctx, prID, login, key.Key)
			}
			if err != nil {
				return CommitViewed{}, err
			}
			return CommitViewed{PRID: prID, SHA: sha, Viewed: req.Viewed}, nil
		}
		return CommitViewed{}, WithReason(CodeInvalid, ReasonCommitNotInPR, "this commit is no longer part of the PR")
	})
}

func validateCommitRequest(req CommitRequest) (string, string, error) {
	prID, sha := NormalizeID(req.PRID), strings.ToLower(req.SHA)
	if prID == "" {
		return "", "", Errorf(CodeInvalid, "pr_id is required")
	}
	if !shaPattern.MatchString(sha) {
		return "", "", Errorf(CodeInvalid, "sha must be a 40-character commit id")
	}
	return prID, sha, nil
}

// rangeRequest returns a reason for unavailable local data, not a failed read.
func (s *Commits) rangeRequest(ctx context.Context, prID string) (diff.CommitRangeRequest, string, error) {
	logger := logging.FromContext(ctx)
	repoURL, repoPath, ok, err := s.RepoForPR(ctx, prID)
	if errors.Is(err, sql.ErrNoRows) {
		return diff.CommitRangeRequest{}, "", Errorf(CodeNotFound, "PR not found")
	}
	if cerr := contextError(err); cerr != nil {
		return diff.CommitRangeRequest{}, "", cerr
	}
	if err != nil {
		logger.Warn("commits: repo lookup failed", "error", err)
		return diff.CommitRangeRequest{}, "could not locate the repository", nil
	}
	if !ok {
		return diff.CommitRangeRequest{}, "no local copy of this repository yet", nil
	}
	current, found, err := s.Patchsets.CurrentPatchset(ctx, prID)
	if cerr := contextError(err); cerr != nil {
		return diff.CommitRangeRequest{}, "", cerr
	}
	if err != nil {
		logger.Warn("commits: current patchset lookup failed", "error", err)
		return diff.CommitRangeRequest{}, "could not load the PR head", nil
	}
	if !found {
		return diff.CommitRangeRequest{}, "this PR has not been synced yet", nil
	}
	mergeBase := current.MergeBaseSHA
	if mergeBase == "" {
		mergeBase, err = s.MergeBase(repoPath, current.BaseSHA, current.HeadSHA)
		if cerr := contextError(err); cerr != nil {
			return diff.CommitRangeRequest{}, "", cerr
		}
		if err != nil {
			logger.Warn("commits: merge base failed", "error", err)
			return diff.CommitRangeRequest{}, "could not find the merge base", nil
		}
	}
	return diff.CommitRangeRequest{RepoURL: repoURL, RepoPath: repoPath, HeadSHA: current.HeadSHA, MergeBaseSHA: mergeBase}, "", nil
}

func rangeFailure(ctx context.Context, err error) (string, error) {
	if cerr := contextError(err); cerr != nil {
		return "", cerr
	}
	if errors.Is(err, diff.ErrHeadUnavailable) {
		return "the PR head or merge base is not in the local repository", nil
	}
	logging.FromContext(ctx).Warn("commits: range failed", "error", err)
	return "could not list commits", nil
}

func (s *Commits) login(ctx context.Context) (string, error) {
	if s.CurrentUser == nil {
		return "", nil
	}
	login, ok, err := s.CurrentUser.Get(ctx, githubProvider)
	if err != nil || !ok {
		return "", err
	}
	return login, nil
}

func logCallTiming(ctx context.Context, method string, started time.Time) {
	logging.FromContext(ctx).Debug("service call timing", "method", method, "duration_ms", time.Since(started).Milliseconds())
}
