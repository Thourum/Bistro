-- Migration: 020_add_comment_diff_hunk
-- Description: Add diff_hunk column to comments for review comment code context.
-- Populated only for kind='review' comments; NULL for issue comments.

ALTER TABLE comments ADD COLUMN diff_hunk TEXT;
