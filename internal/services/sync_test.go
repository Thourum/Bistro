package services

import (
	"context"
	"errors"
	"testing"
)

func assertServiceCode(t *testing.T, err error, code Code) *ServiceError {
	t.Helper()
	var serviceError *ServiceError
	if !errors.As(err, &serviceError) || serviceError.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
	return serviceError
}

func TestForceQueueing(t *testing.T) {
	for _, repoID := range []string{"", "github:Acme/API"} {
		queue := make(chan string, 1)
		s := &Sync{Trigger: queue}
		response, err := s.Force(context.Background(), ForceSyncRequest{RepoID: repoID})
		if err != nil || !response.Queued || response.RepoID != NormalizeID(repoID) {
			t.Fatalf("response = %+v, %v", response, err)
		}
		response, err = s.Force(context.Background(), ForceSyncRequest{RepoID: repoID})
		if err != nil || response.Queued {
			t.Fatalf("full queue response = %+v, %v", response, err)
		}
		if got := <-queue; got != NormalizeID(repoID) {
			t.Fatalf("queued %q", got)
		}
	}
}

type fakePRSyncer struct {
	id  string
	err error
}

func (f *fakePRSyncer) SyncPR(_ context.Context, id string) error { f.id = id; return f.err }

func TestPRSyncHydration(t *testing.T) {
	syncer := &fakePRSyncer{}
	s := &PR{GetSyncer: func() PRSyncer { return syncer }}
	response, err := s.Sync(context.Background(), PRRef{PRID: "github:Acme/API:1"})
	if err != nil || response.PRID != "github:acme/api:1" || syncer.id != response.PRID {
		t.Fatalf("response = %+v, %v; synced %q", response, err, syncer.id)
	}
	_, err = s.Sync(context.Background(), PRRef{})
	assertServiceCode(t, err, CodeInvalid)
	s.GetSyncer = func() PRSyncer { return nil }
	_, err = s.Sync(context.Background(), PRRef{PRID: "pr"})
	assertServiceCode(t, err, CodeUnauthenticated)
}
