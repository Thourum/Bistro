import { describe, expect, it, vi } from 'vitest';
import { launchEditor } from './editorLaunch';

const silentLogger = () =>
  ({
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    // biome-ignore lint/suspicious/noExplicitAny: narrow test double for electron-log
  }) as any;

describe('launchEditor', () => {
  it('resolves ok once the child reports a successful spawn', async () => {
    const logger = silentLogger();

    const result = await launchEditor(
      process.execPath,
      ['-e', 'process.exit(0)'],
      logger,
    );

    expect(result.isOk()).toBe(true);
  });

  it('surfaces an asynchronous ENOENT instead of reporting success', async () => {
    const logger = silentLogger();

    const result = await launchEditor('__bistro_missing_editor__', [], logger);

    expect(result.isErr()).toBe(true);
    expect(result.isErr() && result.error).toMatch(/ENOENT|not found|spawn/i);
    expect(logger.error).toHaveBeenCalled();
  });

  it('detaches the child so it cannot keep the app alive', async () => {
    const logger = silentLogger();
    const unref = vi.fn();

    const result = await launchEditor(
      process.execPath,
      ['-e', 'process.exit(0)'],
      logger,
      // Inject a spawn stub so the detach contract is observable.
      () =>
        ({
          once: (event: string, cb: (...args: unknown[]) => void) => {
            if (event === 'spawn') setTimeout(() => cb(), 0);
          },
          off: vi.fn(),
          unref,
          // biome-ignore lint/suspicious/noExplicitAny: minimal ChildProcess double
        }) as any,
    );

    expect(result.isOk()).toBe(true);
    expect(unref).toHaveBeenCalledOnce();
  });

  it('reports a synchronous spawn throw as an error result', async () => {
    const logger = silentLogger();

    const result = await launchEditor('anything', [], logger, () => {
      throw new Error('EMFILE: too many open files');
    });

    expect(result.isErr()).toBe(true);
    expect(result.isErr() && result.error).toMatch(/EMFILE/);
  });
});
