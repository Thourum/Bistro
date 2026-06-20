-- Migration: 017_add_patchset_identity_to_last_reviewed
-- Description: Add patchset_number and base_sha to last_reviewed_sha for unambiguous since-last-review resolution.
-- Legacy rows (written before this migration) have NULL for both columns and are
-- treated as "patchset number unknown" — the handler degrades gracefully for them.

ALTER TABLE last_reviewed_sha ADD COLUMN patchset_number INTEGER;
ALTER TABLE last_reviewed_sha ADD COLUMN base_sha TEXT;
