-- Migration: 026_add_repo_location
-- Description: Add dev-folder/local vs managed-cache location tracking per repo.

ALTER TABLE repos ADD COLUMN local_path          TEXT;
ALTER TABLE repos ADD COLUMN source              TEXT;
ALTER TABLE repos ADD COLUMN last_used_at        DATETIME;
ALTER TABLE repos ADD COLUMN location_status     TEXT;
ALTER TABLE repos ADD COLUMN location_error_code TEXT;
ALTER TABLE repos ADD COLUMN location_error      TEXT;
