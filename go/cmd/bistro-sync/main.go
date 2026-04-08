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
	flag.Parse()

	app := server.New(*wsPort, *dbPath)
	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("app error: %v", err)
	}
}
