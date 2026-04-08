-- Migration: 002_add_pr_indexes
-- Description: Add indexes for common queries

CREATE INDEX IF NOT EXISTS idx_prs_updated_at ON prs(updated_at);
CREATE INDEX IF NOT EXISTS idx_prs_state ON prs(state);
CREATE INDEX IF NOT EXISTS idx_prs_provider_owner_repo ON prs(provider, owner, repo);
CREATE INDEX IF NOT EXISTS idx_comments_pr_id ON comments(pr_id);
CREATE INDEX IF NOT EXISTS idx_files_pr_id ON files(pr_id);
CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);
