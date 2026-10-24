package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

const (
	prSavepoint = "sp_pr"
	// Surface batches that take noticeably longer than a normal local write.
	slowSyncWrite = 500 * time.Millisecond
)

// Writer persists sync results in the local database.
type Writer struct {
	log         *slog.Logger
	db          *store.DB
	emitter     *events.EventEmitter
	classifier  *classify.EngineProvider
	currentUser currentUserReader
	// postCommit runs after a successful tx.Commit() with the ids of the PRs
	// written in the batch; asynchronous classification must not run inside the tx.
	postCommit func(prIDs []string)
}

type currentUserReader interface {
	GetCurrentUser(ctx context.Context, provider string) (string, bool, error)
}

// NewWriter creates a database writer, using the default logger if log is nil.
func NewWriter(log *slog.Logger, db *store.DB) *Writer {
	if log == nil {
		log = slog.Default()
	}
	return &Writer{log: log, db: db}
}

// WithEmitter wires an EventEmitter into the writer so PR metadata and CI
// status changes are broadcast after the sync transaction commits.
func (w *Writer) WithEmitter(e *events.EventEmitter) *Writer {
	w.emitter = e
	return w
}

// WithClassifier wires the classification engine provider into the writer.
// Each batch uses one engine so all comments see a consistent classification.
func (w *Writer) WithClassifier(p *classify.EngineProvider) *Writer {
	w.classifier = p
	return w
}

// WithCurrentUser attaches a current-user reader used to detect @mentions while
// writing PRs. Nil readers leave mentioned at its database default.
func (w *Writer) WithCurrentUser(reader currentUserReader) *Writer {
	w.currentUser = reader
	return w
}

// WithPostCommit registers a hook invoked after a successful commit with the
// written PR ids. It runs outside the transaction.
func (w *Writer) WithPostCommit(fn func(prIDs []string)) *Writer {
	w.postCommit = fn
	return w
}

type prSnapshot struct {
	state    string
	ciStatus string
	title    string
	draft    bool
}

func readPRSnapshot(ctx context.Context, tx *sql.Tx, prID string) (*prSnapshot, error) {
	var snapshot prSnapshot
	var draftInt int
	err := tx.QueryRowContext(ctx,
		`SELECT state, COALESCE(ci_status,''), title, draft FROM prs WHERE id = ? LIMIT 1`,
		prID,
	).Scan(&snapshot.state, &snapshot.ciStatus, &snapshot.title, &draftInt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read PR snapshot %s: %w", prID, err)
	}
	snapshot.draft = draftInt != 0
	return &snapshot, nil
}

// WriteSyncResults persists all sync results in one transaction. A failed PR
// write rolls back its savepoint without discarding other PRs in the batch.
func (w *Writer) WriteSyncResults(results []*PRFetchResult) error {
	return w.WriteSyncResultsContext(context.Background(), results)
}

// WriteSyncResultsContext writes a batch in one transaction, honoring ctx.
// Each PR has a savepoint for its row, patchset, diff, comments, and threads;
// failed critical writes roll back that PR while other PRs can still commit.
// A failed rollback or release aborts the batch rather than commit partial data.
// Per-comment and per-thread failures are best-effort and logged separately.
func (w *Writer) WriteSyncResultsContext(ctx context.Context, results []*PRFetchResult) (writeErr error) {
	if len(results) == 0 || w.db == nil {
		return nil
	}
	log := logging.FromContext(ctx)
	if log == slog.Default() && w.log != nil {
		log = w.log
	}
	start := time.Now()
	batch := w.newWriterBatch(ctx, log)
	var err error
	batch.tx, err = w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync write transaction: %w", err)
	}
	defer func() {
		// After a successful Commit, Rollback returns sql.ErrTxDone; otherwise
		// preserve a rollback failure alongside the original write error.
		if rollbackErr := batch.tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			writeErr = errors.Join(writeErr, fmt.Errorf("roll back sync write transaction: %w", rollbackErr))
		}
	}()

	for _, result := range results {
		if err := w.writeOnePR(ctx, &batch, result); err != nil {
			return err
		}
	}
	if err := batch.tx.Commit(); err != nil {
		return fmt.Errorf("commit sync write transaction: %w", err)
	}
	w.afterBatchCommit(&batch)
	batch.logSummary(time.Since(start))
	return nil
}

func (w *Writer) newWriterBatch(ctx context.Context, log *slog.Logger) writerBatch {
	batch := writerBatch{log: log}
	if w.classifier != nil {
		batch.engine = w.classifier.Current()
	}
	var err error
	batch.mentionPattern, err = w.mentionPattern(ctx)
	if err != nil {
		// Mention detection is advisory; a lookup error cannot discard a batch.
		log.Warn("mention detection unavailable, continuing without it", "error", err)
	}
	return batch
}

func (w *Writer) afterBatchCommit(batch *writerBatch) {
	for _, event := range batch.pendingEvents {
		w.emitter.Emit(event)
	}
	if w.postCommit != nil && len(batch.writtenPRIDs) > 0 {
		w.postCommit(batch.writtenPRIDs)
	}
}

type writerBatch struct {
	tx             *sql.Tx
	log            *slog.Logger
	engine         *classify.Engine
	mentionPattern *regexp.Regexp
	prCount        int
	prSkipped      int
	prErrors       int
	commentCount   int
	diffCount      int
	pendingEvents  []*events.Event
	writtenPRIDs   []string
}

type prWriteCounts struct {
	comments       int
	diffs          int
	patchsetNumber int
}

func (w *Writer) writeOnePR(ctx context.Context, batch *writerBatch, result *PRFetchResult) error {
	if result == nil {
		batch.prSkipped++
		return nil
	}
	if result.Error != nil {
		batch.prErrors++
		prID := ""
		if result.PR != nil {
			prID = result.PR.ID
		}
		batch.log.Warn("skipping pr write due to upstream error", "pr_id", prID, "error", result.Error)
		return nil
	}
	if result.PR == nil {
		batch.prSkipped++
		return nil
	}

	prID := result.PR.ID
	if _, err := batch.tx.ExecContext(ctx, "SAVEPOINT "+prSavepoint); err != nil {
		batch.prErrors++
		batch.log.Error("skipping PR after savepoint creation failed", "pr_id", prID, "error", err)
		return nil
	}
	snapshot, err := readPRSnapshot(ctx, batch.tx, prID)
	var counts prWriteCounts
	if err == nil {
		counts, err = w.writePRData(ctx, batch, result)
	}
	if err != nil {
		return batch.skipFailedPR(ctx, prID, err)
	}
	if _, err := batch.tx.ExecContext(ctx, "RELEASE SAVEPOINT "+prSavepoint); err != nil {
		// RELEASE failure leaves savepoint state unknown: rolling back the
		// outer transaction is safer than committing a partial PR batch.
		return fmt.Errorf("release savepoint for PR %s: %w", prID, err)
	}

	w.writeAfterSavepoint(ctx, batch, result, counts.patchsetNumber)
	batch.recordWrittenPR(result.PR, snapshot, counts, w.emitter)
	return nil
}

func (batch *writerBatch) recordWrittenPR(pr *provider.PR, snapshot *prSnapshot, counts prWriteCounts, emitter *events.EventEmitter) {
	if snapshot != nil && emitter != nil {
		batch.pendingEvents = append(batch.pendingEvents, collectChangeEvents(snapshot, pr)...)
	}
	batch.prCount++
	batch.commentCount += counts.comments
	batch.diffCount += counts.diffs
	batch.writtenPRIDs = append(batch.writtenPRIDs, pr.ID)
}

func (batch *writerBatch) skipFailedPR(ctx context.Context, prID string, writeErr error) error {
	if rollbackErr := rollbackPRSavepoint(ctx, batch.tx); rollbackErr != nil {
		return errors.Join(fmt.Errorf("write PR %s: %w", prID, writeErr), rollbackErr)
	}
	batch.prErrors++
	batch.log.Error("skipping PR after savepoint rollback", "pr_id", prID, "error", writeErr)
	return nil
}

func rollbackPRSavepoint(ctx context.Context, tx *sql.Tx) error {
	// A failed rollback or release makes the savepoint's state unknowable;
	// the outer transaction must then abort rather than commit partial writes.
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+prSavepoint); err != nil {
		return fmt.Errorf("roll back PR savepoint: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+prSavepoint); err != nil {
		return fmt.Errorf("release rolled-back PR savepoint: %w", err)
	}
	return nil
}

func (w *Writer) writePRData(ctx context.Context, batch *writerBatch, result *PRFetchResult) (prWriteCounts, error) {
	counts, err := w.writeCriticalPRData(ctx, batch.tx, result)
	if err != nil {
		return counts, err
	}
	counts.comments = w.writeComments(ctx, batch, commentBatch{
		comments: result.Comments, prID: result.PR.ID, patchsetNumber: counts.patchsetNumber,
	})
	if err := w.updateMentioned(ctx, batch, result); err != nil {
		return counts, err
	}
	if len(result.Threads) > 0 {
		w.writeReviewThreads(ctx, batch.tx, reviewThreadWrite{
			threads: result.Threads, patchsetNumber: counts.patchsetNumber, engine: batch.engine,
		})
	}
	return counts, nil
}

func (w *Writer) writeCriticalPRData(ctx context.Context, tx *sql.Tx, result *PRFetchResult) (prWriteCounts, error) {
	var counts prWriteCounts
	pr := result.PR
	mode := hydratedWrite
	if result.ListOnly {
		mode = listOnlyWrite
	}
	if err := w.writePRRow(ctx, tx, pr, mode); err != nil {
		return counts, err
	}
	var files []store.DiffFile
	if result.DiffComputed {
		files = result.DiffFiles
		if files == nil {
			files = []store.DiffFile{} // Computed-but-empty still records "no files".
		}
	}
	number, err := w.recordPatchset(ctx, tx, patchsetWrite{pr: pr, mergeBase: result.MergeBaseSHA, files: files})
	if err != nil {
		return counts, err
	}
	counts.patchsetNumber = number
	if result.DiffComputed {
		if err := w.writeDiffs(ctx, tx, pr.ID, result.DiffFiles); err != nil {
			return counts, err
		}
		counts.diffs = len(result.DiffFiles)
	}
	return counts, nil
}

type commentBatch struct {
	comments       []*provider.Comment
	prID           string
	patchsetNumber int
}

func (w *Writer) writeComments(ctx context.Context, batch *writerBatch, comments commentBatch) int {
	written := 0
	for _, comment := range comments.comments {
		if comment == nil {
			continue
		}
		start := time.Now()
		err := w.writeComment(ctx, batch.tx, commentWrite{
			prID: comments.prID, comment: comment,
			patchsetNumber: commentPatchsetNumber(comment, comments.patchsetNumber), engine: batch.engine,
		})
		if err != nil {
			batch.log.Error("failed to write comment", "comment_id", comment.ID, "pr_id", comments.prID,
				"duration_ms", time.Since(start).Milliseconds(), "error", err)
			continue
		}
		written++
	}
	return written
}

func commentPatchsetNumber(comment *provider.Comment, currentNumber int) int {
	// Only current-position review comments belong to the current patchset;
	// outdated comments must retain their original patchset on upsert.
	if comment.Kind == "review" && comment.Position != nil && *comment.Position > 0 {
		return currentNumber
	}
	return 0
}

func (w *Writer) writeAfterSavepoint(ctx context.Context, batch *writerBatch, result *PRFetchResult, patchsetNumber int) {
	w.recordGitHubReviewBaseline(ctx, batch, result)
	if len(result.AnchorUpdates) > 0 {
		w.writeAnchorUpdates(ctx, batch.tx, result.AnchorUpdates, patchsetNumber)
	}
	// Dedup needs this cycle's anchor lines, so it runs after anchor write-back.
	if batch.engine != nil {
		if err := w.dedupPRComments(ctx, batch.tx, result.PR.ID); err != nil {
			batch.log.Warn("dedup post-pass failed (comments remain ungrouped)", "pr_id", result.PR.ID, "error", err)
		}
	}
}

func collectChangeEvents(previous *prSnapshot, pr *provider.PR) []*events.Event {
	var changes []*events.Event
	if previous.state != pr.State {
		changes = append(changes, events.NewPRMetadataChangedEvent(pr.ID, pr.Owner, pr.Repo, pr.Number,
			"state", previous.state, pr.State))
	}
	if previous.title != pr.Title {
		changes = append(changes, events.NewPRMetadataChangedEvent(pr.ID, pr.Owner, pr.Repo, pr.Number,
			"title", previous.title, pr.Title))
	}
	if previous.draft != pr.Draft {
		changes = append(changes, events.NewPRMetadataChangedEvent(pr.ID, pr.Owner, pr.Repo, pr.Number,
			"draft", fmt.Sprint(previous.draft), fmt.Sprint(pr.Draft)))
	}
	if previous.ciStatus != pr.CIStatus {
		changes = append(changes, events.NewCIStatusChangedEvent(pr.ID, pr.Owner, pr.Repo, pr.Number,
			previous.ciStatus, pr.CIStatus))
	}
	return changes
}

func (batch *writerBatch) logSummary(elapsed time.Duration) {
	logFn := batch.log.Info
	if elapsed > slowSyncWrite {
		logFn = batch.log.Warn
	}
	logFn("sync results written", "prs", batch.prCount, "prs_skipped", batch.prSkipped,
		"prs_errors", batch.prErrors, "comments", batch.commentCount, "diffs", batch.diffCount,
		"duration_ms", elapsed.Milliseconds())
}

func (w *Writer) mentionPattern(ctx context.Context) (*regexp.Regexp, error) {
	if w.currentUser == nil {
		return nil, nil
	}
	login, ok, err := w.currentUser.GetCurrentUser(ctx, githubProvider)
	if err != nil {
		return nil, fmt.Errorf("get current user for mention detection: %w", err)
	}
	if !ok || login == "" {
		return nil, nil
	}
	return regexp.Compile(`(?i)@` + regexp.QuoteMeta(login) + `\b`)
}

func hasMention(pattern *regexp.Regexp, comments []*provider.Comment) bool {
	for _, comment := range comments {
		if comment != nil && pattern.MatchString(comment.Body) {
			return true
		}
	}
	return false
}

func hasThreadMention(pattern *regexp.Regexp, threads []*provider.ReviewThread) bool {
	for _, thread := range threads {
		if thread != nil && hasMention(pattern, thread.Comments) {
			return true
		}
	}
	return false
}

type mentionUpdate struct {
	prID        string
	isMentioned bool
	canClear    bool
}

func (w *Writer) updateMentioned(ctx context.Context, batch *writerBatch, result *PRFetchResult) error {
	pattern := batch.mentionPattern
	if pattern == nil {
		return nil
	}
	mentioned := pattern.MatchString(result.PR.Description)
	if !mentioned {
		mentioned = hasMention(pattern, result.Comments)
	}
	if !mentioned {
		mentioned = hasThreadMention(pattern, result.Threads)
	}
	return w.writeMentioned(ctx, batch.tx, mentionUpdate{
		prID: result.PR.ID, isMentioned: mentioned,
		canClear: !result.ListOnly && result.CommentsFetched && result.ThreadsFetched,
	})
}

func (w *Writer) writeMentioned(ctx context.Context, tx *sql.Tx, update mentionUpdate) error {
	mentionedValue := 0
	if update.isMentioned {
		mentionedValue = 1
	}
	query := `UPDATE prs SET mentioned = MAX(mentioned, ?) WHERE id = ?`
	if update.canClear {
		query = `UPDATE prs SET mentioned = ? WHERE id = ?`
	}
	if _, err := tx.ExecContext(ctx, query, mentionedValue, update.prID); err != nil {
		return fmt.Errorf("update mentioned for PR %s: %w", update.prID, err)
	}
	return nil
}
