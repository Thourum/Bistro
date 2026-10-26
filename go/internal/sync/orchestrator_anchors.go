package sync

import (
	"context"
	"log/slog"

	"github.com/itterno/bistro/internal/git/anchor"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

type anchorResolutionOptions struct {
	comments  []*provider.Comment
	threads   []*provider.ReviewThread
	prID      string
	localPath string
	headSHA   string
	prBaseSHA string
	log       *slog.Logger
}

type diffBaseSelection struct {
	patchsets         []store.Patchset
	patchsetNum       int
	isCurrentPosition bool
	prBaseSHA         string
	prHeadSHA         string
}

type anchorResolutionState struct {
	options               anchorResolutionOptions
	patchsets             []store.Patchset
	storedPatchsetNumbers map[string]int
	currentPatchsetNumber int
}

// resolveAnchors uses exact patchset identity for original-position comments;
// matching only the head SHA would be ambiguous after a base retarget.
func (o *Orchestrator) resolveAnchors(ctx context.Context, options anchorResolutionOptions) []anchor.Result {
	candidates := anchorCandidates(options.comments, options.threads)
	if len(candidates) == 0 {
		return nil
	}

	patchsets := o.anchorPatchsets(ctx, options)
	storedPatchsetNums := o.storedAnchorPatchsetNumbers(ctx, options, candidates)
	_, currentPatchsetNum, _ := selectDiffBaseSHA(diffBaseSelection{
		patchsets: patchsets, isCurrentPosition: true,
		prBaseSHA: options.prBaseSHA, prHeadSHA: options.headSHA,
	})

	state := anchorResolutionState{
		options: options, patchsets: patchsets,
		storedPatchsetNumbers: storedPatchsetNums, currentPatchsetNumber: currentPatchsetNum,
	}
	results := make([]anchor.Result, 0, len(candidates))
	for _, comment := range candidates {
		if resolved := o.resolveCommentAnchor(comment, state); resolved != nil {
			results = append(results, *resolved)
		}
	}
	return results
}

func anchorCandidates(comments []*provider.Comment, threads []*provider.ReviewThread) []*provider.Comment {
	var candidates []*provider.Comment
	add := func(comment *provider.Comment) {
		if comment != nil && comment.Kind == "review" && comment.FilePath != "" && isAnchorable(comment) {
			candidates = append(candidates, comment)
		}
	}
	for _, comment := range comments {
		add(comment)
	}
	for _, thread := range threads {
		if thread == nil {
			continue
		}
		for _, comment := range thread.Comments {
			add(comment)
		}
	}
	return candidates
}

func (o *Orchestrator) anchorPatchsets(ctx context.Context, options anchorResolutionOptions) []store.Patchset {
	if o.store == nil || options.prID == "" {
		return nil
	}
	patchsets, err := o.store.ListPatchsets(ctx, options.prID)
	if err != nil {
		options.log.Warn("resolve anchors: could not load patchsets; original-position comments will be skipped",
			"pr_id", options.prID, "error", err)
	}
	return patchsets
}

func (o *Orchestrator) storedAnchorPatchsetNumbers(ctx context.Context, options anchorResolutionOptions, candidates []*provider.Comment) map[string]int {
	if o.store == nil || options.prID == "" {
		return nil
	}
	var needLookup []string
	for _, comment := range candidates {
		if (comment.PatchsetNumber == nil || *comment.PatchsetNumber == 0) &&
			(comment.Position == nil || *comment.Position == 0) {
			needLookup = append(needLookup, comment.ID)
		}
	}
	if len(needLookup) == 0 {
		return nil
	}
	loaded, err := o.store.GetCommentPatchsetNumbers(ctx, needLookup)
	if err != nil {
		options.log.Warn("resolve anchors: could not load comment patchset numbers; affected comments will be skipped", "error", err)
		return nil
	}
	return loaded
}

func (o *Orchestrator) resolveCommentAnchor(comment *provider.Comment, state anchorResolutionState) *anchor.Result {
	isCurrentPosition := comment.Position != nil && *comment.Position > 0
	commitSHA := comment.CommitSHA
	position := 0
	if isCurrentPosition {
		position = *comment.Position
	} else {
		if comment.OriginalPosition != nil && *comment.OriginalPosition > 0 {
			position = *comment.OriginalPosition
		}
		if comment.OriginalSHA != "" {
			commitSHA = comment.OriginalSHA
		}
	}
	if commitSHA == "" || position == 0 {
		state.options.log.Debug("anchor skip: no usable commit SHA or position", "comment_id", comment.ID)
		return nil
	}

	patchsetNum := state.storedPatchsetNumbers[comment.ID]
	if comment.PatchsetNumber != nil {
		patchsetNum = *comment.PatchsetNumber
	}
	diffBaseSHA, resolvedPatchsetNum, skip := selectDiffBaseSHA(diffBaseSelection{
		patchsets: state.patchsets, patchsetNum: patchsetNum, isCurrentPosition: isCurrentPosition,
		prBaseSHA: state.options.prBaseSHA, prHeadSHA: state.options.headSHA,
	})
	if skip {
		state.options.log.Debug("anchor skip: original-position comment has no matching patchset", "comment_id", comment.ID, "patchset_num", patchsetNum)
		return nil
	}

	side := comment.Side
	if side == "" {
		side = "RIGHT"
	}
	resolved, err := o.anchorResolver.Resolve(anchor.Request{
		LocalPath: state.options.localPath, CommitSHA: commitSHA, HeadSHA: state.options.headSHA,
		DiffBaseSHA: diffBaseSHA, FilePath: comment.FilePath, Side: side,
		Position: position, CommentID: comment.ID,
		PatchsetNum: 0,  // A source patchset is not known at fetch time.
		CurrentNum:  -1, // Prevent the unknown 0==0 patchset fast path.
	})
	if err != nil {
		state.options.log.Debug("anchor resolve failed (best-effort, skipping)", "comment_id", comment.ID, "error", err)
		return nil
	}
	resolved.PatchsetNumber = resolvedPatchsetNum
	resolved.CurrentPatchsetNumber = state.currentPatchsetNumber
	return &resolved
}

// selectDiffBaseSHA returns the geometry base and patchset for the comment.
// Original-position comments require an exact patchset number, not a head match.
func selectDiffBaseSHA(options diffBaseSelection) (diffBase string, resolvedPatchsetNum int, skip bool) {
	if options.isCurrentPosition {
		current := 0
		for _, patchset := range options.patchsets {
			if patchset.HeadSHA == options.prHeadSHA {
				current = patchset.Number
			}
		}
		return options.prBaseSHA, current, false
	}
	if options.patchsetNum <= 0 {
		return "", 0, true
	}
	for _, patchset := range options.patchsets {
		if patchset.Number == options.patchsetNum {
			if patchset.MergeBaseSHA != "" {
				return patchset.MergeBaseSHA, patchset.Number, false
			}
			return patchset.BaseSHA, patchset.Number, false
		}
	}
	return "", 0, true
}

// isAnchorable reports whether a review comment has a current or original position.
func isAnchorable(comment *provider.Comment) bool {
	hasCurrent := comment.CommitSHA != "" && comment.Position != nil && *comment.Position > 0
	hasOriginal := comment.OriginalPosition != nil && *comment.OriginalPosition > 0
	return hasCurrent || hasOriginal
}
