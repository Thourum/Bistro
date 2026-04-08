package main

import (
	"context"
	"flag"
	"log"

	"github.com/samuelasvanyi/bistro/internal/server"
)

func main() {
	wsPort := flag.Int("ws-port", 8080, "WebSocket server port")
	dbPath := flag.String("db-path", "./data/sync.db", "SQLite database path")
	replayQueue := flag.Bool("replay-queue", true, "Replay offline queue on startup")
	flag.Parse()

	app := server.New(*wsPort, *dbPath)

	if *replayQueue {
		if err := app.ReplayQueuedActions(context.Background()); err != nil {
			log.Printf("queue replay failed: %v", err)
		}
	}

	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("app error: %v", err)
	}
}
