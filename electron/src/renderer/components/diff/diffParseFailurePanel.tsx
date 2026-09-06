import type { UnrenderableDiffFile } from './diffCodeViewItems';

/**
 * Panel shown in place of CodeView for a file it cannot render: a patch the
 * parser rejected, or a binary file git ships without text content.
 */
export function DiffParseFailurePanel({
  file,
  onOpenInBrowser,
}: {
  file: UnrenderableDiffFile;
  onOpenInBrowser?: () => void;
}): React.ReactElement {
  const isBinary = file.reason === 'binary';
  const tone = isBinary
    ? 'border-outline-variant/30 bg-surface-container text-outline'
    : 'border-error/30 bg-error/10 text-error';
  return (
    <section
      className={`border-b px-4 py-3 font-mono text-[10px] ${tone}`}
      data-unrenderable-diff-file={file.file.id}
    >
      <p className="uppercase tracking-widest">
        {isBinary ? 'Binary file' : `Unable to render ${file.file.path}`}
      </p>
      <p className="mt-1 text-outline">
        {isBinary
          ? file.file.path
          : 'The patch could not be parsed by the diff renderer.'}
      </p>
      {isBinary && onOpenInBrowser !== undefined ? (
        <button
          type="button"
          onClick={onOpenInBrowser}
          className="mt-2 uppercase tracking-widest text-primary hover:underline"
        >
          o — open in browser
        </button>
      ) : null}
    </section>
  );
}
