package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/samuelasvanyi/bistro/internal/git"
	"github.com/samuelasvanyi/bistro/internal/keychain"
	"github.com/samuelasvanyi/bistro/internal/provider"
	github "github.com/samuelasvanyi/bistro/internal/provider/github"
	"github.com/samuelasvanyi/bistro/internal/store"
	"github.com/samuelasvanyi/bistro/internal/sync"
	"github.com/samuelasvanyi/bistro/internal/ws"
)

// App is the top-level orchestrator for the bistro-sync process.
type App struct {
	log            *slog.Logger
	db             *store.DB
	wsServer       *ws.Server
	wsPort         int
	dbPath         string
	orchestrator   *sync.Orchestrator
	prov           provider.Provider
	gitClient      *git.Client
	githubClientID string
}

// SetProvider configures the GitHub provider used by the sync orchestrator.
// Must be called before Run.
func (a *App) SetProvider(prov provider.Provider) {
	a.prov = prov
}

// SetGitClient configures the git client used by the sync orchestrator.
// Must be called before Run.
func (a *App) SetGitClient(gitClient *git.Client) {
	a.gitClient = gitClient
}

// ReplayQueuedActions replays any pending offline queue actions.
// It is a no-op when the orchestrator has not been set up.
func (a *App) ReplayQueuedActions(ctx context.Context) error {
	if a.orchestrator == nil {
		return nil
	}
	return a.orchestrator.ReplayQueuedActions(ctx)
}

// New returns a new App configured with the given WebSocket port and database path.
// Uses default Info-level stderr logging.
func New(wsPort int, dbPath string) *App {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return &App{
		log:      logger,
		wsServer: ws.NewServerWithLogger(logger),
		wsPort:   wsPort,
		dbPath:   dbPath,
	}
}

// NewWithLogger returns a new App with a custom logger.
func NewWithLogger(wsPort int, dbPath string, logger *slog.Logger) *App {
	return &App{
		log:      logger,
		wsServer: ws.NewServerWithLogger(logger),
		wsPort:   wsPort,
		dbPath:   dbPath,
	}
}

// SetGitHubClientID sets the GitHub OAuth app client ID used by the device flow.
func (a *App) SetGitHubClientID(clientID string) {
	a.githubClientID = clientID
}

// handleAuthStart processes an AUTH_START WebSocket message by launching the
// GitHub device flow in a goroutine and returning nil immediately.
func (a *App) handleAuthStart(msg ws.InboundMessage) (interface{}, error) {
	var data struct {
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return nil, fmt.Errorf("invalid AUTH_START data: %w", err)
	}

	if data.Provider != "github" {
		return map[string]interface{}{
			"id":   msg.ID,
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": "unsupported provider: " + data.Provider},
		}, nil
	}

	go a.runDeviceFlow()
	return nil, nil
}

// runDeviceFlow performs the full GitHub device code flow: requests a device
// code, broadcasts it to Electron, polls for the token, stores it in keychain,
// and broadcasts AUTH_SUCCESS or AUTH_FAILED.
func (a *App) runDeviceFlow() {
	ctx := context.Background()
	clientID := a.githubClientID
	if clientID == "" {
		a.wsServer.Broadcast(map[string]interface{}{
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": "GITHUB_CLIENT_ID not configured"},
		})
		return
	}

	auth := github.NewAuthenticator(clientID)

	deviceCode, userCode, verificationURI, interval, err := auth.GetDeviceCode(ctx)
	if err != nil {
		a.log.Error("device code request failed", "error", err)
		a.wsServer.Broadcast(map[string]interface{}{
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": err.Error()},
		})
		return
	}

	a.wsServer.Broadcast(map[string]interface{}{
		"type": "AUTH_DEVICE_CODE",
		"data": map[string]interface{}{
			"user_code":        userCode,
			"verification_uri": verificationURI,
		},
	})

	token, err := auth.PollForToken(ctx, deviceCode, interval)
	if err != nil {
		a.log.Error("token polling failed", "error", err)
		a.wsServer.Broadcast(map[string]interface{}{
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": err.Error()},
		})
		return
	}

	tm := keychain.NewTokenManager()
	if err := tm.StoreGitHubToken(token); err != nil {
		a.log.Error("failed to store token in keychain", "error", err)
	}

	client := github.NewClient(token)
	a.SetProvider(client)

	a.wsServer.Broadcast(map[string]interface{}{
		"type": "AUTH_SUCCESS",
		"data": map[string]string{"provider": "github"},
	})
	a.log.Info("GitHub OAuth device flow completed successfully")
}

// Run initializes the database, starts the WebSocket server, and blocks until
// a SIGTERM or SIGINT signal is received, then shuts down gracefully.
func (a *App) Run(ctx context.Context) error {
	var err error

	a.db, err = store.OpenDB(a.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer a.db.Close()

	if err := store.NewMigrationRunner(a.log, a.db.DB).Run(ctx); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	a.log.Info("database initialized")

	diffStore := store.NewDiffStore(a.db.DB)

	if a.prov != nil && a.gitClient != nil {
		a.orchestrator = sync.NewOrchestrator(a.log, a.prov, a.db, a.gitClient).
			WithDiffStore(diffStore)
	}

	// Register inbound WebSocket handlers.
	a.wsServer.RegisterHandler("AUTH_START", a.handleAuthStart)
	a.wsServer.RegisterHandler("REQUEST_DIFF", newRequestDiffHandler(diffStore, a.log))

	wsAddr := fmt.Sprintf(":%d", a.wsPort)
	if err := a.wsServer.Start(wsAddr); err != nil {
		return fmt.Errorf("failed to start ws server: %w", err)
	}

	a.log.Info("WebSocket server listening", "addr", wsAddr)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	select {
	case <-sigChan:
		a.log.Info("shutdown signal received")
	case <-ctx.Done():
		a.log.Info("context cancelled")
	}

	return a.wsServer.Stop()
}
