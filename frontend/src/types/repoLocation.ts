export type LocationErrorCode =
  | 'path_missing'
  | 'remote_mismatch'
  | 'permission_denied'
  | 'invalid_url'
  | 'invalid_path'
  | 'destination_required'
  | 'destination_conflict'
  | 'clone_failed'
  | 'auth_failed'
  | 'cancelled'
  | 'delete_failed'
  | 'scan_partial';

export interface LocationError {
  repo_id: string;
  operation: 'scan' | 'fetch' | 'diff' | 'clone' | 'prune' | 'recover';
  code: LocationErrorCode;
  message: string;
}

interface ScannedRepository {
  repo_id: string;
  path: string;
  remote_url: string;
  source: 'local';
}

export interface LegacyScanDevFolderResult {
  path: string;
  repositories: ScannedRepository[];
  errors: LocationError[];
}

interface ScanDevFolderSummaryResult {
  ok: true;
  discovered: number;
  healthy: number;
  degraded: number;
  skipped: number;
  partial: boolean;
}

export type ScanDevFolderResult =
  | LegacyScanDevFolderResult
  | ScanDevFolderSummaryResult;
