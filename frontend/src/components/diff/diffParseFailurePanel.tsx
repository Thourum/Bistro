import type { UnrenderableDiffFile } from './diffCodeViewItems';

const BYTES_PER_KB = 1024;

/**
 * Panel shown in place of CodeView for a file it cannot render: a patch the
 * parser rejected, a binary file, or an oversized file without stored content.
 */
export function DiffParseFailurePanel({
  file,
  onOpenInBrowser,
}: {
  file: UnrenderableDiffFile;
  onOpenInBrowser?: () => void;
}): React.ReactElement {
  const isNeutral = file.reason !== 'parse-failed';
  const tone = isNeutral
    ? 'border-outline-variant/30 bg-surface-container text-outline'
    : 'border-error/30 bg-error/10 text-error';
  return (
    <section
      className={`border-b px-4 py-3 font-mono text-[10px] ${tone}`}
      data-unrenderable-diff-file={file.file.id}
    >
      <p className="uppercase tracking-widest">{panelTitle(file)}</p>
      <p className="mt-1 text-outline">
        {isNeutral
          ? file.file.path
          : 'The patch could not be parsed by the diff renderer.'}
      </p>
      {isNeutral && onOpenInBrowser !== undefined ? (
        <button
          type="button"
          onClick={onOpenInBrowser}
          className="mt-2 uppercase tracking-widest text-primary hover:underline"
        >
          {file.reason === 'too-large'
            ? 'o — open on GitHub'
            : 'o — open in browser'}
        </button>
      ) : null}
    </section>
  );
}

function panelTitle(file: UnrenderableDiffFile): string {
  if (file.reason === 'too-large')
    return `File too large (${Math.round(file.patchBytes / BYTES_PER_KB)} KB)`;
  return file.reason === 'binary'
    ? 'Binary file'
    : `Unable to render ${file.file.path}`;
}
