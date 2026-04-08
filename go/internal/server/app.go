package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/samuelasvanyi/bistro/internal/git"
	"github.com/samuelasvanyi/bistro/internal/provider"
	"github.com/samuelasvanyi/bistro/internal/store"
	"github.com/samuelasvanyi/bistro/internal/sync"
	"github.com/samuelasvanyi/bistro/internal/ws"
)

// App is the top-level orchestrator for the bistro-sync process.
type App struct {
	log          *slog.Logger
	db           *store.DB
	wsServer     *ws.Server
	wsPort       int
	dbPath       string
	orchestrator *sync.Orchestrator
}

// setupOrchestrator wires the sync orchestrator into the app using the given provider and git client.
func (a *App) setupOrchestrator(prov provider.Provider, gitClient *git.Client) {
	a.orchestrator = sync.NewOrchestrator(a.log, prov, a.db, gitClient)
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
func New(wsPort int, dbPath string) *App {
	return &App{
		log:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
		wsServer: ws.NewServer(),
		wsPort:   wsPort,
		dbPath:   dbPath,
	}
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
