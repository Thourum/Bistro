/**
 * PR domain model (mirrors Go backend)
 */
export interface PR {
  id: string;
  provider_id: string;
  title: string;
  number: number;
  state: 'open' | 'closed' | 'draft' | 'merged';
  author: string;
  created_at: string;
  updated_at: string;
  base_branch: string;
  head_branch: string;
  additions: number;
  deletions: number;
  changed_files: number;
  body: string;
}

export interface Comment {
  id: string;
  pr_id: string;
  author: string;
  body: string;
  created_at: string;
  updated_at: string;
  line?: number;
  file?: string;
  is_reply_to?: string;
}

export interface File {
  path: string;
  status: 'added' | 'modified' | 'deleted';
  additions: number;
  deletions: number;
}

export interface CIStatus {
  name: string;
  state: 'pending' | 'success' | 'failure';
  description?: string;
  url?: string;
}

export interface PRDetail extends PR {
  files: File[];
  comments: Comment[];
  ci_status: CIStatus[];
  approval_status: 'pending' | 'approved' | 'changes_requested';
}
