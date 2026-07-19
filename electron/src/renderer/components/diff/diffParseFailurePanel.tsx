import type { UnrenderableDiffFile } from './diffCodeViewItems';

export function DiffParseFailurePanel({
  file,
}: {
  file: UnrenderableDiffFile;
}): React.ReactElement {
  return (
    <section
      className="border-b border-error/30 bg-error/10 px-4 py-3 font-mono text-[10px] text-error"
      data-unrenderable-diff-file={file.file.id}
    >
      <p className="uppercase tracking-widest">
        Unable to render {file.file.path}
      </p>
      <p className="mt-1 text-outline">
        The patch could not be parsed by the diff renderer.
      </p>
    </section>
  );
}
