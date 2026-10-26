package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/store/repolocation"
	"github.com/itterno/bistro/internal/store/storetest"
	"github.com/itterno/bistro/internal/ws"
)

// newTestScanDevFolderHandler joins the worker and adapts its broadcast for the
// existing repo-location assertions, while leaving synchronous validation intact.
func newTestScanDevFolderHandler(t *testing.T, locations *repolocation.Store, syncs chan<- string) ws.MessageHandler {
	t.Helper()
	responses := make(chan any, 1)
	start, _, wait := NewScanDevFolderHandlers(ScanDevFolderDeps{
		Context: context.Background(), Locations: locations, SyncTrigger: syncs,
		Broadcast: func(response any) { responses <- response }, Scan: scan.Scan, Log: testRepoLocationLogger(),
	})
	t.Cleanup(wait)
	return func(ctx context.Context, msg ws.InboundMessage) (any, error) {
		response, err := start(ctx, msg)
		if err != nil || response != nil {
			return response, err
		}
		select {
		case response := <-responses:
			wait()
			return response, nil
		case <-time.After(2 * time.Second):
			t.Fatal("scan did not broadcast its result")
			return nil, nil
		}
	}
}

func TestScanCancelOnSameSocketPreservesDiscoveries(t *testing.T) {
	database := storetest.Open(t)
	root := t.TempDir()
	clone := filepath.Join(root, "api")
	if err := createRepo(clone, "origin", "https://github.com/acme/api.git"); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	entered := make(chan struct{})
	syncs := make(chan string, 1)
	server := ws.NewServer()
	start, cancel, wait := NewScanDevFolderHandlers(ScanDevFolderDeps{
		Context: ctx, Locations: repolocation.New(database.DB),
		SyncTrigger: syncs, Broadcast: server.Broadcast, Log: testRepoLocationLogger(),
		Scan: func(scanCtx context.Context, _ string) (scan.Result, error) {
			close(entered)
			<-scanCtx.Done()
			return scan.Result{Found: []scan.Found{{Identity: remote.Identity{Provider: "github", Owner: "acme", Repo: "api"}, Path: clone}}, Partial: true}, nil
		},
	})
	defer func() { stop(); wait() }()
	server.RegisterHandler("SCAN_DEV_FOLDER", start)
	server.RegisterHandler("SCAN_CANCEL", cancel)
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := server.Stop(); err != nil {
			t.Error(err)
		}
	}()
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", server.Port()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(map[string]any{"id": "scan-1", "type": "SCAN_DEV_FOLDER", "data": map[string]any{"path": root, "reason": "settings-change"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("scan never started")
	}
	if err := conn.WriteJSON(map[string]any{"type": "SCAN_CANCEL", "data": map[string]any{"scan_id": "old-scan"}}); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(map[string]any{"type": "SCAN_CANCEL", "data": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("same-socket Stop failed: %v", err)
	}
	var response struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			OK         bool `json:"ok"`
			Partial    bool `json:"partial"`
			Discovered int  `json:"discovered"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "scan-1" || response.Type != "SCAN_DEV_FOLDER_RESULT" || !response.Data.OK || !response.Data.Partial || response.Data.Discovered != 1 {
		t.Fatalf("unexpected response: %s", raw)
	}
	var localPath string
	var watched int
	if err := database.QueryRow(`SELECT local_path, watched FROM repos WHERE id='github:acme/api'`).Scan(&localPath, &watched); err != nil {
		t.Fatal(err)
	}
	if localPath != clone || watched != 0 {
		t.Fatalf("partial discoveries lost or watched: %s %d", localPath, watched)
	}
	select {
	case id := <-syncs:
		if id != "" {
			t.Fatalf("trigger = %q", id)
		}
	default:
		t.Fatal("partial scan did not sync")
	}
	if response, err := cancel(ctx, ws.InboundMessage{Type: "SCAN_CANCEL", Data: json.RawMessage(`{}`)}); err != nil || response != nil {
		t.Fatalf("repeated cancel: response=%v error=%v", response, err)
	}
}

func TestScanDevFolderContextErrorsPreserveDiscoveries(t *testing.T) {
	for _, scanErr := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(scanErr.Error(), func(t *testing.T) {
			database := storetest.Open(t)
			root := t.TempDir()
			clone := filepath.Join(root, "api")
			if err := createRepo(clone, "origin", "https://github.com/acme/api.git"); err != nil {
				t.Fatal(err)
			}
			responses := make(chan any, 1)
			start, _, wait := NewScanDevFolderHandlers(ScanDevFolderDeps{
				Context: context.Background(), Locations: repolocation.New(database.DB),
				Broadcast: func(response any) { responses <- response }, Log: testRepoLocationLogger(),
				Scan: func(context.Context, string) (scan.Result, error) {
					return scan.Result{Found: []scan.Found{{Identity: remote.Identity{Provider: "github", Owner: "acme", Repo: "api"}, Path: clone}}}, fmt.Errorf("walk interrupted: %w", scanErr)
				},
			})
			defer wait()
			startTestScan(t, start, "scan-1", root)
			response := readTestScanResponse(t, responses)
			wait()
			data := responseData(t, response)
			if data["ok"] != true || data["partial"] != true || data["discovered"] != float64(1) {
				t.Fatalf("unexpected partial response: %#v", response)
			}
			assertLocationStatus(t, repolocation.New(database.DB), "github:acme/api", "ok", "")
		})
	}
}

func TestScanDevFolderErrorsAreCorrelated(t *testing.T) {
	for _, failure := range []string{"scan_failed", "persistence_failed"} {
		t.Run(failure, func(t *testing.T) {
			database := storetest.Open(t)
			if failure == "persistence_failed" {
				if _, err := database.Exec("DROP TABLE repos"); err != nil {
					t.Fatal(err)
				}
			}
			responses := make(chan any, 1)
			syncs := make(chan string, 1)
			start, _, wait := NewScanDevFolderHandlers(ScanDevFolderDeps{
				Context: context.Background(), Locations: repolocation.New(database.DB), SyncTrigger: syncs,
				Broadcast: func(response any) { responses <- response }, Log: testRepoLocationLogger(),
				Scan: func(context.Context, string) (scan.Result, error) {
					if failure == "scan_failed" {
						return scan.Result{}, errors.New("filesystem failed")
					}
					return scan.Result{}, nil
				},
			})
			defer wait()
			startTestScan(t, start, "scan-error", t.TempDir())
			response := readTestScanResponse(t, responses)
			wait()
			if response["id"] != "scan-error" || response["type"] != "SCAN_DEV_FOLDER_RESULT" {
				t.Fatalf("uncorrelated response: %#v", response)
			}
			assertLocationEnvelope(t, responseData(t, response), "", "scan_dev_folder", failure)
			select {
			case <-syncs:
				t.Fatal("failed scan triggered sync")
			default:
			}
		})
	}
}

func TestScanDevFolderReplacementWaitsForPredecessor(t *testing.T) {
	database := storetest.Open(t)
	locations := repolocation.New(database.DB)
	root := t.TempDir()
	clone := filepath.Join(root, "api")
	if err := createRepo(clone, "origin", "https://github.com/acme/api.git"); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	firstEntered := make(chan struct{})
	firstPersisted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondEntered := make(chan context.Context, 1)
	responses := make(chan any, 2)
	calls := 0
	start, cancel, wait := NewScanDevFolderHandlers(ScanDevFolderDeps{
		Context: ctx, Locations: locations, Log: testRepoLocationLogger(),
		Broadcast: func(response any) {
			envelope, ok := response.(map[string]any)
			if ok && envelope["id"] == "first" {
				close(firstPersisted)
				select {
				case <-releaseFirst:
				case <-ctx.Done():
					return
				}
			}
			responses <- response
		},
		Scan: func(scanCtx context.Context, _ string) (scan.Result, error) {
			calls++
			if calls == 1 {
				close(firstEntered)
				<-scanCtx.Done()
				return scan.Result{Found: []scan.Found{{Identity: remote.Identity{Provider: "github", Owner: "acme", Repo: "api"}, Path: clone}}}, scanCtx.Err()
			}
			secondEntered <- scanCtx
			<-scanCtx.Done()
			return scan.Result{}, scanCtx.Err()
		},
	})
	defer func() { stop(); wait() }()
	startTestScan(t, start, "first", root)
	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first scan never entered")
	}
	startTestScan(t, start, "second", root)
	select {
	case <-firstPersisted:
	case <-time.After(time.Second):
		t.Fatal("replacement did not cancel and persist first scan")
	}
	select {
	case <-secondEntered:
		t.Fatal("replacement started before predecessor finished")
	default:
	}
	assertLocationStatus(t, locations, "github:acme/api", "ok", "")
	close(releaseFirst)
	readTestScanResponse(t, responses)
	var secondCtx context.Context
	select {
	case secondCtx = <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("replacement never started")
	}
	if _, err := cancel(ctx, ws.InboundMessage{Type: "SCAN_CANCEL", Data: json.RawMessage(`{"scan_id":"first"}`)}); err != nil {
		t.Fatal(err)
	}
	if secondCtx.Err() != nil {
		t.Fatal("stale scan ID canceled replacement")
	}
	if _, err := cancel(ctx, ws.InboundMessage{Type: "SCAN_CANCEL", Data: json.RawMessage(`{"scan_id":"second"}`)}); err != nil {
		t.Fatal(err)
	}
	response := readTestScanResponse(t, responses)
	wait()
	if response["id"] != "second" || responseData(t, response)["partial"] != true {
		t.Fatalf("replacement ownership lost: %#v", response)
	}
}

func TestScanDevFolderShutdownJoinsAndRejectsStarts(t *testing.T) {
	for _, shutdown := range []string{"run canceled", "wait called"} {
		t.Run(shutdown, func(t *testing.T) {
			database := storetest.Open(t)
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			entered := make(chan struct{}, 1)
			responses := make(chan any, 2)
			start, _, wait := NewScanDevFolderHandlers(ScanDevFolderDeps{
				Context: ctx, Locations: repolocation.New(database.DB), Log: testRepoLocationLogger(),
				Broadcast: func(response any) { responses <- response },
				Scan: func(scanCtx context.Context, _ string) (scan.Result, error) {
					entered <- struct{}{}
					<-scanCtx.Done()
					return scan.Result{}, scanCtx.Err()
				},
			})
			defer wait()
			root := t.TempDir()
			startTestScan(t, start, "first", root)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("scan never started")
			}
			if shutdown == "run canceled" {
				stop()
			}
			wait()
			if shutdown == "run canceled" {
				select {
				case response := <-responses:
					t.Fatalf("shutdown broadcast result: %#v", response)
				default:
				}
			}
			startTestScan(t, start, "after-shutdown", root)
			wait()
			select {
			case <-entered:
				t.Fatal("scan started after shutdown")
			default:
			}
		})
	}
}

func TestScanCancelRejectsMalformedID(t *testing.T) {
	_, cancel, wait := NewScanDevFolderHandlers(ScanDevFolderDeps{Context: context.Background()})
	defer wait()
	if _, err := cancel(context.Background(), ws.InboundMessage{Type: "SCAN_CANCEL", Data: json.RawMessage(`{"scan_id":42}`)}); err == nil {
		t.Fatal("malformed scan ID accepted")
	}
}

func startTestScan(t *testing.T, start ws.MessageHandler, id, path string) {
	t.Helper()
	response, err := start(context.Background(), ws.InboundMessage{
		ID: id, Type: "SCAN_DEV_FOLDER", Data: mustJSON(map[string]any{"path": path, "reason": "settings-change"}),
	})
	if err != nil || response != nil {
		t.Fatalf("scan did not start asynchronously: response=%v error=%v", response, err)
	}
}

func readTestScanResponse(t *testing.T, responses <-chan any) map[string]any {
	t.Helper()
	select {
	case response := <-responses:
		envelope, ok := response.(map[string]any)
		if !ok {
			t.Fatalf("expected response envelope, got %T", response)
		}
		return envelope
	case <-time.After(2 * time.Second):
		t.Fatal("scan did not broadcast its result")
		return nil
	}
}
