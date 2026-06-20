-- Migration: 019_add_anchor_current_patchset_number
-- Description: Separate current-cache identity from source geometry identity for
--              anchor freshness. anchor_patchset_number records the source patchset
--              used for comment position geometry; anchor_current_patchset_number
--              records the current patchset at time of anchor resolution (the
--              head+base state the anchor is valid against). They diverge for
--              original-position (historical) comment anchors re-resolved against
--              a newer head. A same-head/different-base retarget increments
--              current_patchset_number while leaving head_sha unchanged, so
--              requiring anchor_current_patchset_number == current patchset safely
--              invalidates stale anchors from before the retarget.
--              NULL = pre-migration cache; treated as stale by handler.

ALTER TABLE comments ADD COLUMN anchor_current_patchset_number INTEGER;
