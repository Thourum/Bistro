-- Migration: 018_add_anchor_patchset_number
-- Description: Add anchor_patchset_number to comments for precise anchor cache
--              invalidation. anchor_head_sha alone is insufficient when the same
--              head SHA appears under multiple bases (retarget). Comparing
--              anchor_patchset_number against the current patchset number for
--              (pr_id, head_sha, base_sha) detects retargets that produce
--              different patchset geometry with the same head.
--              NULL = anchor was computed before this migration; treat as stale.

ALTER TABLE comments ADD COLUMN anchor_patchset_number INTEGER;
