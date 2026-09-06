import { mkdir, mkdtemp, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { resolveEditorTarget } from './editorTarget';

/**
 * Builds a temp git checkout plus an out-of-tree sibling directory:
 *
 *   <base>/repo/.git/
 *   <base>/repo/src/app.ts
 *   <base>/outside/secret.env
 */
async function buildFixture(): Promise<{
  base: string;
  repoRoot: string;
  outside: string;
}> {
  const base = await mkdtemp(join(tmpdir(), 'bistro-editor-target-'));
  const repoRoot = join(base, 'repo');
  const outside = join(base, 'outside');
  await mkdir(join(repoRoot, '.git'), { recursive: true });
  await mkdir(join(repoRoot, 'src'), { recursive: true });
  await mkdir(outside, { recursive: true });
  await writeFile(join(repoRoot, 'src', 'app.ts'), 'export const a = 1;\n');
  await writeFile(join(outside, 'secret.env'), 'TOKEN=hunter2\n');
  return { base, repoRoot, outside };
}

describe('resolveEditorTarget', () => {
  it('accepts a plain file inside the checkout and returns its canonical path', async () => {
    const { repoRoot } = await buildFixture();

    const result = await resolveEditorTarget({
      repoRoot,
      relativePath: 'src/app.ts',
    });

    expect(result.isOk()).toBe(true);
    expect(
      result.isOk() && result.value.endsWith(join('repo', 'src', 'app.ts')),
    ).toBe(true);
  });

  it('rejects an in-repo symlink whose target escapes the checkout', async () => {
    const { repoRoot, outside } = await buildFixture();
    await symlink(join(outside, 'secret.env'), join(repoRoot, 'leak.env'));

    const result = await resolveEditorTarget({
      repoRoot,
      relativePath: 'leak.env',
    });

    expect(result.isErr()).toBe(true);
    expect(result.isErr() && result.error).toMatch(/outside repository/);
  });

  it('rejects a file reached through an in-repo symlinked directory', async () => {
    const { repoRoot, outside } = await buildFixture();
    await symlink(outside, join(repoRoot, 'linked'));

    const result = await resolveEditorTarget({
      repoRoot,
      relativePath: join('linked', 'secret.env'),
    });

    expect(result.isErr()).toBe(true);
    expect(result.isErr() && result.error).toMatch(/outside repository/);
  });

  it.each([
    ['parent traversal', join('..', 'outside', 'secret.env')],
    ['absolute escape', '/etc/passwd'],
    ['nested traversal', join('src', '..', '..', 'outside', 'secret.env')],
  ])('rejects %s', async (_label, relativePath) => {
    const { repoRoot } = await buildFixture();

    const result = await resolveEditorTarget({ repoRoot, relativePath });

    expect(result.isErr()).toBe(true);
  });

  it('rejects a sibling directory that merely shares the root prefix', async () => {
    const { base } = await buildFixture();
    const evil = join(base, 'repo-evil');
    await mkdir(join(evil, '.git'), { recursive: true });
    await writeFile(join(evil, 'note.txt'), 'nope\n');

    const result = await resolveEditorTarget({
      repoRoot: join(base, 'repo'),
      relativePath: join('..', 'repo-evil', 'note.txt'),
    });

    expect(result.isErr()).toBe(true);
  });

  it('rejects a repoRoot that is not a git checkout', async () => {
    const { outside } = await buildFixture();

    const result = await resolveEditorTarget({
      repoRoot: outside,
      relativePath: 'secret.env',
    });

    expect(result.isErr()).toBe(true);
    expect(result.isErr() && result.error).toMatch(/not a git checkout/);
  });

  it('rejects a relative repoRoot instead of resolving it against cwd', async () => {
    const result = await resolveEditorTarget({
      repoRoot: 'repo',
      relativePath: 'src/app.ts',
    });

    expect(result.isErr()).toBe(true);
    expect(result.isErr() && result.error).toMatch(/absolute/);
  });

  it('rejects a repoRoot that does not exist', async () => {
    const { base } = await buildFixture();

    const result = await resolveEditorTarget({
      repoRoot: join(base, 'missing'),
      relativePath: 'src/app.ts',
    });

    expect(result.isErr()).toBe(true);
  });

  it('rejects a target that does not exist inside the checkout', async () => {
    const { repoRoot } = await buildFixture();

    const result = await resolveEditorTarget({
      repoRoot,
      relativePath: join('src', 'nope.ts'),
    });

    expect(result.isErr()).toBe(true);
  });
});
