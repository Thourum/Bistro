package services

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/services/servicestest"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestMarkReadPersistsAndEmitsNormalizedUniqueIDs(t *testing.T) {
	db := storetest.Open(t)
	events := &servicestest.Recorder{}
	s := &ReadState{DB: db, CurrentUser: &mockCurrentUserReader{login: "alice", found: true}, Events: events}
	started := time.Now().UTC().Add(-time.Second)
	result, err := s.MarkRead(context.Background(), MarkReadRequest{PRIDs: []string{"github:O/R:1", "github:o/r:1", "github:O/R:2"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Marked != 2 {
		t.Fatalf("marked = %d", result.Marked)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM pr_read_state WHERE login='alice'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rows = %d", count)
	}
	var timestamp time.Time
	if err := db.QueryRow("SELECT last_read_at FROM pr_read_state WHERE pr_id='github:o/r:1' AND login='alice'").Scan(&timestamp); err != nil {
		t.Fatal(err)
	}
	if timestamp.Before(started) {
		t.Fatalf("timestamp = %v", timestamp)
	}
	want := []servicestest.Event{{Name: EventPRsReadChanged, Data: PRsReadChanged{PRIDs: []string{"github:o/r:1", "github:o/r:2"}}}}
	if !reflect.DeepEqual(events.Events(), want) {
		t.Fatalf("events = %+v, want %+v", events.Events(), want)
	}
}

func TestMarkReadRejectsInvalidAndUnauthenticatedRequests(t *testing.T) {
	db := storetest.Open(t)
	events := &servicestest.Recorder{}
	s := &ReadState{DB: db, CurrentUser: &mockCurrentUserReader{login: "alice", found: true}, Events: events}
	for _, ids := range [][]string{nil, make([]string, maxPRsReadRequestSize+1), {"p", ""}} {
		_, err := s.MarkRead(context.Background(), MarkReadRequest{PRIDs: ids})
		assertServiceCode(t, err, CodeInvalid)
	}
	s.CurrentUser = &mockCurrentUserReader{}
	_, err := s.MarkRead(context.Background(), MarkReadRequest{PRIDs: []string{"p"}})
	assertServiceCode(t, err, CodeUnauthenticated)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM pr_read_state").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 || len(events.Events()) != 0 {
		t.Fatalf("rejected request wrote %d rows and %d events", count, len(events.Events()))
	}
}

func TestMarkReadTimestampComparableWithUpdatedAt(t *testing.T) {
	db := storetest.Open(t)
	updated := time.Now().UTC().Add(-time.Minute)
	mustExecServicesDB(t, db.DB, `INSERT INTO prs (id,provider,owner,repo,number,title,author,state,created_at,updated_at,base_branch,head_branch,base_sha,head_sha)
 VALUES ('p','github','o','r',1,'T','a','open',?,?,'main','f','base','head')`, updated, updated)
	_, err := (&ReadState{DB: db, CurrentUser: &mockCurrentUserReader{login: "alice", found: true}, Events: &servicestest.Recorder{}}).MarkRead(context.Background(), MarkReadRequest{PRIDs: []string{"p"}})
	if err != nil {
		t.Fatal(err)
	}
	var unread int
	err = db.QueryRow(`SELECT COALESCE((SELECT unixepoch(r.last_read_at) FROM pr_read_state r WHERE r.pr_id=prs.id AND r.login='alice'),0)<unixepoch(prs.updated_at) FROM prs WHERE id='p'`).Scan(&unread)
	if err != nil {
		t.Fatal(err)
	}
	if unread != 0 {
		t.Fatal("marked PR remains unread")
	}
}

func TestMarkReadRollsBackBeforeEmitting(t *testing.T) {
	db := storetest.Open(t)
	events := &servicestest.Recorder{}
	mustExecServicesDB(t, db.DB, `CREATE TRIGGER reject_read BEFORE INSERT ON pr_read_state WHEN NEW.pr_id='reject' BEGIN SELECT RAISE(ABORT,'rejected'); END`)
	_, err := (&ReadState{DB: db, CurrentUser: &mockCurrentUserReader{login: "alice", found: true}, Events: events}).MarkRead(context.Background(), MarkReadRequest{PRIDs: []string{"p", "reject"}})
	assertServiceCode(t, err, CodeInternal)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM pr_read_state").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 || len(events.Events()) != 0 {
		t.Fatalf("failed batch left %d rows and %d events", count, len(events.Events()))
	}
}

func TestMarkReadAcceptsLimitBeforeDeduplication(t *testing.T) {
	db := storetest.Open(t)
	events := &servicestest.Recorder{}
	ids := make([]string, maxPRsReadRequestSize)
	for i := range ids {
		ids[i] = "github:O/R:1"
	}
	result, err := (&ReadState{DB: db, CurrentUser: &mockCurrentUserReader{login: "alice", found: true}, Events: events}).MarkRead(context.Background(), MarkReadRequest{PRIDs: ids})
	if err != nil || result.Marked != 1 || len(events.Events()) != 1 {
		t.Fatalf("limit request = %+v, %v, events %+v", result, err, events.Events())
	}
}
