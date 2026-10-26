package sync

import (
	"context"
	"errors"
	"testing"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/storetest"
)

type mentionedTestCurrentUserReader struct {
	login string
	ok    bool
	err   error
}

func (r mentionedTestCurrentUserReader) GetCurrentUser(context.Context, string) (string, bool, error) {
	return r.login, r.ok, r.err
}

func mentionedTestPR(id, body string) *provider.PR {
	return &provider.PR{
		ID:          id,
		Provider:    "github",
		Owner:       "org",
		Repo:        "repo",
		Number:      1,
		Title:       "Mention test",
		Description: body,
		Author:      "bob",
		State:       "open",
		BaseBranch:  "main",
		HeadBranch:  "mention-test",
		BaseSHA:     "base",
		HeadSHA:     "head",
		CreatedAt:   mustTime(),
		UpdatedAt:   mustTime(),
	}
}

func mentionedValue(t *testing.T, db *store.DB, prID string) int {
	t.Helper()

	var mentioned int
	if err := db.QueryRow(`SELECT mentioned FROM prs WHERE id = ?`, prID).Scan(&mentioned); err != nil {
		t.Fatalf("read mentioned for %s: %v", prID, err)
	}
	return mentioned
}

func TestWriterSetsMentionedFromPRBody(t *testing.T) {
	db := storetest.Open(t)
	pr := mentionedTestPR("github:org/repo:mention-body", "hey @alice please look")
	writer := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "alice", ok: true})

	if err := writer.WriteSyncResults([]*PRFetchResult{{PR: pr}}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	if got := mentionedValue(t, db, pr.ID); got != 1 {
		t.Errorf("mentioned = %d, want 1", got)
	}
}

func TestWriterSetsMentionedFromComment(t *testing.T) {
	db := storetest.Open(t)
	pr := mentionedTestPR("github:org/repo:mention-comment", "no mention here")
	writer := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "alice", ok: true})
	comment := &provider.Comment{
		ID:        "mention-comment",
		PRID:      pr.ID,
		Author:    "bob",
		Body:      "Could @alice review this?",
		CreatedAt: mustTime(),
		UpdatedAt: mustTime(),
	}

	if err := writer.WriteSyncResults([]*PRFetchResult{{PR: pr, Comments: []*provider.Comment{comment}}}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	if got := mentionedValue(t, db, pr.ID); got != 1 {
		t.Errorf("mentioned = %d, want 1", got)
	}
}

func TestWriterMentionedMatchesCaseInsensitiveExactLogin(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "case insensitive", body: "hey @Alice", want: 1},
		{name: "does not match login prefix", body: "hey @alice2", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := storetest.Open(t)
			pr := mentionedTestPR("github:org/repo:mention-"+tt.name, tt.body)
			writer := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "alice", ok: true})

			if err := writer.WriteSyncResults([]*PRFetchResult{{PR: pr}}); err != nil {
				t.Fatalf("WriteSyncResults: %v", err)
			}

			if got := mentionedValue(t, db, pr.ID); got != tt.want {
				t.Errorf("mentioned = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWriterLeavesMentionedUnsetWithoutCurrentUser(t *testing.T) {
	db := storetest.Open(t)
	pr := mentionedTestPR("github:org/repo:mention-no-user", "hey @alice please look")
	writer := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{})

	if err := writer.WriteSyncResults([]*PRFetchResult{{PR: pr}}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	if got := mentionedValue(t, db, pr.ID); got != 0 {
		t.Errorf("mentioned = %d, want 0", got)
	}
}

func TestWriterPersistsPRWhenCurrentUserLookupFails(t *testing.T) {
	db := storetest.Open(t)
	pr := mentionedTestPR("github:org/repo:mention-lookup-error", "hey @alice please look")
	writer := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{err: errors.New("keyring unavailable")})

	if err := writer.WriteSyncResults([]*PRFetchResult{{PR: pr}}); err != nil {
		t.Fatalf("WriteSyncResults() error = %v, want nil when current-user lookup fails", err)
	}

	var storedTitle string
	if err := db.QueryRow(`SELECT title FROM prs WHERE id = ?`, pr.ID).Scan(&storedTitle); err != nil {
		t.Fatalf("read persisted PR: %v", err)
	}
	if got := mentionedValue(t, db, pr.ID); got != 0 {
		t.Errorf("mentioned = %d, want 0 when mention detection is unavailable", got)
	}
}

func TestWriterPreservesMentionedWhenCurrentUserLookupFails(t *testing.T) {
	db := storetest.Open(t)
	pr := mentionedTestPR("github:org/repo:mention-preserve-on-error", "hey @alice please look")

	okWriter := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "alice", ok: true})
	mustWritePR(t, okWriter, pr)
	if got := mentionedValue(t, db, pr.ID); got != 1 {
		t.Fatalf("precondition mentioned = %d, want 1", got)
	}

	failingWriter := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{err: errors.New("keyring unavailable")})
	if err := failingWriter.WriteSyncResults([]*PRFetchResult{{PR: pr}}); err != nil {
		t.Fatalf("WriteSyncResults() error = %v, want nil when current-user lookup fails", err)
	}
	if got := mentionedValue(t, db, pr.ID); got != 1 {
		t.Errorf("mentioned = %d, want existing value 1 preserved", got)
	}
}

func TestWriterSetsMentionedFromThreadComment(t *testing.T) {
	db := storetest.Open(t)
	pr := mentionedTestPR("github:org/repo:mention-thread", "no mention here")
	writer := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "alice", ok: true})
	thread := &provider.ReviewThread{
		ID:   "mention-thread",
		PRID: pr.ID,
		Comments: []*provider.Comment{{
			ID:        "mention-thread-comment",
			PRID:      pr.ID,
			Author:    "bob",
			Body:      "Please review this line, @alice.",
			Kind:      "review",
			CreatedAt: mustTime(),
			UpdatedAt: mustTime(),
		}},
	}

	if err := writer.WriteSyncResults([]*PRFetchResult{{
		PR: pr, Threads: []*provider.ReviewThread{thread}, CommentsFetched: true, ThreadsFetched: true,
	}}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	if got := mentionedValue(t, db, pr.ID); got != 1 {
		t.Errorf("mentioned = %d, want 1", got)
	}
}

func TestWriterClearsMentionedAfterAllCommentSourcesRefresh(t *testing.T) {
	db := storetest.Open(t)
	pr := mentionedTestPR("github:org/repo:mention-clear", "no mention here")
	writer := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "alice", ok: true})
	comment := &provider.Comment{
		ID:        "mention-clear-comment",
		PRID:      pr.ID,
		Author:    "bob",
		Body:      "Please look, @alice.",
		CreatedAt: mustTime(),
		UpdatedAt: mustTime(),
	}

	if err := writer.WriteSyncResults([]*PRFetchResult{{
		PR: pr, Comments: []*provider.Comment{comment}, CommentsFetched: true, ThreadsFetched: true,
	}}); err != nil {
		t.Fatalf("initial WriteSyncResults: %v", err)
	}

	refreshedPR := mentionedTestPR(pr.ID, "no mention here")
	if err := writer.WriteSyncResults([]*PRFetchResult{{
		PR: refreshedPR, CommentsFetched: true, ThreadsFetched: true,
	}}); err != nil {
		t.Fatalf("refreshed WriteSyncResults: %v", err)
	}

	if got := mentionedValue(t, db, pr.ID); got != 0 {
		t.Errorf("mentioned after refreshed sync = %d, want 0", got)
	}
}

func TestWriterPreservesCommentMentionWithoutCommentRefresh(t *testing.T) {
	db := storetest.Open(t)
	pr := mentionedTestPR("github:org/repo:mention-no-comment-refresh", "no mention here")
	writer := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "alice", ok: true})
	comment := &provider.Comment{
		ID:        "mention-no-comment-refresh-comment",
		PRID:      pr.ID,
		Author:    "bob",
		Body:      "Please look, @alice.",
		CreatedAt: mustTime(),
		UpdatedAt: mustTime(),
	}

	if err := writer.WriteSyncResults([]*PRFetchResult{{
		PR: pr, Comments: []*provider.Comment{comment}, CommentsFetched: true, ThreadsFetched: true,
	}}); err != nil {
		t.Fatalf("hydrated WriteSyncResults: %v", err)
	}

	withoutComments := mentionedTestPR(pr.ID, "no mention here")
	if err := writer.WriteSyncResults([]*PRFetchResult{{PR: withoutComments}}); err != nil {
		t.Fatalf("without-comments WriteSyncResults: %v", err)
	}

	if got := mentionedValue(t, db, pr.ID); got != 1 {
		t.Errorf("mentioned after without-comments sync = %d, want 1", got)
	}
}

func TestWriterListOnlySyncPreservesCommentMention(t *testing.T) {
	db := storetest.Open(t)
	pr := mentionedTestPR("github:org/repo:mention-list-only", "no mention here")
	writer := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "alice", ok: true})
	comment := &provider.Comment{
		ID:        "mention-list-only-comment",
		PRID:      pr.ID,
		Author:    "bob",
		Body:      "Please look, @alice.",
		CreatedAt: mustTime(),
		UpdatedAt: mustTime(),
	}

	if err := writer.WriteSyncResults([]*PRFetchResult{{PR: pr, Comments: []*provider.Comment{comment}}}); err != nil {
		t.Fatalf("hydrated WriteSyncResults: %v", err)
	}

	listOnlyPR := mentionedTestPR(pr.ID, "no mention here")
	if err := writer.WriteSyncResults([]*PRFetchResult{{PR: listOnlyPR, ListOnly: true}}); err != nil {
		t.Fatalf("list-only WriteSyncResults: %v", err)
	}

	if got := mentionedValue(t, db, pr.ID); got != 1 {
		t.Errorf("mentioned after list-only sync = %d, want 1", got)
	}
}
