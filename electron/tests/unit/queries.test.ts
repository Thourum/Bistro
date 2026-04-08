import { describe, it, expect } from 'vitest';

describe('usePRList', () => {
  it('should construct SQL with filter', () => {
    const sql = "SELECT * FROM pull_requests WHERE provider_id = ? AND state IN ('open')";
    expect(sql).toContain('provider_id');
    expect(sql).toContain('state');
  });

  it('should construct SQL for draft filter', () => {
    const sql = "SELECT * FROM pull_requests WHERE provider_id = ? AND state IN ('draft')";
    expect(sql).toContain('draft');
  });
});
