package prsync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/prsync/writer"
	"github.com/itterno/bistro/internal/store"
)

// SupportsInboxDiscovery reports whether the provider can discover relevant PRs
// from provider-native inbox/search state without scanning every watched repo.
func (o *Orchestrator) SupportsInboxDiscovery() bool {
	_, ok := o.provider.(provider.InboxDiscoveryProvider)
	return ok
}

// DiscoverAndSyncInbox runs the local-first inbox discovery lane: GitHub search
// finds lightweight `involves:USERNAME` PR pointers, then provider repo PR
// endpoints hydrate canonical PR rows before SQLite is updated. The local-only
// high-water cursor advances only after detail hydration and DB writes succeed.
func (o *Orchestrator) DiscoverAndSyncInbox(ctx context.Context) (*SyncResult, error) {
	discovery, ok := o.provider.(provider.InboxDiscoveryProvider)
	if !ok {
		return &SyncResult{}, nil
	}
	if o.store == nil {
		return nil, errors.New("store not configured")
	}
	username, err := o.provider.GetUsername(ctx)
	if err != nil {
		return nil, fmt.Errorf("get username for inbox discovery: %w", err)
	}
	scope := provider.GitHub + ":inbox:" + username + ":search:involves"
	started := time.Now().UTC()
	options, err := o.inboxDiscoveryOptions(ctx, scope, username)
	if err != nil {
		return nil, err
	}
	discovered, err := discovery.DiscoverInboxPRs(ctx, options)
	if err != nil {
		return nil, o.recordInboxCursorError(ctx, scope, err)
	}
	if discovered == nil {
		discovered = &provider.InboxDiscoveryResult{}
	}
	result := &SyncResult{PRCount: len(discovered.PRs)}
	if len(discovered.PRs) > 0 {
		if err := o.writeDiscoveredPRs(ctx, scope, discovered.PRs, result); err != nil {
			return nil, err
		}
		if len(result.Errors) > 0 {
			o.recordInboxCursorError(ctx, scope,
				fmt.Errorf("inbox discovery had %d per-PR hydration errors", len(result.Errors)))
			return result, nil
		}
	}
	highWater := discovered.HighWater
	if highWater.IsZero() {
		highWater = started
	}
	if err := o.store.SaveSyncCursorSuccess(ctx, scope, store.FormatSyncCursorTime(highWater), discovered.ETag); err != nil {
		return nil, fmt.Errorf("save inbox cursor: %w", err)
	}
	return result, nil
}

func (o *Orchestrator) inboxDiscoveryOptions(ctx context.Context, scope, username string) (provider.InboxDiscoveryOptions, error) {
	options := provider.InboxDiscoveryOptions{Username: username, Limit: inboxDiscoveryLimit}
	cursor, err := o.store.GetSyncCursor(ctx, scope)
	if err != nil {
		return options, fmt.Errorf("load inbox cursor: %w", err)
	}
	if cursor == nil {
		return options, nil
	}
	options.ETag = cursor.ETag
	if cursor.CursorValue == "" {
		return options, nil
	}
	parsed, err := store.ParseSyncCursorTime(cursor.CursorValue)
	if err != nil {
		return options, fmt.Errorf("parse inbox cursor %q: %w", cursor.CursorValue, err)
	}
	lagged := parsed.Add(-inboxCursorLag)
	options.Since = &lagged
	return options, nil
}

func (o *Orchestrator) writeDiscoveredPRs(ctx context.Context, scope string, prs []*provider.PR, result *SyncResult) error {
	details, err := o.FetchPRDetails(ctx, prs, true, true, true)
	if err != nil {
		return o.recordInboxCursorError(ctx, scope, err)
	}
	for _, detail := range details {
		if detail == nil {
			continue
		}
		if detail.Error != nil {
			result.Errors = append(result.Errors, detail.Error.Error())
			continue
		}
		result.UpdatedPRs++
		result.CommentCount += len(detail.Comments)
	}
	writer := writer.New(o.log, o.store).WithEmitter(o.emitter).WithClassifier(o.classifier).WithCurrentUser(o.currentUser).WithPostCommit(o.postCommit)
	if err := writer.WriteSyncResultsContext(ctx, details); err != nil {
		return o.recordInboxCursorError(ctx, scope, err)
	}
	return nil
}

// Cursor failure recording is best-effort: preserve the original sync failure
// while surfacing a failed local error write to the application log.
func (o *Orchestrator) recordInboxCursorError(ctx context.Context, scope string, original error) error {
	if err := o.store.MarkSyncCursorError(ctx, scope, original); err != nil {
		logging.FromContext(ctx).Warn("record inbox cursor error failed", "error", err)
	}
	return original
}
