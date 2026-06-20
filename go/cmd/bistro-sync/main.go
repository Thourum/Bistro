package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/keychain"
	github "github.com/itterno/bistro/internal/provider/github"
	"github.com/itterno/bistro/internal/server"
)

func main() {
	wsPort := flag.Int("ws-port", 8080, "WebSocket server port (0 = random available port, reported via ready signal on stdout)")
	dbPath := flag.String("db-path", "./data/sync.db", "SQLite database path")
	replayQueue := flag.Bool("replay-queue", true, "Replay offline queue on startup")
	logLevel := flag.String("log-level", "info", "Log level: debug, info, warn, error")
	logFile := flag.String("log-file", "", "Log file path (default: stderr only)")
	syncInterval := flag.Duration("sync-interval", 5*time.Minute, "Interval between sync cycles")
	autoSync := flag.Bool("auto-sync", true, "Enable automatic periodic sync (manual FORCE_SYNC is unaffected)")
	exitOnStdinClose := flag.Bool("exit-on-stdin-close", false,
		"Exit when stdin closes (set by the Electron shell so the engine dies with its parent)")
	flag.Parse()

	if *syncInterval <= 0 {
		fmt.Fprintf(os.Stderr, "invalid -sync-interval: %s (must be > 0)\n", syncInterval.String())
		os.Exit(2)
	}

	// Setup logging
	logCloser, err := setupLogging(*logLevel, *logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup logging: %v\n", err)
		os.Exit(1)
	}
	if logCloser != nil {
		defer logCloser.Close()
	}

	// Single-instance guard: a second engine on the same database would
	// fight over the SQLite write lock and wedge both syncs. An orphaned
	// instance (e.g. Electron crashed without killing its child) caused
	// exactly that, so fail fast and loud instead.
	if dbDir := filepath.Dir(*dbPath); dbDir != "." && dbDir != "" {
		if err := os.MkdirAll(dbDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create db directory %s: %v\n", dbDir, err)
			os.Exit(1)
		}
	}
	lock := flock.New(*dbPath + ".lock")
	locked, err := lock.TryLock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to acquire instance lock %s: %v\n", lock.Path(), err)
		os.Exit(1)
	}
	if !locked {
		fmt.Fprintf(os.Stderr,
			"another bistro-sync instance is already using %s — kill it or pass a different -db-path\n",
			*dbPath,
		)
		os.Exit(1)
	}
	defer lock.Unlock() //nolint:errcheck

	log := slog.Default()
	log.Info("bistro-sync starting",
		"ws-port", *wsPort,
		"db-path", *dbPath,
		"log-level", *logLevel,
		"log-file", *logFile,
	)

	app := server.NewWithLogger(*wsPort, *dbPath, log)
	app.SetSyncInterval(*syncInterval)
	app.SetAutoSync(*autoSync)

	// Read GitHub OAuth credentials from environment.
	clientID := os.Getenv("GITHUB_CLIENT_ID")
	if clientID == "" {
		log.Warn("GITHUB_CLIENT_ID not set — OAuth will fail until configured")
	}
	app.SetGitHubClientID(clientID)

	clientSecret := os.Getenv("GITHUB_CLIENT_SECRET")
	if clientSecret == "" {
		log.Warn("GITHUB_CLIENT_SECRET not set — OAuth will fail until configured")
	}
	app.SetGitHubClientSecret(clientSecret)

	// Check keychain for an existing GitHub token — skip the auth flow if found.
	tm := keychain.NewTokenManager()

	// Git client for cloning PR branches and generating diffs locally.
	// Clone auth comes from the keychain so private repos work without env vars.
	gitClient := git.NewClient(git.NewCache())
	gitClient.SetTokenSource(func() string {
		token, err := tm.GetGitHubToken()
		if err != nil {
			return ""
		}
		return token
	})
	app.SetGitClient(gitClient)

	existingToken, err := tm.GetGitHubToken()
	if err == nil && existingToken != "" {
		log.Info("found existing GitHub token in keychain — skipping OAuth flow")
		client := github.NewClient(existingToken)
		app.SetProvider(client)
	} else {
		log.Info("no GitHub token in keychain — waiting for OAuth authorization code flow")
	}

	// Parent-death watchdog: Electron holds our stdin pipe open. If the
	// shell dies without killing us (crash, SIGKILL, killed process tree),
	// stdin hits EOF and we exit instead of becoming an orphan that holds
	// the database hostage.
	if *exitOnStdinClose {
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			log.Info("stdin closed — parent gone, shutting down")
			os.Exit(0)
		}()
	}

	// Tell Run() to replay the queue after the DB and orchestrator are ready.
	// Pre-Run replay was always a no-op: the orchestrator (and its queue) does
	// not exist until Run() opens the database.
	app.SetReplayOnStart(*replayQueue)

	if err := app.Run(context.Background()); err != nil {
		log.Error("app error", "error", err)
		os.Exit(1)
	}
}

// setupLogging configures slog with the given level and optional file output.
// Returns an io.Closer for the log file (nil when no file is used). The caller
// must close it when the process exits.
func setupLogging(level, logFile string) (io.Closer, error) {
	// Parse log level
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level: %s (must be debug, info, warn, error)", level)
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
		AddSource: logLevel <= slog.LevelDebug, // Include source file:line for debug and below
	}

	var out io.Writer = os.Stderr

	var closer io.Closer

	// If log file specified, write to both stderr and file
	if logFile != "" {
		// Ensure log directory exists
		logDir := filepath.Dir(logFile)
		if logDir != "." && logDir != "" {
			if err := os.MkdirAll(logDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create log directory: %w", err)
			}
		}

		// Open log file for appending
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}

		// Log to both stderr and file
		out = io.MultiWriter(os.Stderr, f)
		closer = f

		// Log file location to stderr immediately
		fmt.Fprintf(os.Stderr, "Logs written to: %s\n", logFile)
	}

	handler := slog.NewTextHandler(out, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return closer, nil
}
