-- +goose Up
-- Migration: 000_schema_migrations
-- Description: Create migration tracking table

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
-- This migration is irreversible.
